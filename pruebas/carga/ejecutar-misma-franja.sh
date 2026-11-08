#!/usr/bin/env bash
# Corre misma-franja.js en sus dos modos sobre el sistema ya levantado con cupo alto:
#   misma       todas las reservas sobre una franja
#   repartidas  control: las reservas rotan entre 173 franjas
# Por modo deja en pruebas/carga/: resultado-<modo>.json (k6), bloqueos-<modo>.txt (contadores de
# esperas por bloqueo de fila de InnoDB cada 2 s), stats-<modo>.txt (docker stats cada 5 s, como
# ejecutar.sh) y k6-<modo>.log. Para el modo "misma" deja además
# verificacion-misma.json: traslados_reservados de la franja antes y después.
# Después: node pruebas/carga/resumir-misma-franja.mjs
# Se ejecuta desde la raíz del repositorio: bash pruebas/carga/ejecutar-misma-franja.sh
# Solo en local: nunca contra la nube (ADR-026).
set -euo pipefail

# En Git Bash (Windows) evita que se reescriban las rutas que se le pasan a docker.
export MSYS_NO_PATHCONV=1
DIR_HOST="$(pwd -W 2>/dev/null || pwd)/pruebas/carga"
CARPETA="pruebas/carga"
PROYECTO="${COMPOSE_PROJECT_NAME:-$(basename "$PWD" | tr '[:upper:]' '[:lower:]')}"
RED="${RED:-${PROYECTO}_default}"
IMAGEN_K6="${IMAGEN_K6:-grafana/k6:1.3.0}" # la misma versión que la prueba de la Entrega 2

# variable lee un valor de .env (el mismo archivo que usa docker compose).
variable() { grep -E "^$1=" .env | tail -n 1 | cut -d= -f2-; }

MYSQL_ROOT_PASSWORD="$(variable MYSQL_ROOT_PASSWORD)"
CLIENT_ID="$(variable EXTERNO_CLIENT_ID)"
CLIENT_SECRET="$(variable EXTERNO_CLIENT_SECRET)"
SECRETO_INTERNO="$(variable SECRETO_INTERNO)"

# FRANJA: por defecto, la de 10 h de dentro de 3 días (hora de Córdoba), lejos de los datos de una
# demo. Se puede forzar con FRANJA=2026-11-20T10:00:00-03:00. FRANJA_UTC es cómo la guarda MySQL.
FRANJA="${FRANJA:-$(node -e 'const d=new Date(Date.now()-3*3600e3+3*86400e3);console.log(d.toISOString().slice(0,10)+"T10:00:00-03:00")')}"
FRANJA_UTC="$(node -e 'console.log(new Date(process.argv[1]).toISOString().slice(0,19).replace("T"," "))' "$FRANJA")"

mysql() {
  docker compose exec -T -e MYSQL_PWD="$MYSQL_ROOT_PASSWORD" mysql mysql -uroot -N -B -e "$1"
}

# franja devuelve "traslados_reservados<TAB>cupo_traslados" de FRANJA, o nada si todavía no tiene fila.
franja() {
  mysql "SELECT traslados_reservados, cupo_traslados FROM operaciones.franjas WHERE inicio = '$FRANJA_UTC'"
}

# Bloqueos: se usan los contadores de InnoDB Innodb_row_lock_* (SHOW GLOBAL STATUS). Acumulan
# cuántas veces una transacción esperó un bloqueo de fila y cuántos ms en total, que es lo que
# queremos ver del FOR UPDATE. performance_schema está apagado en compose.yaml para ahorrar memoria
# y SHOW ENGINE INNODB STATUS solo muestra las esperas de ese instante, no el acumulado.
muestrear_bloqueos() {
  local archivo="$1"
  : > "$archivo"
  while true; do
    local filas; filas=$(mysql "SHOW GLOBAL STATUS LIKE 'Innodb_row_lock%'" || true)
    printf -- '--- %s\n%s\n' "$(date +%s)" "$filas" >> "$archivo"
    sleep 2
  done
}

# muestrear_stats es el mismo muestreo de ejecutar.sh: una muestra de docker stats cada 5 s, en
# segundo plano porque tarda ~3 s. Solo los contenedores de este proyecto.
muestrear_stats() {
  local archivo="$1"
  : > "$archivo"
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
  docker run --rm --network "$RED" -v "$DIR_HOST:/scripts" "$IMAGEN_K6" run \
    -e FRANJA="$FRANJA" -e CLIENT_ID="$CLIENT_ID" -e CLIENT_SECRET="$CLIENT_SECRET" \
    -e SECRETO_INTERNO="$SECRETO_INTERNO" "$@"
}

correr() {
  local modo="$1"
  echo "== Modo $modo: esperando 10 s a que se estabilice"
  sleep 10
  muestrear_bloqueos "$CARPETA/bloqueos-$modo.txt" &
  local pid_bloqueos=$!
  muestrear_stats "$CARPETA/stats-$modo.txt" &
  local pid_stats=$!
  k6 /scripts/misma-franja.js -e MODO="$modo" 2>&1 | tee "$CARPETA/k6-$modo.log"
  sleep 3 # una muestra más después del último escalón
  kill "$pid_bloqueos" "$pid_stats"
  wait "$pid_bloqueos" "$pid_stats" 2>/dev/null || true
}

echo "== Franja de la prueba: $FRANJA ($FRANJA_UTC UTC)"
antes="$(franja)"
if [ -n "$antes" ] && [ "$(cut -f2 <<< "$antes")" -lt 100000 ]; then
  # La fila guarda el cupo con el que se creó: si es el de por defecto, la prueba se cortaría con 409.
  echo "La franja ya existe con cupo $(cut -f2 <<< "$antes"): elegí otra con FRANJA=..." >&2
  exit 1
fi
reservados_antes="$(cut -f1 <<< "${antes:-0}")"

correr misma
reservados_despues="$(franja | cut -f1)"
printf '{"franja": "%s", "traslados_reservados_antes": %s, "traslados_reservados_despues": %s}\n' \
  "$FRANJA" "${reservados_antes:-0}" "${reservados_despues:-0}" > "$CARPETA/verificacion-misma.json"

correr repartidas

echo "Listo. Resumen: node $CARPETA/resumir-misma-franja.mjs"
