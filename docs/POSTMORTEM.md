# Postmortem — Caída provocada

> Análisis de lo ocurrido durante la jornada de fallas controladas (consigna §3, §2.9).
> Caída provocada de **Operaciones** el **2026-11-08**, en local, con el perfil de observabilidad. Todas las horas son **UTC** (Argentina = UTC − 3). Solo se registra lo observado; la evidencia está en [`evidencias/postmortem/`](evidencias/postmortem/) y el procedimiento, en [`pruebas/fallas/`](../pruebas/fallas/README.md).

## Resumen
Con tráfico real por el gateway (0,4 reservas y 0,4 consultas por segundo durante 15 min), se detuvo el contenedor de Operaciones a las **18:33:06** y se lo volvió a arrancar a las **18:41:06** (8 min). Durante **8 min 13 s** las reservas respondieron `503 SERVICIO_NO_DISPONIBLE`; las consultas de traslados ya reservados siguieron respondiendo `200` todo el tiempo. Al volver, las reservas respondieron `201` a los 24 s del arranque. No hubo reservas duplicadas ni reservas hechas sin que el consumidor se enterara.

## Impacto
De [`resumen.md`](evidencias/postmortem/resumen.md) (k6, 728 pedidos a `/traslados`):

| Operación | `201` / `200` | `503` | `409` | Total |
| --- | --- | --- | --- | --- |
| Reservar (`POST /traslados`) | 166 | **199** | 2 | 367 |
| Consultar (`GET /traslados/{id}`) | 361 | 0 | 0 | 361 |

- **Reservas:** todas las de los minutos 3 a 10 dieron `503` (24 por minuto), desde la primera a las 18:33:14 hasta la última a las 18:41:27. La capacidad publicada no pudo tomar reservas durante 8 min 13 s.
- **Qué vio el consumidor:** `503` con `codigo: SERVICIO_NO_DISPONIBLE`, `Retry-After: 30` y el detalle "Reintentar con la misma Idempotency-Key" ([`retry-after.txt`](evidencias/postmortem/retry-after.txt); k6 no guarda headers, así que se verificó aparte con un pedido a las 18:47:14, con Operaciones detenido otra vez).
  - **162** de los 503 llegaron en milisegundos (p50 de 3 a 4 ms por minuto).
  - **37** tardaron **~2 s**, el timeout de Integración hacia Operaciones: 11 antes de que se abriera el breaker y 26 durante la caída, que coinciden con los pedidos de prueba del breaker en semiabierto (en los logs, cada paso a `half-open` vuelve a `open` 2 s después, cada ~30 s por réplica).
- **Siguió funcionando:** las 361 consultas por id respondieron `200` (p95 5 ms), desde la copia de Integración en MongoDB, sin llamar a Operaciones.
- **Sin duplicados ni reservas perdidas** ([`reservas.txt`](evidencias/postmortem/reservas.txt), cruce de todas las claves de k6 con MySQL de Operaciones):
  - las 168 claves que recibieron `201` (166 en k6 y 2 reintentos) tienen exactamente una fila en `claves_idempotencia` y una solicitud;
  - ninguna de las 196 claves que recibieron solo `503` dejó una reserva.
- **Los dos `409 SIN_DISPONIBILIDAD` de k6 no fueron por la caída:** los dos son de la franja 2026-11-11 10:00 (−03:00), que ya estaba llena (cupo 4, 4 reservados) desde la corrida de los tests de punta a punta del mismo día. El cupo alto (`FRANJA_CUPO_TRASLADOS=100000`) solo se aplica a las franjas que se crean después; esa ya existía en el volumen de MySQL.

