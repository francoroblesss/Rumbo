# Logs y paneles de Grafana (final)

Corrida del **23/11/2026, 19:56–20:45 UTC** (16:56–17:45 en Córdoba), en Windows 11 con Docker Desktop. Volúmenes limpios (`docker compose down -v`), `COMPOSE_PROFILES=observabilidad`, `OTEL_EXPORTER_OTLP_ENDPOINT=http://lgtm:4318`, `LOGS_MYSQL_DSN` en `.env` y cupo alto (`FRANJA_CUPO_TRASLADOS=100000`). Se corrió con los dos PR de la final juntos (este y el del reintento de Integración, ADR-035), porque el panel 18 muestra la métrica que agrega aquel.

## Consulta por id con Memcached prendido y apagado (`cache.txt`, `k6-cache.txt`, `cache.png`)
`bash pruebas/carga/cache-encendida-apagada.sh`: 20 req/s de `GET /traslados/{id}` durante 9 min. Memcached se apagó a las 20:19:45 y se prendió a las 20:22:44. Las consultas a Prometheus son las del final de cada fase (ventana de 2 min).

| Fase | p95 hit (Memcached) | p95 miss (MongoDB) | p95 error (Memcached caído) | p95 HTTP `GET /traslados/{id}` | Reparto (op/s) |
| --- | --- | --- | --- | --- | --- |
| Prendido (min 3, 20:19:44) | **0,78 ms** | 2,4 ms | — | 4,75 ms | hit 19,7 · miss 0,3 · error 0 |
| Apagado (min 6, 20:22:44) | — | — | **242,5 ms** | 242,5 ms | hit 0 · miss 0 · error 20 |
| Prendido otra vez (min 9, 20:25:44) | **0,90 ms** | 5,7 ms | — | 4,75 ms | hit 19,7 · miss 0,3 · error 0 |

Lo que se ve, tal cual:
- **Con Memcached, la consulta tarda menos de 1 ms (p95). Sin Memcached (miss), MongoDB responde en 2,4–5,7 ms.** Con un índice por id MongoDB es rápido: la diferencia entre hit y miss existe (3 a 6 veces), pero son milisegundos.
- **Con Memcached caído, la consulta tarda ~242 ms**, 50 veces más que un miss. No es "caché apagada = MongoDB directo": cada consulta paga el timeout de la caché (100 ms, `TimeoutCache`) en el `Get` y otra vez en el `Set` que intenta guardar el resultado. El contenedor detenido no rechaza la conexión, así que se espera el timeout entero. Por eso, tener la caché caída es mucho peor que no tenerla (en Render, sin `MEMCACHED_ADDR`, se lee directo de MongoDB, ADR-026).
- **El consumidor no ve ningún error:** 10 821 pedidos, 0 fallas (`k6-cache.txt`). La degradación funciona: más lento, pero responde.
- Al volver Memcached, la primera consulta de cada id es un miss y después todo es hit otra vez.
- El p95 HTTP de la fase "prendido" (4,75 ms) es mayor que el de la caché (0,78 ms) porque incluye el resto del pedido (gin, identidad, JSON) y porque los buckets del histograma HTTP son más gruesos (el primero es 5 ms).
- En el panel 19 las barras `error` siguen un par de minutos después de prender Memcached: el panel usa `rate(...[5m])`.

**La primera corrida (20:06–20:16) salió vacía:** el script pedía `rate(...[1m])`, pero los servicios exportan las métricas por OTLP cada 60 s, así que en 1 min hay una sola muestra y `rate()` no devuelve nada (5 muestras en 5 min, verificado con `count_over_time`). Se cambió la ventana del script a 2 min (`VENTANA`) y se repitió (20:16–20:26). Los paneles usan `[5m]` y no tienen este problema. La primera corrida igual se ve en los paneles 16 y 19 (la primera meseta de ~240 ms).

