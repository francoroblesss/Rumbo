# Tableros de Grafana

Archivos JSON de los tableros de Rumbo (D11). Se montan en el contenedor `lgtm`. Los paneles de logs (11, 12 y 14) consultan la fuente **Logs (MySQL)** (`uid: mysql-logs`, definida en `infra/grafana/fuente-logs-mysql.yaml`, ADR-023), con las variables `nivel`, `servicio` y `correlation_id`; el resto, Prometheus.
