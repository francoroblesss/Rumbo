# Evidencias — Presentación Grupal

Una sección por cada punto que pide la consigna para la Presentación Grupal (`contexto/proyecto/consigna.md`, "Miércoles 11 o viernes 13 de noviembre"). Cada una dice qué se verificó, cómo y dónde está el resultado. Las verificaciones de la Entrega 2 que siguen valiendo están en [`entrega-2.md`](entrega-2.md).

## 1. Frontend completo, desde el ingreso hasta la funcionalidad compartida
- **Qué hay:** aplicación web (`web/`, [ADR-029](../adr/ADR-029-aplicacion-web.md)) con ingreso, menú por rol y las pantallas de operador (franjas, traslados, avance de estado, cancelación, búsqueda, avisos), cliente de organización (sus traslados, cancelación, búsqueda, avisos) y administrador (además, reconstruir el índice). La funcionalidad compartida es la del flujo de la capacidad: lo que reserva el sistema externo por la API, el operador lo ve y lo avanza, y el sistema externo y el cliente ven el resultado.
- **Verificado:**
  - CI: job `Web` (lint con oxlint, build y 19 tests de `api.ts` y `sse.ts`).
  - Recorrido integrado por el gateway con las mismas llamadas que hace la web: [`presentacion/recorrido-integrado.txt`](presentacion/recorrido-integrado.txt).

## 2. Punto de entrada único con autenticación, direccionamiento y control de tráfico
- **Qué hay:** API Gateway (`servicios/gateway`): token firmado, ruteo por prefijo, límite de 60 pedidos por minuto por organización, correlation id, CORS de la web y secreto interno hacia los servicios ([ADR-026](../adr/ADR-026-despliegue-en-render.md)).
- **Verificado:**
  - Sin token → `401`, en local (test e2e `TestSinToken`) y en la nube ([`presentacion/render.txt`](presentacion/render.txt)).
  - Directo a Operaciones o a Integración en la nube, sin pasar por el gateway (con un `X-Rumbo-Roles: operador` falso) → `401` ([`render.txt`](presentacion/render.txt)).
  - Límite de tráfico: los tests e2e esperan el `Retry-After` de los `429` y repiten el mismo pedido (el sistema externo y el cliente comparten el cupo de su organización).

## 3. Al menos tres servicios operativos
Gateway, Identidad, Operaciones, Integración (dos réplicas) y Notificaciones intervienen en el flujo ([`recorrido-integrado.txt`](presentacion/recorrido-integrado.txt)): login y perfil (Identidad), reserva y avance (Integración y Operaciones), avisos (Notificaciones). Recursos está en esqueleto ([ADR-024](../adr/ADR-024-alcance-de-la-presentacion.md)).

## 4. Capacidad propia desplegada en la nube, con URL pública
- **URL:** **https://rumbo-gateway.onrender.com**, contrato 4.0.1 ([`contracts/README.md`](../contracts/README.md)), en Render ([ADR-026](../adr/ADR-026-despliegue-en-render.md)) con un monitor que la mantiene despierta ([ADR-030](../adr/ADR-030-monitor-externo-en-render.md)).
- **Verificado contra la nube** ([`presentacion/render.txt`](presentacion/render.txt)):
  - token `200`, disponibilidad `200`, reserva `201` `PROGRAMADO`, el mismo pedido con la misma `Idempotency-Key` → `201` con el mismo id, consulta `200` y cancelación `200`;
  - `401` sin token y sin el secreto interno;
  - arranque en frío medido y por qué hizo falta el monitor; con el monitor, 22 minutos sin ningún reinicio y `/salud` en menos de 1 s.
- Se le pasó al grupo consumidor la URL, el `client_id`, el `client_secret` (por privado) y el contrato.