## Logs (`logs.txt`)
- **Correlation id largo:** un `GET /disponibilidad` con un `X-Correlation-ID` de 100 caracteres dejó 3 logs (gateway, Integración y Operaciones) con el correlation id recortado a 64. Filtrando el panel 14 por esos 64 caracteres aparecen los tres (`panel-14-correlacion.png`). Antes del arreglo, MySQL rechazaba el lote entero.
- **Identidad:** aparece en la variable `servicio` y en el panel 14 ("request atendida" del login, "usuario sembrado", "servicio escuchando"). Están los seis servicios: `gateway`, `identidad`, `integracion` (con `integracion-1` e `integracion-2` en la columna `instancia`), `notificaciones`, `operaciones` y `recursos`.
- **Logs del arranque:** el primer log de cada servicio está en la tabla entre 0,5 y 2 s después de que arrancó su contenedor, con los reintentos de conexión a RabbitMQ y "migraciones aplicadas" incluidos.
- **`rumbo_logs_descartados_total{motivo="mysql"}`:**
  - **Reinicio sin `-v` (20:44): 0.**
  - **Arranque con volúmenes limpios: 6** (3 por réplica de Integración, a las 19:57:03 y 19:57:05, con `connection refused`). En la primera inicialización, la imagen de MySQL levanta un servidor temporal **sin red** (`port: 0`) para crear la base y correr `01-esquema.sql`. El healthcheck de `mysql-logs` (`mysqladmin ping -h localhost`, por socket) ya lo da por sano, y los servicios que esperan con `depends_on` arrancan. El servidor real empieza a escuchar en el 3306 recién a las 19:57:06. Las dos réplicas de Integración arrancan primero y sus primeros lotes se pierden.
  - **No se corrigió:** hace falta cambiar el healthcheck de `mysql-logs` (por ejemplo, `-h 127.0.0.1`, que va por TCP y falla con el servidor temporal), y esta tarea no lo permite. Queda para decidir.
  - El panel 13 tampoco muestra esos 6: el contador nace con su primer valor (3) y `increase()` no lo cuenta (ver "Contadores que nacen con su primer valor").

## Los 19 paneles (`tablero-1.png`, `tablero-2.png`, `tablero-3.png`)
Tráfico generado: login como operador, cliente y administrador con `GET /api/identidad/yo`; 3 reservas (una al aeropuerto), consultas y disponibilidad con el token del sistema externo; listados, franjas y avisos del operador y del cliente; un traslado `EN_CURSO` y `COMPLETADO`; búsquedas; el stream de avisos abierto 2 min; `POST /admin/reindexacion`; `reintentos.sh` y dos congelamientos manuales de Operaciones con reservas en curso; `cache-encendida-apagada.sh`; pedidos sin token y con un token inválido.

