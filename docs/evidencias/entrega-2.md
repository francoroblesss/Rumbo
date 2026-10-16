# Evidencia de observabilidad — Entrega 2

Evidencia de que el sistema se puede seguir sin inspeccionar cada componente a mano ([ADR-014](../adr/ADR-014-observabilidad.md)): una request real a través del gateway, su traza distribuida, sus logs correlacionados y el balanceo. Se obtuvo el 2026-10-16 con `docker compose up --build -d` y el perfil `observabilidad` (Grafana LGTM). Tempo, Loki y Prometheus se consultaron por la API de Grafana (`/api/datasources/proxy/uid/<tempo|loki|prometheus>/…`), que es lo mismo que muestra Grafana → Explore.

## 1. `POST /v2/traslados` a través del gateway, con token
```bash
# Token del sistema externo (client credentials)
curl -s -X POST localhost:8080/v2/auth/token -H 'content-type: application/json' \
  -d '{"client_id":"org-almacenamiento","client_secret":"<EXTERNO_CLIENT_SECRET de .env>"}'
# Solicitud
curl -i -X POST localhost:8080/v2/traslados -H "Authorization: Bearer $TOKEN" \
  -H 'content-type: application/json' -H 'Idempotency-Key: evidencia-1792166937' \
  -d '{"referencia_externa":"evidencia-entrega-2","cosa":{"descripcion":"cajas de mercadería seca","cantidad":4,"peso_kg":80},"origen":{"nombre":"Depósito central","direccion":"Av. Colón 1000, Córdoba"},"destino":{"nombre":"Sucursal Nueva Córdoba","direccion":"Obispo Trejo 300, Córdoba"}}'
```
```
HTTP/1.1 202 Accepted
Location: /v2/traslados/sol-3efbcb8a-2a31-4106-8410-0c3bbde66b24
X-Correlation-Id: fc5fb26d-4b24-4af9-b600-3a09788700ba
X-Correlation-Id: fc5fb26d-4b24-4af9-b600-3a09788700ba

{"id":"sol-3efbcb8a-2a31-4106-8410-0c3bbde66b24","referencia_externa":"evidencia-entrega-2","estado":"RECIBIDA", … ,"creado_en":"2026-10-16T13:08:57.305-03:00"}
```
**`X-Correlation-ID`: `fc5fb26d-4b24-4af9-b600-3a09788700ba`.**

> Observación: la respuesta trae el header `X-Correlation-Id` **dos veces**, con el mismo valor: el gateway agrega el suyo y además copia el que devuelve Integración. No rompe nada, pero conviene que el gateway no lo duplique.

## 2. Traza en Tempo
Se buscó el `trace_id` en los logs de ese `correlation_id` (punto 3) y se pidió la traza a Tempo (`/api/traces/3ccd2ca1528770e62a4f9326b6e870be`). Árbol de spans (servicio · nombre · duración · inicio relativo):
```
gateway · POST /v2/traslados · 21.0 ms (+0.0 ms)
  gateway · HTTP POST · 6.8 ms (+14.2 ms)
    integracion · POST /v2/traslados · 1.5 ms (+18.8 ms)
      integracion · publicar integracion.solicitud_carga.recibida · 8.0 ms (+687.9 ms)
        operaciones · consumir integracion.solicitud_carga.recibida · 14.5 ms (+690.4 ms)
          operaciones · publicar operaciones.solicitud.recibida · 3.6 ms (+1174.5 ms)
            integracion · consumir operaciones.solicitud.recibida · 2.5 ms (+1175.5 ms)
```
Es **el flujo principal de ADR-014 en una sola traza**, cruzando HTTP y RabbitMQ:
- La respuesta `202` sale a los ~21 ms.
- El **relay del outbox** publica el comando ~670 ms después (su ciclo de sondeo), y el span de publicación queda colgado del pedido HTTP original, porque el contexto de traza se guarda con el pendiente.
- Operaciones lo consume y registra la solicitud. ~480 ms después su propio relay publica el evento, que Integración consume para actualizar su copia.
- El contexto viaja en los headers AMQP `traceparent` / `tracestate`.

