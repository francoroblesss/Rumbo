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
- Por qué no pasó en el postmortem, con la misma configuración: <COMPLETAR: lo que muestre la corrida nueva de esta tarea. Hipótesis a confirmar o descartar: un `GET` que da `503` se reintenta en la otra réplica, así que cada `GET /disponibilidad` le suma una falla a **las dos** réplicas; en el postmortem los `503` eran solo de `POST`, que no se reintentan, y le sumaban una falla a una sola. Escribí qué se confirmó, sin inventar: si no se pudo confirmar, decilo.>
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
- **Caída de Operaciones** (`pruebas/fallas/`, misma corrida que el PR #33, evidencias en `docs/evidencias/caida-balanceador/`): **cero `502`**; `GET /traslados/{id}` con `200` durante toda la caída; `GET /disponibilidad` y las reservas con `503` y `Retry-After`; sin reservas duplicadas ni perdidas. Resultado: <COMPLETAR: números de resumen.md y reservas.txt.>
- **Una réplica caída se sigue detectando** (`docker compose stop integracion-2` con tráfico): ningún pedido falla y el log de NGINX muestra todo en `integracion-1`; al volver, el tráfico se reparte otra vez. Resultado: <COMPLETAR: pedidos enviados, estados recibidos y horas.>