## Línea temporal
| Hora (UTC) | Evento | Fuente |
| --- | --- | --- |
| 18:29:40 | Alertas cargadas, en Normal (sin datos) | Grafana, `activeAt` de las alertas |
| 18:30:06 | Arranca k6; primer pedido a las 18:30:10 | Paso 2 · `linea-temporal.txt`, `resumen.md` |
| 18:32:06 | Circuito cerrado (0) en las dos réplicas; alertas en Normal | Métrica `rumbo_circuito_estado` · `metricas.txt` |
| 18:32:40 | **"SLO latencia traslados" pasa a Pending, antes de la caída** (ver Detección) | Alerta |
| **18:33:06** | **`docker compose stop operaciones`** (detenido a las 18:33:10) | Paso 3 |
| 18:33:14.656 | Primera reserva con `503`, a los 2005 ms: `context deadline exceeded` hacia `operaciones:8082` | k6 · log `WARN "no se pudo llamar a Operaciones"` (`correlation_id 46fa6d53-…`) |
| 18:33:41.500 | Breaker de `integracion-1` hacia Operaciones: `closed` → `open` | Log `WARN "cambió el estado del circuit breaker"` |
| 18:33:42.653 | Primera reserva con `503` sin esperar el timeout (2 ms en el gateway, 1 ms en Integración, sin llamada a Operaciones) | k6 · log del gateway (`correlation_id d5087a93-…`) |
| 18:33:47.153 | Breaker de `integracion-2`: `closed` → `open` | Log |
| 18:34:15 | `rumbo_circuito_estado{dependencia="operaciones"}` = 2 (abierto) en las dos réplicas | Métrica (`query_range`, paso 15 s) |
| 18:34:12 → 18:41:07 | Cada ~30 s por réplica: `open` → `half-open` → 2 s después → `open` | Log (sección del breaker de `logs.txt`) |
| 18:35:00 | Primer punto con tasa de 5xx distinta de 0 en el gateway (ventana de 5 min) | Métrica (`query_range`, paso 60 s) |
| 18:35:40 | "SLO errores traslados" pasa a Pending | Alerta |
| 18:37:40 | "SLO latencia traslados" pasa a Alerting (Firing) | Alerta |
| **18:40:40** | **"SLO errores traslados" pasa a Alerting (Firing)**, 7 min 34 s después de la caída | Alerta |
| **18:41:06** | **`docker compose start operaciones`** | Paso 5 |
| 18:41:07.642 | Operaciones: `"servicio escuchando"` (antes, `"migraciones aplicadas"` y `"conectado a RabbitMQ"`) | Log `INFO` de Operaciones |
| 18:41:05 → 18:41:07 | `integracion-2` prueba en semiabierto justo antes de que Operaciones escuche: vuelve a `open` | Log |
| 18:41:14 | Operaciones sano (healthcheck) | `linea-temporal.txt` |
| 18:41:27.628 | Última reserva con `503` (3 ms, breaker todavía abierto) | k6 |
| 18:41:30.168 | Breaker de `integracion-1`: `half-open` → `closed`; Operaciones registra `"franja reservada"` (`correlation_id 1b10df22-…`) | Log |
| 18:41:30.177 | Primera reserva con `201` después de la caída (52 ms) | k6 |
| 18:41:37.653 | Breaker de `integracion-2`: `half-open` → `closed` | Log |
| 18:42:04 | La métrica muestra el circuito cerrado (0) | `linea-temporal.txt` |
| 18:42:05 | Reintento de 3 reservas que dieron `503`, con la misma clave: 2 × `201`, 1 × `409 SIN_DISPONIBILIDAD` | Paso 6 · `reintentos.txt` |
| 18:45:20 | Termina k6. Las dos alertas de SLO siguen en Alerting | Paso 7 · `metricas.txt` |
| 18:46:00 | La tasa de 5xx del gateway (ventana de 5 min) vuelve a 0 | Métrica (`query_range`) |

## Detección
Desde el inicio de la caída (18:33:06):