| Panel | ¿Muestra datos? | Con qué tráfico | Observación |
| --- | --- | --- | --- |
| 1. Requests por segundo | Sí | Todo | Las dos mesetas de 20 req/s son las pruebas de caché |
| 2. Errores 5xx (%) | Sí | Congelamientos de Operaciones | Picos de Operaciones (~5 % y ~50 %). Son los pedidos **originales** congelados: al volver, Operaciones los procesa con el contexto ya cancelado (Integración los abandonó por timeout y reintentó), responde `503` a nadie y lo loguea como `ERROR` ("iniciar transacción: … operation was canceled"). El reintento sí dio `201` y no hubo reservas duplicadas. **Hallazgo:** estos `503` inflan los 5xx de Operaciones; habría que tratar el contexto cancelado como "el cliente se fue" y no como error del servidor (es Operaciones, no se tocó) |
| 3. Latencia p95 (sin el stream) | Sí | Todo | Ya no aparece el stream de 2 min. El pico de ~2,4 s del gateway y de Integración (20:36–20:40) son las reservas que se reintentaron durante los congelamientos manuales |
| 4. Requests por instancia de Integración | Sí | Pruebas de caché y reservas | Las dos réplicas, parejas |
| 5. Hit ratio de las cachés | Sí | Pruebas de caché | Cerca del 100 %, **incluso con Memcached caído**: el panel calcula hit / (hit + miss) y excluye los `error`, así que la caída no se ve (para eso está el 19). Baja a ~75 % solo con poco tráfico (los miss de las consultas sueltas) |
| 6. Pendientes de outbox | Sí | Reservas | 0, con un 1 de paso |
| 7. Mensajes procesados | Sí | Reservas, cambios de estado | Todo `ok` |
| 8. Estado de los circuitos | Sí | — | `clima` y `operaciones` cerrados (los congelamientos fueron más cortos que lo que abre el breaker) |
| 9. Consultas de clima | **No** | Reserva al aeropuerto | El clima se consulta solo si la franja empieza dentro de las próximas 6 h (R-46). A la hora de la prueba (17:40 en Córdoba) no había ninguna franja reservable en ese rango: R-44 exige 2 h de anticipación y la última franja del día es a las 18 h, así que las reservas para hoy dieron `422` y la del aeropuerto quedó para mañana. No es un problema del panel: se ve con una reserva al aeropuerto entre las 8 y las 16 h. Además, un único evento no se vería por los contadores que nacen con su primer valor |
| 10. Rechazos del gateway | Sí | Pedidos sin token y con token inválido (20:38–20:41) | `no_autenticado` |
| 11. Errores recientes | Sí | Congelamientos | Los `ERROR` del panel 2 |
| 12. Logs por minuto | Sí | Todo | Los `WARN` de las caídas de Memcached ("la caché falló, se sigue con MongoDB") |
| 13. Logs descartados | Sí (en 0) | Arranque | Muestra 0, pero hubo 6 descartes al arrancar con volúmenes limpios (ver "Logs"). La serie nace en 3 e `increase()` da 0 |
| 14. Logs | Sí | Todo | Con `servicio = identidad` y con el correlation id largo (`panel-14-correlacion.png`) |
| 15. Demora de indexación | Sí | Reservas y reindexación | Debajo de 1 s |
| 16. Consulta por id según la caché | Sí | `cache-encendida-apagada.sh` | Nuevo. Ver la tabla de arriba |
| 17. Búsqueda p50 y p95 | Sí | Búsquedas sostenidas (una cada 5 s, 4 min) | Nuevo. p95 de ~70 ms al principio, que baja a ~10 ms. Con 4 búsquedas sueltas en un mismo minuto **no se veía nada**: la serie nació con esas 4 y `rate()` dio 0 |
| 18. Reintentos | Sí | `reintentos.sh` y congelamientos manuales | Nuevo. `operaciones · reserva` y `operaciones · disponibilidad`; `cancelacion` en 0. **Al principio mostraba 0** aunque había reintentos (contadores que nacen con su primer valor). Se arregló en el PR del reintento: los contadores de Integración arrancan en 0 |
| 19. Caché por resultado | Sí | `cache-encendida-apagada.sh` | Nuevo. Muestra los `error` de las dos caídas de Memcached, que el panel 5 no muestra |

### Contadores que nacen con su primer valor
Un contador de OpenTelemetry aparece en Prometheus recién con su primer `Add`, y aparece **con ese valor** (1, 3…). `increase()` y `rate()` no tienen una muestra anterior y no cuentan ese salto. Para métricas de volumen alto (pedidos HTTP) no se nota. Para eventos raros (reintentos, descartes de logs, consultas de clima, rechazos sueltos) el panel queda en 0 aunque el evento pasó. En `rumbo_reintentos_total` de Integración se resolvió inicializando las tres series en 0. El adaptador de clima de Operaciones (Llanca) cuenta en la misma métrica y tiene el mismo problema. `rumbo_logs_descartados_total` y `rumbo_clima_consultas_total` también. Se avisó; no se tocaron porque no son de esta tarea.

### Otros
- En los logs de Identidad aparecen dos `failed to upload metrics … broken pipe`: el exportador OTLP falló mientras `lgtm` arrancaba (se recuperó solo).
- El primer `docker compose up --wait` con volúmenes limpios cortó por "operaciones unhealthy" (Operaciones tarda en arrancar en esta máquina, también sin estos cambios). Un segundo `up -d --wait` levantó lo que faltaba.