## 3. Logs del `correlation_id` en los tres servicios
Loki: `{service_name=~".+"} | correlation_id="fc5fb26d-4b24-4af9-b600-3a09788700ba"` (los campos estructurados se ven como etiquetas):
```
2026-10-16T16:08:57.306Z integracion  INFO  "solicitud recibida"              solicitud_id=sol-3efbcb8a-2a31-4106-8410-0c3bbde66b24 trace_id=3ccd2ca1528770e62a4f9326b6e870be
2026-10-16T16:08:57.307Z integracion  INFO  "request atendida"                metodo=POST ruta=/v2/traslados estado_http=202 duracion_ms=1  trace_id=3ccd2ca1528770e62a4f9326b6e870be
2026-10-16T16:08:57.308Z gateway      INFO  "request atendida"                metodo=POST ruta=/v2/traslados estado_http=202 duracion_ms=20 trace_id=3ccd2ca1528770e62a4f9326b6e870be
2026-10-16T16:08:57.984Z integracion  INFO  "solicitud enviada a Operaciones" solicitud_id=sol-3efbcb8a-2a31-4106-8410-0c3bbde66b24 trace_id=3ccd2ca1528770e62a4f9326b6e870be
2026-10-16T16:08:57.991Z operaciones  INFO  "solicitud externa registrada"    solicitud_id=sol-3efbcb8a-2a31-4106-8410-0c3bbde66b24 trace_id=3ccd2ca1528770e62a4f9326b6e870be
2026-10-16T16:08:58.464Z integracion  INFO  "evento de Operaciones procesado" solicitud_id=sol-3efbcb8a-2a31-4106-8410-0c3bbde66b24 tipo_mensaje=operaciones.solicitud.recibida aplicado=true trace_id=3ccd2ca1528770e62a4f9326b6e870be
```
Los tres servicios (gateway, Integración y Operaciones) registran el mismo `correlation_id` y el mismo `trace_id`: desde un log se llega a la traza y viceversa. Ningún log tiene el token ni el secreto.

## 4. Balanceo (ADR-009)
**Log de NGINX** (`docker compose logs balanceador-integracion`). Durante el escenario C de la prueba de carga, `replica=` alterna entre las dos réplicas (round robin). La request de la evidencia queda registrada con su `correlation`:
```
2026-10-16T15:36:47+00:00 "GET /v2/traslados/sol-5f76b384-… HTTP/1.1" estado=200 replica=172.23.0.13:8085 tiempo=0.002 correlation=-
2026-10-16T15:36:47+00:00 "GET /v2/traslados/sol-a07ab74e-… HTTP/1.1" estado=200 replica=172.23.0.12:8085 tiempo=0.001 correlation=-
2026-10-16T15:36:47+00:00 "GET /v2/traslados/sol-dcc475d5-… HTTP/1.1" estado=200 replica=172.23.0.13:8085 tiempo=0.001 correlation=-
2026-10-16T15:36:47+00:00 "GET /v2/traslados/sol-5816460e-… HTTP/1.1" estado=200 replica=172.23.0.12:8085 tiempo=0.002 correlation=-
2026-10-16T15:36:47+00:00 "GET /v2/traslados/sol-dcc475d5-… HTTP/1.1" estado=200 replica=172.23.0.13:8085 tiempo=0.001 correlation=-
2026-10-16T15:36:47+00:00 "GET /v2/traslados/sol-db78f965-… HTTP/1.1" estado=200 replica=172.23.0.12:8085 tiempo=0.001 correlation=-
2026-10-16T16:08:57+00:00 "POST /v2/traslados HTTP/1.1" estado=202 replica=172.23.0.9:8085 tiempo=0.004 correlation=fc5fb26d-4b24-4af9-b600-3a09788700ba
```
(Las requests de k6 van directo al balanceador y no traen `X-Correlation-ID`, por eso `correlation=-`. Las IP cambian después del reinicio de las réplicas de la prueba de RabbitMQ; ver [ADR-016](../adr/ADR-016-validacion-d5.md).)

**Panel "Requests por instancia de Integración"**, durante el escenario C: `sum by (instancia) (rate(rumbo_http_duracion_segundos_seconds_count{job="integracion"}[2m]))`
```
hora (UTC)   integracion-1   integracion-2
15:36:17        15,25 req/s      9,73 req/s
15:36:47        15,25 req/s     12,35 req/s
15:37:17        27,95 req/s     27,61 req/s
```
En todo el escenario C: 4296 requests en `integracion-1` y 3608 en `integracion-2`. Más datos en el [informe de capacidad](../capacidad/informe-entrega-2.md).

## 5. Otras verificaciones (ADR-015 y ADR-016)
| Qué | Resultado |
| --- | --- |
| Límite de tráfico del gateway | Ráfaga de 14 `GET` con el mismo token: 11 `200` (ráfaga de 10 más uno recargado) y 3 `429 Too Many Requests` con `Retry-After: 1` |
| DLQ | `{esto no es json` publicado en `operaciones.solicitudes-externas` → log `WARN "mensaje no procesado" destino=operaciones.solicitudes-externas.dlq reintentos=0 error="sobre inválido: …"` y 1 mensaje en la DLQ |
| Alerta "Mensajes en DLQ" | Después de corregir `for: 5m` → `for: 1m`: mensaje a las 16:06:39, `Pending` a las 16:07:40 y `Firing` a las 16:08:40 UTC |
| Idempotencia del consumidor | El mismo comando publicado dos veces → una sola fila en `operaciones.solicitudes` y un solo log "solicitud externa registrada" |
| RabbitMQ detenido | `POST /v2/traslados` → `502` (no `202`): los servicios terminan al cerrarse el canal. Detalle y ajustes en [ADR-016](../adr/ADR-016-validacion-d5.md) |
| Outbox al volver RabbitMQ | `rumbo_outbox_pendientes` = 0 en `integracion-1`, `integracion-2` y `operaciones`; `POST` → `202` después de reiniciar el balanceador |