| Señal | Cuándo | Demora |
| --- | --- | --- |
| Breaker abierto, en los logs de Integración | 18:33:41 y 18:33:47 | 35 s y 41 s |
| Panel del circuito (`rumbo_circuito_estado` = 2) | 18:34:15 | 1 min 9 s |
| 5xx en el gateway (tablero) | 18:35:00 | ~2 min (resolución de 1 min) |
| Alerta "SLO errores traslados" en Pending | 18:35:40 | 2 min 34 s |
| **Alerta "SLO errores traslados" en Firing** | **18:40:40** | **7 min 34 s** |

- **Comparación con el SLO (ADR-014):** el objetivo es menos de 1 % de respuestas 5xx en 30 días. Durante la caída, la tasa de 5xx de `/traslados` en 5 min llegó al **50 %**; en los 15 min de la prueba fue del 27 % (199 de 728). La alerta disparó como está definida: tasa mayor a 1 % sostenida 5 min (`for: 5m`). Los 2 min 34 s hasta Pending salen de que se evalúa cada minuto y de que la ventana de 5 min tiene que acumular errores. Como referencia (no es tráfico real): con la tasa de esta prueba sostenida 30 días (~2,07 millones de pedidos), el presupuesto de error sería de ~20.700 respuestas 5xx, y esta caída de 8 min usó 199, cerca del 1 %.
- **La alerta "SLO latencia traslados" no sirvió:** pasó a Pending a las 18:32:40, **antes** de la caída, y a Firing a las 18:37:40. El histograma del gateway dio un p95 de **4,75 s** constante en todo el período, mientras que k6 midió reservas de 25 a 36 ms de mediana (p95 ≤ 281 ms, salvo los 503 por timeout). Causa, vista en el código: `rumbo_http_duracion_segundos` se registra en **segundos** sin límites de buckets propios (`comun/observabilidad`), y el SDK de OpenTelemetry usa por defecto límites pensados para milisegundos (0, 5, 10, 25, …). Toda request de menos de 5 s cae en el primer bucket, y `histogram_quantile` interpola 0,95 × 5 = 4,75. Con cualquier tráfico, esa alerta dispara.

## Respuesta
La respuesta fue la del procedimiento (arrancar el contenedor a los 8 min), no una reacción a la alerta. Desde `docker compose start operaciones` (18:41:06):

| Pieza | Listo a las | Tiempo |
| --- | --- | --- |
| Operaciones escuchando | 18:41:07.6 | 1,6 s |
| Operaciones sano (healthcheck) | 18:41:14 | 8 s |
| Breaker de `integracion-1` cerrado / primera reserva `201` | 18:41:30.2 | 24 s |
| Breaker de `integracion-2` cerrado | 18:41:37.7 | 32 s |
| Tasa de 5xx del gateway en 0 (ventana de 5 min) | 18:46:00 | ~5 min |

- Entre que Operaciones estuvo listo y la primera reserva `201` pasaron **22,6 s**, en los que hubo 7 reservas más con `503`: el breaker seguía abierto esperando sus 30 s antes de probar. `integracion-2` probó a las 18:41:05, 2 s antes de que Operaciones escuchara, y tuvo que esperar otro ciclo.
- **Paso 6, reintento con la misma `Idempotency-Key`** ([`reintentos.txt`](evidencias/postmortem/reintentos.txt)):
  - dos reservas que habían dado `503` (a las 18:37:20 y 18:41:27) dieron `201`, y cada clave tiene **una** fila en `claves_idempotencia`;
  - la tercera (503 a las 18:33:14) dio **`409 SIN_DISPONIBILIDAD`**: era para la franja 2026-11-11 10:00, la que ya estaba llena por los e2e. No se reservó nada ni quedó fila de la clave.

  El comportamiento es el del contrato (un `503` no reservó nada, y al reintentar la franja ya no tenía lugar), pero **no es lo que se esperaba** ("cada una da `201`"): la prueba arrancó con datos de una corrida anterior.

