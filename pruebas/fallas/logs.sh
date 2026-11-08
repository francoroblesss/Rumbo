#!/usr/bin/env bash
# Logs de la caída provocada desde el MySQL de logs (ADR-023), con el usuario de solo lectura de
# Grafana: WARN y ERROR de Integración y del gateway, e INFO de Operaciones desde que volvió. El
# período sale de la línea temporal (del inicio al fin de k6). Lo llama caida.sh; también se puede
# correr solo (con mysql-logs levantado), desde la raíz:
#   bash pruebas/fallas/logs.sh docs/evidencias/postmortem/linea-temporal.txt > docs/evidencias/postmortem/logs.txt
set -euo pipefail
LINEA="$1"
hora() { grep "$1" "$LINEA" | head -1 | cut -d' ' -f1 | sed 's/T/ /; s/Z//'; }
DESDE=$(hora 'inicio de k6')
VUELTA=$(hora 'RECUPERACIÓN')
HASTA=$(hora 'fin de k6')
PASS=$(grep -E '^GRAFANA_LOGS_DB_PASSWORD=' .env | cut -d= -f2-)
consulta() {
  docker compose exec -T mysql-logs mysql --default-character-set=utf8mb4 -ugrafana_lector -p"$PASS" observabilidad --table -e "$1" 2>/dev/null
}
PERIODO="creado_en BETWEEN '$DESDE' AND '$HASTA'"

echo "Logs del MySQL de logs (ADR-023) entre $DESDE y $HASTA (inicio y fin de k6), horas en UTC."
echo
echo "## Conteo por servicio, nivel y mensaje"
consulta "SELECT servicio, nivel, mensaje, COUNT(*) AS cantidad, MIN(creado_en) AS primero, MAX(creado_en) AS ultimo FROM logs
  WHERE $PERIODO AND ((servicio IN ('integracion','gateway') AND nivel IN ('WARN','ERROR')) OR (servicio = 'operaciones' AND creado_en >= '$VUELTA'))
  GROUP BY servicio, nivel, mensaje ORDER BY primero;"
echo
echo "## Cambios de estado del circuit breaker (Integración → Operaciones)"
consulta "SELECT creado_en, instancia, correlation_id, atributos FROM logs
  WHERE $PERIODO AND servicio = 'integracion' AND mensaje LIKE '%circuit breaker%' ORDER BY creado_en;"
echo
echo "## WARN y ERROR de integracion y gateway"
consulta "SELECT creado_en, nivel, servicio, instancia, mensaje, correlation_id, LEFT(atributos, 400) AS atributos FROM logs
  WHERE $PERIODO AND servicio IN ('integracion','gateway') AND nivel IN ('WARN','ERROR') ORDER BY creado_en;"
echo
echo "## INFO de operaciones al volver (sin los 'request atendida')"
consulta "SELECT creado_en, nivel, servicio, mensaje, correlation_id, LEFT(atributos, 300) AS atributos FROM logs
  WHERE creado_en BETWEEN '$VUELTA' AND '$HASTA' AND servicio = 'operaciones' AND nivel = 'INFO' AND mensaje <> 'request atendida' ORDER BY creado_en LIMIT 60;"
