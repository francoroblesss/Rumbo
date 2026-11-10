# ADR-031 — El balanceador saca de rotación una réplica solo por fallas de la réplica

- **Fecha:** 2026-11-10
- **Estado:** aceptada
- **Reemplaza parcialmente a:** [ADR-009](ADR-009-balanceo.md), en el punto "Reintento en otra réplica" (que reintentaba también ante `502` y `503`). El resto de ADR-009 sigue vigente, incluido el health check pasivo `max_fails=3 fail_timeout=30s`.

## Contexto
- ADR-009 decidió dos cosas para el NGINX que balancea las dos réplicas de Integración:
  - **detección de réplicas caídas** con health checks pasivos: "tres errores de conexión o timeout dentro de 30 s sacan a la réplica de rotación durante 30 s";
  - **reintento en otra réplica** "para errores de conexión, timeout, `502` y `503`" (`proxy_next_upstream error timeout http_502 http_503`), solo en métodos idempotentes.
- En NGINX esas dos cosas no son independientes: **lo que cuenta como falla para `max_fails` es lo que lista `proxy_next_upstream`**. Con `http_503` en la lista, cada `503` que responde una réplica cuenta como falla **de esa réplica**, aunque la réplica esté sana.
- Integración responde `503 SERVICIO_NO_DISPONIBLE` (con `Retry-After`) **solo cuando falla una dependencia que comparten las dos réplicas**: Operaciones (por timeout, error de conexión o breaker abierto, ADR-008 y ADR-021) o MongoDB. Nunca responde `502`.
- **Medido el 2026-11-10** en la corrida de la caída de Operaciones con `GET /disponibilidad` (`docs/evidencias/caida-disponibilidad/`, PR #33): además de los `503` esperados, hubo **`502` en las tres operaciones** (33 de disponibilidad, 31 reservas y **26 `GET /traslados/{id}`**). `GET /traslados/{id}` no depende de Operaciones: en el postmortem había dado 361 `200` de 361. Los logs de NGINX muestran `no live upstreams while connecting to upstream`: con las dos réplicas marcadas como caídas, NGINX corta **todo** el tráfico de Integración durante 30 s.
- Por qué no pasó en el postmortem, con la misma configuración: en el postmortem, los 199 `503` que respondió Integración fueron todos de `POST /traslados` y coinciden 1 a 1 con los 199 `503` que recibió k6 (`docs/evidencias/postmortem/logs.txt` y `resumen.md`): NGINX no reintenta `POST`, y esos `503` no sacaron de rotación a ninguna réplica (cerca de 24 por minuto durante 8 minutos, sin un solo `502`). En la corrida del PR #33, en cambio, Integración registró **57** `503` de `GET /disponibilidad` contra **15** que llegaron al consumidor (`docs/evidencias/caida-disponibilidad/logs.txt` y `resumen.md`): cada `GET` con `503` se reintentaba en la otra réplica. **Se confirmó** que lo que disparó los `502` fueron los `GET` con `503`, reintentados por NGINX: los `503` de `POST` no contaron como fallas. Con ADR-031, la corrida nueva registró 49 `503` de `GET /disponibilidad` en Integración y los mismos 49 en k6 (sin reintento), y cero `502`. **No se pudo confirmar** con las evidencias si cada `GET` le sumaba una falla a las dos réplicas o solo a la primera (el log de NGINX de la corrida del PR #33 no se guardó por pedido); la hipótesis es que la falla se cuenta cuando NGINX decide reintentar, que solo pasa en métodos idempotentes.
- Del teórico:
  - teórico 6: un health check tiene que distinguir **vivo ≠ listo ≠ dependencia degradada**; una sonda demasiado amplia también daña;
  - teórico 7: contener los *faults* evita **fallas en cascada**; "reintentar contra todas las réplicas ante una dependencia común saturada multiplica la carga"; y "toda excepción = transitoria" y "retry en cada capa" son anti-patrones. Debe haber **un solo dueño del reintento**: el consumidor, con su `Idempotency-Key` (ADR-004).

## Decisión
- **`proxy_next_upstream error timeout;`** en `infra/nginx/integracion.conf`: NGINX reintenta en la otra réplica, y cuenta la falla para `max_fails`, **solo ante un error de conexión o un timeout**, que son las fallas **de la réplica** (proceso caído, colgado o sin red).
- Un `503` (o cualquier otra respuesta) de una réplica **se devuelve tal cual** al gateway y al consumidor: es la respuesta correcta de una réplica sana ("mi dependencia no está, reintentá después de `Retry-After`").
- **Se mantiene de ADR-009:** round robin, `max_fails=3 fail_timeout=30s`, `proxy_next_upstream_tries 2`, timeouts de 1 s (conexión) y 5 s (lectura), sin `non_idempotent`, y el resto. **Se mantiene de ADR-017:** `resolver` y `resolve`.

## Alternativas consideradas
- **Dejarlo como estaba y anotarlo como limitación.** Descartada: con una dependencia caída, NGINX saca de rotación réplicas sanas y deja sin servicio también las operaciones que no dependen de ella. Es justo la falla en cascada que el teórico 7 pide contener.
- **Sacar solo `http_503` y dejar `http_502`.** Descartada: Integración nunca responde `502`; dejarlo no protege de nada y mantiene la puerta abierta al mismo problema.
- **Subir `max_fails` o bajar `fail_timeout`.** Descartada: solo corre el umbral; con suficiente tráfico, el problema vuelve.
- **Health check activo** (`/salud` periódico). Descartada, como en ADR-017: en NGINX es una función de NGINX Plus.

## Consecuencias
- **Ganamos:**
  - una dependencia caída ya no deja sin servicio a todo Integración: `GET /traslados/{id}` sigue respondiendo `200` desde MongoDB mientras Operaciones está caído;
  - el consumidor recibe siempre el `503` con `Retry-After` del contrato, no un `502` sin indicaciones;
  - menos carga contra la dependencia caída: un `GET` ya no la llama dos veces (una por réplica).
- **Pagamos:**
  - si una réplica responde `503` por un problema **solo suyo** (por ejemplo, perdió su conexión a MongoDB y la otra no), NGINX no la saca de rotación: la mitad de los pedidos dan `503` hasta que se reconecte. El consumidor reintenta con su `Idempotency-Key`, como en cualquier `503`;
  - un `GET` que da `503` ya no se reintenta en la otra réplica: si el problema era de una sola réplica, ese reintento lo hace el consumidor.

## Cómo se verifica
- **Caída de Operaciones** (`pruebas/fallas/`, misma corrida que el PR #33, evidencias en `docs/evidencias/caida-balanceador/`): **cero `502`**; `GET /traslados/{id}` con `200` durante toda la caída; `GET /disponibilidad` y las reservas con `503` y `Retry-After`; sin reservas duplicadas ni perdidas. Resultado: corrida del 2026-11-10, 14:27–14:42 UTC: **cero `502`** (en el PR #33, 90). Durante la caída (minutos 3–10): `GET /traslados/{id}` con 192 `200` de 192 (antes, 166 `200` y 26 `502`); `GET /disponibilidad` con 48 `503` (antes, 15 `503` y 33 `502`), todos con `Retry-After`; reservas con 191 `503` y 1 `201` (antes, 161 `503`, 30 `502` y 1 `201`). Sin `no live upstreams` en los logs. `reservas.txt`: 172 claves con `201`, una fila y una solicitud por clave, y 0 claves con solo `503` que tengan reserva; las 3 reservas reintentadas con la misma `Idempotency-Key` dieron `201`.
- **Una réplica caída se sigue detectando** (`docker compose stop integracion-2` con tráfico): ningún pedido falla y el log de NGINX muestra todo en `integracion-1`; al volver, el tráfico se reparte otra vez. Resultado: 40 `GET /disponibilidad`, uno cada 2 s, de 14:43:10 a 14:44:32 UTC, con `stop` de `integracion-2` a las 14:43:30 y `start` a las 14:43:56: **40 `200`**. NGINX tuvo dos timeouts de conexión hacia `integracion-2` (14:43:32 y 14:43:37), los reintentó en `integracion-1` (los dos dieron `200`), y de 14:43:39 a 14:43:59 todo lo atendió `integracion-1`; desde 14:44:01 el tráfico vuelve a alternar entre las dos réplicas (`docs/evidencias/caida-balanceador/balanceador.txt`).
