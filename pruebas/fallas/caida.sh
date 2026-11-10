#!/usr/bin/env bash
# Caída provocada de Operaciones (consigna §2.9, docs/POSTMORTEM.md). Corre el procedimiento con
# los minutos fijos y anota la hora UTC de cada paso en docs/evidencias/postmortem/linea-temporal.txt:
#   min 0      arranca k6 (15 min de tráfico por el gateway)
#   min 3      docker compose stop operaciones (la caída provocada)
#   min 3–11   Operaciones caído; cada 2 min se consultan por la API de Grafana el estado del
#              circuito, la tasa de 5xx por servicio y el estado de las alertas
#   min 11     docker compose start operaciones
#   después    se espera el breaker cerrado y se reintentan 3 reservas que dieron 503 con su misma
#              Idempotency-Key; se cuenta en MySQL una fila por clave
#   min 15     termina k6; se sacan los logs del MySQL de logs y el resumen de k6
#
# Requisitos: el sistema levantado con el perfil de observabilidad y cupo alto (ver README.md).
# Se ejecuta desde la raíz del repositorio: bash pruebas/fallas/caida.sh
set -uo pipefail
export MSYS_NO_PATHCONV=1

EV="${EVIDENCIAS:-docs/evidencias/postmortem}"
mkdir -p "$EV"
LINEA="$EV/linea-temporal.txt"
METRICAS="$EV/metricas.txt"
: > "$LINEA"
: > "$METRICAS"
PROYECTO="${COMPOSE_PROJECT_NAME:-$(basename "$PWD" | tr '[:upper:]' '[:lower:]')}"
RED="${RED:-${PROYECTO}_default}"
IMAGEN_K6="${IMAGEN_K6:-grafana/k6:1.3.0}"
GRAFANA="${GRAFANA:-http://localhost:3000}"

# Solo las variables que hacen falta (la línea con espacios de .env no se puede hacer source).
variable() { grep -E "^$1=" .env | head -1 | cut -d= -f2-; }
CLIENT_ID=$(variable EXTERNO_CLIENT_ID)
CLIENT_SECRET=$(variable EXTERNO_CLIENT_SECRET)
OPERACIONES_DB_PASSWORD=$(variable OPERACIONES_DB_PASSWORD)
GRAFANA_LOGS_DB_PASSWORD=$(variable GRAFANA_LOGS_DB_PASSWORD)

anotar() { echo "$(date -u +%Y-%m-%dT%H:%M:%SZ) $*" | tee -a "$LINEA"; }

# prometheus <consulta>: consulta instantánea a Prometheus por la API de Grafana, una línea por serie.
prometheus() {
  echo "  consulta: $1"
  curl -s -u admin:admin -G --data-urlencode "query=$1" "$GRAFANA/api/datasources/proxy/uid/prometheus/api/v1/query" |
    python3 -c 'import json,sys
r = json.load(sys.stdin).get("data", {}).get("result", [])
print("    (sin series)" if not r else "\n".join("    %s = %s" % ({k: v for k, v in s["metric"].items() if k not in ("__name__", "instance")}, s["value"][1]) for s in r))'
}

# alertas: estado de las alertas de Grafana (Normal, Pending o Firing) con su activeAt.
alertas() {
  echo "  alertas (/api/prometheus/grafana/api/v1/alerts):"
  curl -s -u admin:admin "$GRAFANA/api/prometheus/grafana/api/v1/alerts" |
    python3 -c 'import json,sys
a = json.load(sys.stdin).get("data", {}).get("alerts", [])
print("    (ninguna activa: todas en Normal)" if not a else "\n".join("    %s: %s desde %s" % (x["labels"].get("alertname"), x["state"], x.get("activeAt")) for x in a))'
}

consultar_grafana() {
  {
    echo "### $(date -u +%Y-%m-%dT%H:%M:%SZ) · $1"
    prometheus 'rumbo_circuito_estado'
    # Ventana de 5 min, como la alerta: con [1m] no hay dos muestras (las métricas se exportan por
    # OTLP cada pocos segundos a minutos) y la consulta sale vacía.
    prometheus 'sum by (job) (rate(rumbo_http_duracion_segundos_seconds_count{estado=~"5.."}[5m]))'
    prometheus 'sum by (job, estado) (rate(rumbo_http_duracion_segundos_seconds_count{job="gateway",ruta="/traslados"}[5m]))'
    alertas
    echo
  } >> "$METRICAS"
  anotar "consulta a Grafana: $1"
  free -h | sed -n 2p # control de memoria (no va a la evidencia)
}

