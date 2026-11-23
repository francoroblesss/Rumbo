#!/usr/bin/env bash
# Reintento de Integración a Operaciones (ADR-035): congela Operaciones 1,5 s tres veces mientras
# hay tráfico real por el gateway y muestra que el consumidor no se entera. Anota la hora UTC de
# cada paso en docs/evidencias/reintentos-integracion/linea-temporal.txt:
#   min 0        arranca k6 (4 min del mismo tráfico que la caída: reservas, consultas y disponibilidad)
#   min 1, 2, 3  docker compose pause operaciones, 1,5 s, docker compose unpause operaciones
#   al terminar  reintentos por operación y estado del breaker (Prometheus por la API de Grafana),
#                los WARN "reintento a Operaciones" y "no se pudo llamar a Operaciones" del MySQL de
#                logs y el resumen de k6
#
# Con el proceso congelado, el primer intento vence a 1 s, el segundo sale a ~1,1–1,3 s y puede
# volver a vencer, y el tercero sale después del unpause y responde. El pedido congelado se procesa
# al volver (timeout = resultado desconocido, teórico 7): la Idempotency-Key evita la reserva doble,
# y lo comprueba verificar-reservas.sh.
#
# Requisitos: el sistema levantado con el perfil de observabilidad y cupo alto (ver README.md).
# Se ejecuta desde la raíz del repositorio: bash pruebas/fallas/reintentos.sh
set -uo pipefail
export MSYS_NO_PATHCONV=1

EV="${EVIDENCIAS:-docs/evidencias/reintentos-integracion}"
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

# esperar_hasta <segundos desde el inicio>
esperar_hasta() { local falta=$(( INICIO + $1 - $(date +%s) )); (( falta > 0 )) && sleep "$falta"; }

anotar "inicio de k6 (4 min de tráfico por el gateway)"
INICIO=$(date +%s)
docker run --rm --user "$(id -u):$(id -g)" --network "$RED" -v "$PWD:/trabajo" "$IMAGEN_K6" run -q \
  -e CLIENT_ID="$CLIENT_ID" -e CLIENT_SECRET="$CLIENT_SECRET" -e DURACION=4m \
  --out json=/trabajo/$EV/k6.json /trabajo/pruebas/fallas/caida-operaciones.js > "$EV/k6-salida.txt" 2>&1 &
K6=$!

for minuto in 1 2 3; do
  esperar_hasta $(( minuto * 60 ))
  anotar "PAUSA $minuto: docker compose pause operaciones"
  docker compose pause operaciones
  sleep 1.5
  docker compose unpause operaciones
  anotar "VUELTA $minuto: docker compose unpause operaciones"
done

wait "$K6"
anotar "fin de k6"

# Los servicios exportan las métricas por OTLP cada 60 s: se espera una exportación antes de consultarlas.
sleep 70
{
  echo "### $(date -u +%Y-%m-%dT%H:%M:%SZ) · al terminar k6"
  prometheus 'sum by (operacion) (increase(rumbo_reintentos_total{dependencia="operaciones"}[10m]))'
  prometheus 'max(rumbo_circuito_estado{dependencia="operaciones"})'
} >> "$METRICAS"
anotar "métricas en $METRICAS"

# Logs del MySQL de logs (ADR-023), con el usuario de solo lectura de Grafana, como logs.sh.
hora() { grep "$1" "$LINEA" | head -1 | cut -d' ' -f1 | sed 's/T/ /; s/Z//'; }
DESDE=$(hora 'inicio de k6')
HASTA=$(hora 'fin de k6')
consulta() {
  docker compose exec -T mysql-logs mysql --default-character-set=utf8mb4 -ugrafana_lector -p"$GRAFANA_LOGS_DB_PASSWORD" observabilidad --table -e "$1" 2>/dev/null
}
FILTRO="creado_en BETWEEN '$DESDE' AND '$HASTA' AND servicio = 'integracion' AND mensaje IN ('reintento a Operaciones', 'no se pudo llamar a Operaciones')"
{
  echo "Logs del MySQL de logs (ADR-023) entre $DESDE y $HASTA (inicio y fin de k6), horas en UTC."
  echo
  echo "## Conteo por mensaje e instancia"
  consulta "SELECT mensaje, instancia, COUNT(*) AS cantidad, MIN(creado_en) AS primero, MAX(creado_en) AS ultimo FROM logs
    WHERE $FILTRO GROUP BY mensaje, instancia ORDER BY primero;"
  echo
  echo "## Detalle"
  consulta "SELECT creado_en, nivel, instancia, mensaje, correlation_id, LEFT(atributos, 400) AS atributos FROM logs
    WHERE $FILTRO ORDER BY creado_en;"
} > "$EV/logs.txt"
anotar "logs extraídos a $EV/logs.txt"

# En esta prueba el consumidor no reintenta a mano: verificar-reservas.sh lee este archivo igual.
echo "(sin reintentos manuales del consumidor: los reintentos los hace Integración, ver metricas.txt y logs.txt)" > "$EV/reintentos.txt"

node pruebas/fallas/resumir.mjs "$EV/k6.json" "$EV/resumen.md" > /dev/null
anotar "resumen de k6 en $EV/resumen.md"