## 5. Capacidad del proveedor incorporada al flujo principal
- **Qué hay:** el clima del aeropuerto ([ADR-012](../adr/ADR-012-consumo-clima.md), [ADR-020](../adr/ADR-020-contrato-clima-2.md), [ADR-025](../adr/ADR-025-clima-en-las-reservas.md)) se consulta después de reservar (asíncrono, cola `operaciones.clima`) y al pasar a `EN_CURSO` (síncrono): si el aeropuerto no es operable, el traslado queda con alerta climática, se ve en la web y Notificaciones avisa al operador y a la organización. Fail-open: si el proveedor falla, la alerta queda "clima no verificado" y la operación sigue.
- **Verificado:**
  - test de contrato con `kin-openapi` contra el contrato 2.1.0 fijado ([ADR-032](../adr/ADR-032-contrato-clima-2-1.md)) en [`contracts/consumidos/`](../contracts/consumidos/README.md) (`servicios/operaciones/adaptadores/clima/contrato_test.go`): ejemplos de respuesta, errores, validación de pedidos y breaker;
  - tests de los casos de uso de clima en reservas y al pasar a `EN_CURSO` (`aplicacion/solicitudes_test.go`, `TestRevisarClimaDeReserva` y `TestAvanzarEstado`).
  - contra el **servicio real** del proveedor (`https://clima-aeropuerto.onrender.com`, con la clave que nos asignaron el 10/11): sin clave responde `401`; el 10/11 respondió `503 WEATHER_SOURCE_UNAVAILABLE` con `Retry-After: 60` (lo medimos de 13:14 a 14:02 UTC; según el proveedor, hasta que el cupo se reinició a la medianoche UTC), porque su fuente (Open-Meteo, plan gratuito) le devolvía `429` por el cupo diario de su IP de salida, que Render comparte (diagnóstico del proveedor, en sus logs); el 11/11 a las 02:40 UTC, con el cupo reiniciado, respondió **`200`** en 0,66 s (SACO, `operable: true`). Nuestro adaptador trata ese `503` como falla del proveedor: la operación sigue con "clima no verificado" (fail-open), que es lo que el proveedor recomienda.
  - verificado de punta a punta el 11/11 a las 14:31 UTC, con Operaciones apuntando al servicio real y nuestra clave: una reserva al aeropuerto dio `201` y el consumidor de clima recibió `503` (su cupo se había vuelto a agotar), así que registró `clima revisado después de la reserva` con `resultado: no_verificado` y la reserva siguió normal (fail-open).

## 6. Al menos dos tipos de almacenamiento integrados al flujo
En una misma reserva ([`recorrido-integrado.txt`](presentacion/recorrido-integrado.txt)):
- **MySQL** (Operaciones): franja, solicitud, traslado, outbox y clave de idempotencia, en una transacción con bloqueo de la fila de la franja ([ADR-022](../adr/ADR-022-consistencia-y-concurrencia.md));
- **MongoDB**: la copia de la reserva en Integración (lo que consulta el sistema externo), los avisos en Notificaciones y los usuarios en Identidad.

## 7. Caché en un flujo de lectura relevante, con medición antes y después
Memcached con cache-aside sobre `GET /traslados/{id}` (lo que consulta el sistema externo para seguir su traslado), [ADR-007](../adr/ADR-007-cache.md). Medido con k6 en la Entrega 2 ([`capacidad/informe-entrega-2.md`](../capacidad/informe-entrega-2.md)), escalón de 100 req/s:

| Métrica | Sin caché | Con caché | Cambio |
| --- | ---: | ---: | ---: |
| p95 | 3,8 ms | 2,4 ms | −37 % |
| p99 | 6,3 ms | 4,1 ms | −35 % |
| CPU de `integracion-1` | 22,7 % | 13,5 % | −41 % |
| CPU de `mongo` | 11,0 % | 7,8 % | −29 % |

Hit ratio de 99,0 % (`rumbo_cache_operaciones_total`). El flujo cacheado no cambió desde entonces.

## 8. Dos estilos de arquitectura interna reconocibles
[ADR-010](../adr/ADR-010-estilos-internos.md), [`ARCHITECTURE.md`](../ARCHITECTURE.md) §3.1:
- **Hexagonal** en Operaciones: `dominio/` sin dependencias, `aplicacion/` con los casos de uso, `puertos/` y `adaptadores/` (`api`, `mysql`, `mensajeria`, `clima`, `solr`). Los casos de uso se testean en memoria con dobles de los puertos (`aplicacion/memoria_test.go`).
- **Capas** (controllers → services → repositories) en Identidad, Notificaciones, Integración y Recursos.

