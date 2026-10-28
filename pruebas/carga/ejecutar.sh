#!/usr/bin/env bash
# Corre los escenarios A, B y C de ADR-015 sobre el sistema ya levantado (docker compose up -d).
#   A: 1 réplica de Integración, sin Memcached
#   B: 1 réplica, con Memcached
#   C: 2 réplicas, con Memcached
# Por escenario deja en pruebas/carga/: resultado-<E>.json (k6), stats-<E>.txt (docker stats cada
# 5 s) y k6-<E>.log (salida de k6). Después: node pruebas/carga/resumir.mjs
# Se ejecuta desde la raíz del repositorio: bash pruebas/carga/ejecutar.sh
set -euo pipefail

# En Git Bash (Windows) evita que se reescriban las rutas que se le pasan a docker.
export MSYS_NO_PATHCONV=1
DIR_HOST="$(pwd -W 2>/dev/null || pwd)/pruebas/carga"
CARPETA="pruebas/carga"
PROYECTO="${COMPOSE_PROJECT_NAME:-$(basename "$PWD" | tr '[:upper:]' '[:lower:]')}"
RED="${RED:-${PROYECTO}_default}"
IMAGEN_K6="${IMAGEN_K6:-grafana/k6:1.3.0}" # versión fija: la misma herramienta en cada corrida

muestrear_stats() {
  local archivo="$1"
  : > "$archivo"
  # Una muestra cada 5 s: cada una corre en segundo plano porque docker stats tarda ~3 s, y se
  # escribe de una vez (marca de tiempo + filas). Solo los contenedores de este proyecto.
  while true; do
    (
      local t; t=$(date +%s)
      local filas; filas=$(docker stats --no-stream --format '{{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}' $(docker compose ps -q))
      printf -- '--- %s\n%s\n' "$t" "$filas" >> "$archivo"
    ) &
    sleep 5
  done
}

k6() {
  docker run --rm --network "$RED" -v "$DIR_HOST:/scripts" "$IMAGEN_K6" run "$@"
}

correr() {
  local escenario="$1"
  echo "== Escenario $escenario: esperando 15 s a que se estabilice"
  sleep 15
  muestrear_stats "$CARPETA/stats-$escenario.txt" &
  local pid_stats=$!
  local pid_d5=""
  # d5.js es la medición histórica de ADR-016 y no corre con el contrato 4.0.0 (ver su encabezado).
  if [ "$escenario" = "C" ] && [ "${MEDIR_D5:-0}" = "1" ]; then
    # D5 (ADR-016): las llamadas salientes se miden durante el escalón de 10 req/s, que empieza
    # 90 s después del setup; d5.js tarda ~7 s en preparar sus solicitudes.
    (sleep 83 && k6 /scripts/d5.js > "$CARPETA/k6-d5.log" 2>&1) &
    pid_d5=$!
  fi
  k6 /scripts/traslados.js -e ESCENARIO="$escenario" 2>&1 | tee "$CARPETA/k6-$escenario.log"
  [ -n "$pid_d5" ] && wait "$pid_d5"
  kill "$pid_stats"
  wait "$pid_stats" 2>/dev/null || true
}

echo "== A: una réplica de Integración y sin caché"
docker compose stop integracion-2 memcached
correr A

echo "== B: una réplica de Integración y con caché"
docker compose start memcached
correr B

echo "== C: dos réplicas de Integración y con caché"
docker compose start integracion-2
correr C

echo "Listo. Resumen: node $CARPETA/resumir.mjs"
