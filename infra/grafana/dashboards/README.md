# Tableros de Grafana

Archivos JSON de los tableros de Rumbo (D11). Se montan en el contenedor `lgtm`. Los paneles de logs (11, 12 y 14) consultan la fuente **Logs (MySQL)** (`uid: mysql-logs`, definida en `infra/grafana/fuente-logs-mysql.yaml`, ADR-023), con las variables `nivel`, `servicio` y `correlation_id`; el resto, Prometheus.

Los paneles 16 a 19 muestran la consulta por id según la caché (16: latencia p95 de hit, miss y error junto a la del pedido HTTP; 19: operaciones de la caché por resultado, incluidos los errores con Memcached caído), la latencia p50 y p95 de la búsqueda de Solr (17) y los reintentos de llamadas salientes por dependencia y operación (18: Integración → Operaciones, ADR-035, y Operaciones → clima, ADR-034). El panel 3 excluye el stream de avisos (`ruta` terminada en `/stream`), que dura lo que la pestaña está abierta.