## Causa
- **Caída provocada del contenedor de Operaciones** (`docker compose stop`), que simula una caída del proceso. No hubo otra falla.
- **Por qué el impacto se limitó a las reservas** (ADR-008, ADR-021):
  - **Timeout de 2 s:** las llamadas a Operaciones no quedaron colgadas. Fallaron con `context deadline exceeded` o `request canceled while waiting for connection`, no con conexión rechazada, así que cada pedido esperaba los 2 s completos.
  - **Circuit breaker** (`sony/gobreaker`, 5 fallas consecutivas, 30 s abierto): a los 35–41 s dejó de llamar a Operaciones, y desde ahí el consumidor recibió el `503` en milisegundos, sin acumular pedidos esperando.
  - **Copia local en Integración (read model):** las consultas por id no dependen de Operaciones y siguieron en `200`.
  - **Idempotencia:** un `503` no deja una reserva a medias. El consumidor reintenta con la misma clave y obtiene una sola reserva (168 claves con `201`, 168 filas, 0 duplicadas).
- **Por qué las reservas sí cayeron:** desde ADR-021 la reserva es síncrona contra Operaciones (el cupo se decide en una transacción con `FOR UPDATE`, ADR-022). Hay una sola instancia de Operaciones, así que es un punto único de falla para reservar.

## Qué funcionó y qué no
**Funcionó:**
- `503 SERVICIO_NO_DISPONIBLE` con `Retry-After: 30`, honesto: no se aceptó ninguna reserva que no se pudiera cumplir (teórico 7: degradación sin mentir).
- El breaker se abrió en las dos réplicas, probó cada ~30 s en semiabierto y se cerró solo al volver Operaciones.
- Las consultas no se enteraron de la caída (361 de 361 en `200`).
- Cero reservas duplicadas o fantasma (cruce completo con MySQL).
- La alerta "SLO errores traslados" disparó según su definición.
- Los logs del período quedaron en el MySQL de logs con `correlation_id`, y se reconstruyó cada paso.
- Al inicio (18:30:20) hubo un `WARN` de Memcached en Integración ("la caché falló, se sigue con MongoDB"), sin relación con la caída: la consulta siguió desde MongoDB como prevé ADR-007.

**No funcionó o no se verificó:**
- La alerta de latencia dispara siempre, por los buckets del histograma (Detección). Tampoco sirve el p95 de latencia del tablero.
- La alerta de errores tardó **7 min 34 s**, cuando el breaker abierto ya decía lo que pasaba a los 35 s.
- Recuperación: 22,6 s de `503` con Operaciones ya listo, por la espera del breaker abierto. Es el costo conocido de ADR-008 ("mientras el breaker está abierto, también se rechazan pedidos que podrían haber funcionado").
- Uno de los tres reintentos dio `409` por datos de una corrida anterior: la prueba no arrancó con volúmenes limpios.
- **No se probó** `GET /disponibilidad` durante la caída: el tráfico de la prueba solo tenía reservas y consultas por id.
- Las consultas de la tasa de 5xx cada 2 min salieron vacías (ventana `[1m]` más corta que el intervalo de exportación). Se recuperaron después, con `query_range` y ventana de 5 min, sobre los datos guardados en Grafana (`metricas.txt`, última sección).

## Acciones de mejora
| # | Acción | Sale de | Dueño | Cuándo |
| --- | --- | --- | --- | --- |
| 1 | Límites de buckets explícitos en segundos para `rumbo_http_duracion_segundos` en `comun/observabilidad` (por ejemplo 0,005 a 5 s), para que el p95 y la alerta "SLO latencia traslados" midan algo | p95 constante de 4,75 s; alerta en Pending antes de la caída | Franco (`servicios/comun`) | Deuda: fuera de este PR, se propone antes de la Presentación |
| 2 | Alerta específica por breaker abierto (`max(rumbo_circuito_estado{dependencia="operaciones"}) == 2` durante 1 min), además de la del SLO | Breaker abierto a los 35 s, alerta del SLO a los 7 min 34 s | Franco (`infra/grafana`) | Deuda |
| 3 | Segunda réplica de Operaciones detrás del balanceador (la consistencia del cupo ya la da MySQL con `FOR UPDATE`, ADR-022; el costo, ADR-015) | 8 min sin reservas con una sola instancia | Grupo (decisión con ADR) | Deuda |
| 4 | Arrancar la prueba con volúmenes limpios (`docker compose down -v`), así todas las franjas toman el cupo alto | `409` en un reintento y en 2 reservas por una franja llena de los e2e | Sofia | **Ahora** (hecho: `pruebas/fallas/README.md`) |
| 5 | Consultas del orquestador con ventana de 5 min | Tasa de 5xx vacía cada 2 min | Sofia | **Ahora** (hecho: `pruebas/fallas/caida.sh`) |
| 6 | Agregar `GET /disponibilidad` al tráfico de la prueba | No se verificó su `503` durante la caída | Sofia | Deuda: próxima corrida |

