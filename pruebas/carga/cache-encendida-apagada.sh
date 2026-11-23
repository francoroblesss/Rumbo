#!/usr/bin/env bash
# Consulta por id con Memcached prendido, apagado y prendido otra vez (ADR-007). Corre
# consulta-por-id.js (20 req/s de GET /traslados/{id} durante 9 min) y:
#   min 0      arranca k6 (Memcached prendido)
#   min 3      docker compose stop memcached
#   min 6      docker compose start memcached
#   cada 1 min consulta a Prometheus, a través de Grafana, el p95 de la consulta por resultado de la
#              caché (rumbo_cache_lectura_duracion_segundos) y las operaciones de la caché por
#              resultado (rumbo_cache_operaciones_total), con la hora UTC
# Todo queda en docs/evidencias/observabilidad-final/cache.txt; la salida de k6, en k6-cache.txt.
#
# Requisitos: el sistema levantado con el perfil de observabilidad y cupo alto
# (FRANJA_CUPO_TRASLADOS=100000), y Node.js 22 (para leer las respuestas de Prometheus).
# Se ejecuta desde la raíz del repositorio: bash pruebas/carga/cache-encendida-apagada.sh
set -uo pipefail

# En Git Bash (Windows) evita que se reescriban las rutas que se le pasan a docker.
export MSYS_NO_PATHCONV=1
DIR_HOST="$(pwd -W 2>/dev/null || pwd)/pruebas/carga"
EV="${EVIDENCIAS:-docs/evidencias/observabilidad-final}"
mkdir -p "$EV"
SALIDA="$EV/cache.txt"
: > "$SALIDA"
PROYECTO="${COMPOSE_PROJECT_NAME:-$(basename "$PWD" | tr '[:upper:]' '[:lower:]')}"
RED="${RED:-${PROYECTO}_default}"
IMAGEN_K6="${IMAGEN_K6:-grafana/k6:1.3.0}"
GRAFANA="${GRAFANA:-http://localhost:3000}"
# Secreto interno de .env (ADR-026): sin él Integración responde 401.
SECRETO_INTERNO=$(grep -E '^SECRETO_INTERNO=' .env | head -1 | cut -d= -f2-)

anotar() { echo "$(date -u +%Y-%m-%dT%H:%M:%SZ) $*" | tee -a "$SALIDA"; }

# prometheus <consulta>: consulta instantánea a Prometheus por la API de Grafana, una línea por serie.
prometheus() {
  echo "  consulta: $1"
  curl -s -u admin:admin -G --data-urlencode "query=$1" "$GRAFANA/api/datasources/proxy/uid/prometheus/api/v1/query" |
    node -e 'let t = ""; process.stdin.on("data", (d) => (t += d)).on("end", () => {
      const r = (JSON.parse(t || "{}").data || {}).result || [];
      console.log(r.length ? r.map((s) => `    ${JSON.stringify(s.metric)} = ${s.value[1]}`).join("\n") : "    (sin series)");
    });'
}

consultar() {
  {
    echo "### $(date -u +%Y-%m-%dT%H:%M:%SZ) · $1"
    prometheus 'histogram_quantile(0.95, sum by (le, resultado) (rate(rumbo_cache_lectura_duracion_segundos_seconds_bucket{cache="solicitudes"}[1m])))'
    prometheus 'sum by (resultado) (rate(rumbo_cache_operaciones_total{cache="solicitudes"}[1m]))'
    echo
  } >> "$SALIDA"
}

# esperar_hasta <segundos desde el inicio>
esperar_hasta() { local falta=$(( INICIO + $1 - $(date +%s) )); (( falta > 0 )) && sleep "$falta"; }

anotar "inicio de k6 (9 min, 20 req/s de GET /traslados/{id}), Memcached prendido"
INICIO=$(date +%s)
docker run --rm --network "$RED" -v "$DIR_HOST:/scripts" "$IMAGEN_K6" run -e SECRETO_INTERNO="$SECRETO_INTERNO" /scripts/consulta-por-id.js > "$EV/k6-cache.txt" 2>&1 &
K6=$!

for minuto in 1 2 3 4 5 6 7 8 9; do
  esperar_hasta $(( minuto * 60 ))
  case $minuto in
    3)
      consultar "min 3, Memcached prendido (justo antes de apagarlo)"
      anotar "MEMCACHED APAGADO: docker compose stop memcached"
      docker compose stop memcached
      ;;
    6)
      consultar "min 6, Memcached apagado (justo antes de prenderlo)"
      anotar "MEMCACHED PRENDIDO: docker compose start memcached"
      docker compose start memcached
      ;;
    *)
      if (( minuto < 3 )); then fase="prendido"; elif (( minuto < 6 )); then fase="apagado"; else fase="prendido otra vez"; fi
      consultar "min $minuto, Memcached $fase"
      ;;
  esac
done

wait "$K6"
anotar "fin de k6"
