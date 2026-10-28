# Práctico — Clase 8: Observabilidad (Prometheus, logs en MySQL, Grafana)

Fuente: repo del docente, `CLASE_8/` (`PRESENTACION/PRESENTACION.pdf`, `DEMO/`; sin actividades). Commits del 15 y 16/10/2026.

## Teoría de la presentación ("De la señal al diagnóstico")
- **Observabilidad:** recopilar y analizar información para entender el estado y el comportamiento del sistema (rutas que se vuelven lentas, más errores) aunque "responda bien la mayoría de las veces".
- **Métrica:** valor numérico en el tiempo (requests, % de error, duración, CPU/memoria, mensajes pendientes en una cola). Responde "¿aumentaron los errores?", "¿qué ruta tarda más?", "¿cuántas req/s?".
  - **Counter** (`requests_total`): solo sube. **Histogram** (`request_duration_seconds`): agrupa duraciones en cubetas → promedios y percentiles.
  - Etiquetas de **ruta y código HTTP** para separar 200 de 500.
- **Log:** registro de un evento concreto con mensaje, nivel y hora (`info operacion_exitosa`, `warn operacion_lenta`, `error operacion_fallida: fallo_simulado`).
- **Se complementan:** la métrica avisa ("20 respuestas 500 en un minuto"); el log explica el detalle. "MÉTRICA detecta una tendencia → LOG ayuda a investigar el evento".
- **CorrelationID / TrackID:** ID único asignado cuando la solicitud entra al sistema y propagado a todos los servicios y logs (ej. header `X-Correlation-ID: 7f3a9c`). Sirve para relacionar logs de distintos servicios, reconstruir el recorrido, diagnosticar errores y latencias, y que el usuario reporte su ID a soporte. **Buenas prácticas: generarlo en el primer punto de entrada si el cliente no lo envía, reutilizarlo si ya viene y devolverlo en la respuesta.**
- **Prometheus:** modelo de **scraping** (consulta `/metrics` periódicamente; en la demo cada 5 s) y almacena las series.
- **Grafana:** no genera métricas ni logs; consulta fuentes (Prometheus y MySQL) y las muestra en un tablero común.
- Pregunta de cierre: "si aumenta el número de errores en el gráfico, ¿qué información buscaríamos en los logs?".
- **No cubre** trazas distribuidas (solo se anunciaron en la Clase 7), alertas ni OpenTelemetry.

## Demo (`DEMO/`, módulo `observabilidad-demo`, go 1.22, Gin v1.10.0, client_golang v1.20.5, GORM v1.25.12 + driver mysql v1.5.7)
```
GET /ok, /slow, /error, /compra/... → API → contador e histograma → /metrics → Prometheus → Grafana
                                      └→ INSERT en app_logs → MySQL → Grafana
```
- `compose.yaml` (`name: clase8-observabilidad`): `app` (build, healthcheck con `wget /health`), `prometheus` (`prom/prometheus:v3.7.3`), `mysql` (`mysql:8.4`, init `001_schema.sql`, healthcheck que consulta la tabla), `grafana` (`grafana/grafana:latest`, admin/admin, provisioning de datasources y dashboard montado `:ro`). Volúmenes para los tres almacenamientos.
- `main.go`: `gin.SetMode(gin.ReleaseMode)`, `gin.New()` + `gin.Recovery()`. `/metrics` (`gin.WrapH(promhttp.Handler())`) y `/health` **fuera** del grupo medido "para no contar las sondas". Grupo `apiRoutes` con `observability.RecordMetrics`; subgrupo `/compra` con `observability.AddTrackID`. `log.Fatalf` si no conecta a MySQL al arrancar.
- `observability/metrics.go`: `promauto.NewCounterVec(requests_total, [route,status])` y `promauto.NewHistogramVec(request_duration_seconds, [route,status])`; middleware: `start := time.Now(); ctx.Next(); route := ctx.FullPath(); status := strconv.Itoa(ctx.Writer.Status())` → `Inc()` y `Observe(...)`. **Usa `FullPath()` (la plantilla de ruta) como etiqueta**, no la URL con IDs.
- `observability/track_id.go`: 16 bytes de `crypto/rand` → hex de 32 caracteres; `ctx.Set("track_id", id)` y header `X-Track-ID`. (Simplificación: siempre genera uno nuevo, no reutiliza el entrante.)
- `logs/mysql.go`: `Store` con GORM; `Log{TrackID, Stage, Level, Message}` en la tabla `app_logs` (`AutoMigrate` para agregar columnas); `Write` hace `Create` y, si MySQL falla, solo `log.Printf` (el evento se pierde).
- `controllers/compra.go`: una sola API **simula** las etapas `gateway` → `pedidos` → `pagos` escribiendo logs con el mismo `track_id` y distinta `stage`; `/compra/error` falla en pagos y `/compra/error-pedidos` en pedidos (no hay paso de pagos).
- `mysql/001_schema.sql`: `app_logs` con índices por `created_at`, `track_id`, `stage`; usuario `grafana_reader` con **solo `SELECT`** (Grafana lee, no escribe).
- `prometheus.yml`: job `demo-app`, target `app:8080` (nombre del servicio en Compose, no `localhost`), cada 5 s.
- Dashboard (`grafana/dashboards/demo.json`): solicitudes/s por ruta y estado, duración media, **% de fallos 5xx** (torta), **fallos por etapa** con `increase()` sobre el contador existente (sin métrica nueva), logs del recorrido; variable de texto `track_id` que filtra los paneles SQL con `${track_id:sqlstring}`.
- Consulta de logs en Grafana → Explore → MySQL Logs (formato Table): `SELECT created_at AS time, stage, track_id, message AS body, level FROM app_logs WHERE $__timeFilter(created_at) ORDER BY created_at DESC`.
- Comandos: `docker compose up --build -d`; `curl localhost:8080/{ok,slow,error,compra/ok,compra/error,compra/error-pedidos,metrics}`; Prometheus `http://localhost:9090/targets` (`demo-app` UP, `requests_total{job="demo-app"}`); Grafana `http://localhost:3000`; `docker compose down [-v]`.

## Cómo se relaciona con Rumbo
- Rumbo adopta el esquema de la clase desde [ADR-023](../../docs/adr/ADR-023-logs-en-mysql.md):
  - los logs se guardan en la tabla `logs` de un **MySQL aparte** (base `observabilidad`, no la del negocio);
  - Grafana los lee con un usuario de solo `SELECT` (`grafana_lector`);
  - las métricas siguen en Prometheus y las trazas en Tempo, vía OpenTelemetry (imagen LGTM).
- **Diferencia con la demo (gana el teórico 7):** la demo hace un `INSERT` sincrónico por log y la request espera a MySQL. Rumbo encola el log y lo inserta en lotes en segundo plano, con cola acotada y 2 conexiones. Si MySQL está lento o caído, el log se descarta (y se cuenta), pero la request no se frena.
- Coincide con la buena práctica del `X-Correlation-ID` (`servicios/comun/observabilidad`): el gateway lo genera si no viene, lo reutiliza si viene y lo devuelve.
- Igual que la demo, la métrica de duración usa la plantilla de ruta (`c.FullPath()`) como etiqueta, para no disparar la cardinalidad. A diferencia de la demo, `observabilidad.Instrumentar` también mide `/health` (las sondas cuentan en `rumbo_http_duracion_segundos`; se filtran por `ruta` en los paneles si molestan).