## Evidencia
En [`docs/evidencias/postmortem/`](evidencias/postmortem/) (sin tokens ni secretos):
- [`linea-temporal.txt`](evidencias/postmortem/linea-temporal.txt): hora UTC de cada paso, anotada por `caida.sh`.
- [`resumen.md`](evidencias/postmortem/resumen.md): estados por minuto y por operación, hitos y duraciones; sale de [`k6.json`](evidencias/postmortem/k6.json), un punto por pedido. El resumen de k6 está en [`k6-salida.txt`](evidencias/postmortem/k6-salida.txt).
- [`metricas.txt`](evidencias/postmortem/metricas.txt): circuito, 5xx y alertas cada 2 min, y la consulta posterior por rango.
- [`logs.txt`](evidencias/postmortem/logs.txt): logs del MySQL de logs (usuario de solo lectura de Grafana), con el conteo por mensaje y los cambios del breaker.
- [`reintentos.txt`](evidencias/postmortem/reintentos.txt), [`reservas.txt`](evidencias/postmortem/reservas.txt) y [`retry-after.txt`](evidencias/postmortem/retry-after.txt).

Extractos de [`logs.txt`](evidencias/postmortem/logs.txt):
```
-- primera reserva fallida: espera el timeout de 2 s
18:33:14.655 WARN  integracion integracion-1 no se pudo llamar a Operaciones  46fa6d53-f4af-4d3c-9825-70c024212bc1
             {"ruta": "/internas/reservas", "error": "Post \"http://operaciones:8082/internas/reservas\": context deadline exceeded (Client.Timeout exceeded while awaiting headers)"}
18:33:14.656 ERROR gateway     request atendida  46fa6d53-f4af-4d3c-9825-70c024212bc1  {"ruta": "/traslados", "metodo": "POST", "duracion_ms": 2003, "estado_http": 503}
-- se abre el breaker
18:33:41.500 WARN  integracion integracion-1 cambió el estado del circuit breaker {"desde": "closed", "hacia": "open", "dependencia": "operaciones"}
-- con el breaker abierto: 503 en 2 ms, sin llamar a Operaciones
18:33:42.652 ERROR integracion integracion-1 request atendida  d5087a93-b916-4f5d-970b-41020f1816c0  {"ruta": "/traslados", "duracion_ms": 1, "estado_http": 503}
18:33:42.653 ERROR gateway     request atendida  d5087a93-b916-4f5d-970b-41020f1816c0  {"ruta": "/traslados", "duracion_ms": 2, "estado_http": 503}
-- vuelve Operaciones y se cierra el breaker
18:41:07.642 INFO  operaciones servicio escuchando {"puerto": "8082"}
18:41:30.168 WARN  integracion integracion-1 cambió el estado del circuit breaker {"desde": "half-open", "hacia": "closed", "dependencia": "operaciones"}
18:41:30.168 INFO  operaciones franja reservada  1b10df22-6d0a-4941-926e-a55a10fab800  {"solicitud_id": "sol-867f35cb-…", "franja_inicio": "2026-11-26T12:00:00-03:00"}
```