## 9. Evento de dominio publicado y consumido, con consumidor idempotente y mensajes no procesables
- **Evento:** `operaciones.solicitud.aceptada` (la reserva), publicado con outbox y consumido por cuatro colas: Integración, Notificaciones, clima e indexador ([`ARCHITECTURE.md`](../ARCHITECTURE.md) §5).
- **Idempotencia y DLQ, verificadas** (CA-42, PR #21 de Notificaciones): el mismo mensaje dos veces deja un solo aviso (`"aviso registrado"` y después `"aviso ya registrado"`), y un mensaje con datos inválidos va a `notificaciones.avisos.dlq` (de 0 a 1 mensaje, 0 reintentos). Tests con MongoDB real en `servicios/notificaciones/services/avisos_mongo_test.go`.
- **En el flujo integrado:** los avisos del operador y del cliente llegaron por el stream con 0,9 s de demora ([`recorrido-integrado.txt`](presentacion/recorrido-integrado.txt)).
- **Alerta por DLQ:** regla `Mensajes en DLQ` en Grafana ([ADR-016](../adr/ADR-016-validacion-d5.md)).

## 10. Búsqueda indexada con paginación, filtros, orden y actualización o reconstrucción del índice
- **Qué hay:** Apache Solr 9 con el core `traslados`, en Operaciones (F-27, [ADR-011](../adr/ADR-011-busqueda.md), `interfaces-internas.md` §16, [`ARCHITECTURE.md`](../ARCHITECTURE.md) §4 y §7):
  - `GET /api/operaciones/traslados/busqueda` con texto libre (análisis en español), filtros por organización, estado, rango de fechas y alerta climática, orden por fecha o relevancia, y paginación;
  - el índice se actualiza por eventos (cola `operaciones.indexador`, versión monotónica) y se reconstruye desde MySQL con `POST /api/operaciones/admin/reindexacion`.

  En la web: `/buscar` (operador y cliente) y `/admin/busqueda` (administrador).
- **Verificado con tests contra Solr real** (testcontainers, en el CI; `servicios/operaciones/adaptadores/solr/solr_test.go`):
  - el esquema se aplica dos veces sin error;
  - "CAJA" encuentra "Cajas" y "mercaderia" encuentra "mercadería";
  - relevancia, filtros, paginación con total, y un traslado sin franja queda último por fecha;
  - cada organización ve solo lo suyo, incluso con un intento de inyección en el filtro (CA-25);
  - un evento viejo no pisa uno nuevo, y 10 escrituras concurrentes del mismo documento (versiones 4 a 13) dejan la 13.
- **Verificado con el sistema levantado, por el gateway** (PR #29):
  - **Reindexación:** como operador da `403`; como administrador, `202`, y en el log `"reindexación terminada"` con 12 505 traslados en ~61 s.
  - **Demora de indexación:** una reserva nueva aparece en la búsqueda a los **0,64 s** (`demora_segundos` 0,51; objetivo ≤ 5 s, panel 15).
  - **Aislamiento (CA-25):** el cliente que pide otra organización ve solo la suya. `estado=ASIGNADO` → `422`; sin token → `401`.
  - **Solr caído (CA-43):** con `docker compose stop solr`, la búsqueda responde `503`, pero reservar da `201` y avanzar a `EN_CURSO` da `200`. Al volver Solr, el traslado aparece `EN_CURSO` a los 7,1 s, porque lo aplica un reintento del indexador.

## 11. Verificación de las decisiones con evidencia del sistema

| Decisión | Evidencia |
| --- | --- |
| Consistencia de la reserva (D4, [ADR-022](../adr/ADR-022-consistencia-y-concurrencia.md)) | Test con MySQL real: 20 reservas simultáneas sobre 3 lugares dan 3. Prueba de carga sobre una sola franja: 6249 `201` y 6249 lugares ocupados, sin errores, hasta 100 reservas por segundo ([`capacidad/informe-presentacion.md`](../capacidad/informe-presentacion.md)) |
| Capacidad y principal límite (D13, [ADR-015](../adr/ADR-015-capacidad-y-costos.md)) | Mismo informe: p95 ≤ 62 ms a 100 reservas/s; principal límite `mysql` (21,5 % de un núcleo) |
| Resiliencia ante la caída de Operaciones (D10, [ADR-008](../adr/ADR-008-resiliencia.md), [ADR-021](../adr/ADR-021-capacidad-de-autoservicio.md)) | [`POSTMORTEM.md`](../POSTMORTEM.md): 8 min de caída provocada, `503` con `Retry-After`, breaker abierto a los 35 s, consultas en `200` todo el tiempo, cero reservas duplicadas o perdidas; evidencia en [`postmortem/`](postmortem/). Misma caída con `GET /disponibilidad` en el tráfico (acción de mejora 6): `503` con `Retry-After` y `200` 9 s después de volver ([`caida-disponibilidad/`](caida-disponibilidad/README.md)) |
| Balanceo y detección de réplicas caídas (D12, [ADR-009](../adr/ADR-009-balanceo.md), [ADR-031](../adr/ADR-031-balanceador-solo-fallas-de-replica.md)) | Caída de Operaciones con el balanceador corregido: de 90 `502` a 0, y `GET /traslados/{id}` con 192 `200` de 192; con `integracion-2` detenida, 40 de 40 pedidos en `200` y todo atendido por `integracion-1` ([`caida-balanceador/`](caida-balanceador/README.md)) |
| Tests (consigna §2.11) | **Unitarios sobre las reglas de negocio:** cobertura de **98,1 %** en `operaciones/dominio` y **91,5 %** en `operaciones/aplicacion`, medida el 09/11 con `go test -short -cover ./dominio/... ./aplicacion/...` en `servicios/operaciones`. **De integración contra dependencias reales** (testcontainers, en el CI): Operaciones con MySQL y Solr; Identidad y Notificaciones con MongoDB; Integración con MongoDB y Memcached; `comun` con MySQL y RabbitMQ (el gateway no tiene almacenamiento propio). **De carga:** [`capacidad/`](../capacidad/) |
| Flujo de punta a punta | Job `e2e` del CI ([ADR-028](../adr/ADR-028-tests-de-punta-a-punta.md)): flujo principal, idempotencia, franja llena, cancelación y sin token, en cada PR |
| Búsqueda (D6, [ADR-011](../adr/ADR-011-busqueda.md)) | §10: tests con Solr real; demora de indexación de 0,51 s (objetivo ≤ 5 s); `503` solo en la búsqueda con Solr caído (CA-43); reindexación de 12 505 traslados en ~61 s |
| Tiempo real (bonus, [ADR-027](../adr/ADR-027-avisos-en-tiempo-real.md), CA-41) | `demora_ms` 912–935 en el stream ([`recorrido-integrado.txt`](presentacion/recorrido-integrado.txt)) |
| Despliegue ([ADR-026](../adr/ADR-026-despliegue-en-render.md), [ADR-030](../adr/ADR-030-monitor-externo-en-render.md)) | [`presentacion/render.txt`](presentacion/render.txt) |
| Observabilidad (D11, [ADR-014](../adr/ADR-014-observabilidad.md)) | Traza, logs correlacionados y balanceo de la Entrega 2 en [`entrega-2.md`](entrega-2.md); logs y métricas de la caída en [`postmortem/`](postmortem/). p95 de latencia real con los buckets en segundos: de 4,75 s constante a 0,236 s (gateway) con el mismo tráfico (acción de mejora 1 del postmortem). Alertas nuevas (acción de mejora 2): `Circuito abierto` en *Firing* ~1 min 40 s después de abrirse el breaker (la del SLO había tardado 7 min 34 s) y `Outbox sin publicar` ~3 min 55 s después de reservar con RabbitMQ caído (PR #32) |

## 12. Limitaciones conocidas y deuda técnica aceptada
En [`ARCHITECTURE.md`](../ARCHITECTURE.md) §10, actualizada para la Presentación: alcance recortado, Recursos en esqueleto, punto único de falla para reservar, alertas, cuota de Render y arranque en frío, nube sin caché ni búsqueda, índice atrasado después de una caída larga de Solr (se recupera reindexando), stream que consulta MongoDB cada segundo y el clima real, que depende del cupo diario de la fuente del proveedor.