# esperar_hasta <segundos desde el inicio>
esperar_hasta() { local falta=$(( INICIO + $1 - $(date +%s) )); (( falta > 0 )) && sleep "$falta"; }

anotar "inicio de k6 (15 min de tráfico por el gateway)"
INICIO=$(date +%s)
docker run --rm --user "$(id -u):$(id -g)" --network "$RED" -v "$PWD:/trabajo" "$IMAGEN_K6" run -q \
  -e CLIENT_ID="$CLIENT_ID" -e CLIENT_SECRET="$CLIENT_SECRET" \
  --out json=/trabajo/$EV/k6.json /trabajo/pruebas/fallas/caida-operaciones.js > "$EV/k6-salida.txt" 2>&1 &
K6=$!

esperar_hasta 120
consultar_grafana "min 2, antes de la caída"

esperar_hasta 180
anotar "CAÍDA PROVOCADA: docker compose stop operaciones"
docker compose stop operaciones
anotar "Operaciones detenido"

for minuto in 3 5 7 9; do
  esperar_hasta $(( minuto * 60 + 20 ))
  consultar_grafana "min $minuto, Operaciones caído"
done

esperar_hasta 660
consultar_grafana "min 11, Operaciones caído (justo antes de volver)"
anotar "RECUPERACIÓN: docker compose start operaciones"
docker compose start operaciones
# Se espera el healthcheck del mismo contenedor (un "up" lo recrearía sin el cupo alto).
until [[ "$(docker inspect -f '{{.State.Health.Status}}' "$(docker compose ps -q operaciones)")" == "healthy" ]]; do sleep 2; done
anotar "Operaciones sano (healthcheck)"

# El breaker pasa a semiabierto a los 30 s de abrirse y se cierra con la primera llamada buena.
for _ in $(seq 1 36); do
  estado=$(curl -s -u admin:admin -G --data-urlencode 'query=max(rumbo_circuito_estado{dependencia="operaciones",job="integracion"})' \
    "$GRAFANA/api/datasources/proxy/uid/prometheus/api/v1/query" | python3 -c 'import json,sys; r=json.load(sys.stdin)["data"]["result"]; print(r[0]["value"][1] if r else "?")')
  [[ "$estado" == "0" ]] && { anotar "breaker de Integración hacia Operaciones cerrado (rumbo_circuito_estado = 0)"; break; }
  sleep 5
done

anotar "reintento de 3 reservas que dieron 503, con la misma Idempotency-Key"
CLIENT_ID="$CLIENT_ID" CLIENT_SECRET="$CLIENT_SECRET" node pruebas/fallas/reintentar.mjs 3 "$EV/k6.json" | tee "$EV/reintentos.txt"
CLAVES=$(grep '^CLAVES=' "$EV/reintentos.txt" | cut -d= -f2 | sed "s/,/','/g")
{
  echo
  echo "Filas en operaciones.claves_idempotencia por clave reintentada (endpoint reserva):"
  docker compose exec -T mysql mysql --default-character-set=utf8mb4 -uoperaciones -p"$OPERACIONES_DB_PASSWORD" operaciones --table -e \
    "SELECT clave, COUNT(*) AS filas, MAX(estado_http) AS estado_http FROM claves_idempotencia WHERE endpoint = 'reserva' AND clave IN ('$CLAVES') GROUP BY clave;" 2>/dev/null
} | tee -a "$EV/reintentos.txt"

esperar_hasta 780
consultar_grafana "min 13, recuperación"

wait "$K6"
anotar "fin de k6"
consultar_grafana "min 15, fin del tráfico"

bash pruebas/fallas/logs.sh "$LINEA" > "$EV/logs.txt"
anotar "logs extraídos a $EV/logs.txt"

node pruebas/fallas/resumir.mjs "$EV/k6.json" "$EV/resumen.md" > /dev/null
anotar "resumen de k6 en $EV/resumen.md"
