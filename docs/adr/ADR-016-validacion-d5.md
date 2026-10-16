# ADR-016 — Validación de la comunicación entre servicios (D5) con latencias medidas

- **Fecha:** 2026-10-16
- **Estado:** aceptada
- **Reemplaza parcialmente a:** ninguno (ningún timeout cambia)

## Contexto
La Entrega 2 pide **validar D5** (ADR-004). En la devolución de la Entrega 1, el profesor pidió **"no jugársela con las comunicaciones y los tiempos"**. ADR-004 fijó timeouts a priori:
- **5 s** de deadline en el gateway;
- **2 s** Integración → Operaciones;
- **800 ms** Operaciones → Recursos y Operaciones → Identidad;
- y ADR-012 fijó **1 s** Operaciones → clima.

El teórico 7 dice que un timeout tiene que salir del **presupuesto de la operación** y de la **latencia real** de la dependencia, no de un número copiado.

## Método
- **Medición:** latencias de cliente de cada llamada saliente, tomadas de la métrica `http.client.request.duration` que genera `otelhttp` en `observabilidad.ClienteHTTP`, y de los spans en Tempo. Se miden en el escalón de **10 req/s del escenario C** de ADR-015 (carga normal).
- **Regla de cálculo** (aplicarla sin excepciones): **timeout = p99 medido × 3, redondeado hacia arriba a múltiplos de 100 ms, con un mínimo de 300 ms y un máximo de 5 s** (el deadline del gateway).
  - El factor 3 deja margen para variaciones normales sin esperar indefinidamente a una dependencia degradada.
  - Si el valor calculado es **menor o igual** al vigente, **se mantiene el vigente** (no se ajusta hacia abajo con una sola medición).
  - Si es **mayor**, se adopta el nuevo.

## Resultados
| Llamada | Timeout vigente | p50 | p99 medido | Timeout por la regla | Decisión |
| --- | --- | --- | --- | --- | --- |
| Integración → Operaciones (respuesta contrapropuesta / cancelación) | 2 s | 13,9 ms¹ | 41,0 ms¹ | 41 × 3 = 123 → 200 ms → mínimo **300 ms** | **Se mantiene 2 s** (300 ms ≤ 2 s) |
| Operaciones → clima (mock del proveedor) | 1 s | 6,3 ms² | 141,5 ms³ | 141,5 × 3 = 424,5 → **500 ms** | **Se mantiene 1 s** (500 ms ≤ 1 s) |
| Gateway → servicios (deadline total) | 5 s | 2,3 ms⁴ | 53,7 ms⁴ | — | Se mantiene: es el presupuesto total |
| Operaciones → Recursos / Identidad | 800 ms | — | — | — | Sin implementar en esta etapa: se valida cuando existan las llamadas |

Cómo se obtuvo cada valor (scripts en `pruebas/carga/`: `d5.js` genera las llamadas y `latencias-d5.mjs` las lee de Tempo y Prometheus):
- ¹ **Integración → Operaciones:** 61 spans de cliente en Tempo (`server.address = operaciones`), uno por cada cancelación lanzada a 2 por segundo durante el escalón de 10 req/s de C (2026-10-16, 15:34:37 a 15:35:17 UTC). Visto desde k6 (incluye el trabajo propio de Integración), la cancelación completa tuvo p50 de 19,3 ms y p99 de 47,7 ms, sin errores.
- ² **Clima, p50:** 30 consultas directas al mock (`GET /v1/airports/SACO/conditions`, la misma request que arma el adaptador), en el mismo escalón: p50 de 6,3 ms y p99 de 31,6 ms. Hacen falta porque el adaptador cachea el clima 5 min (ADR-012): de 30 aceptaciones al aeropuerto en ese escalón, solo la primera salió al proveedor (49,0 ms); las demás se resolvieron con la caché.
- ³ **Clima, p99:** se toma el valor más conservador, la llamada real más lenta de Operaciones al proveedor en toda la sesión. Hubo 3 llamadas reales (49,0, 109,7 y 141,5 ms), y cada una abre una conexión nueva, porque con la caché de 5 min la conexión anterior ya se cerró. Así va a ser también en producción.
- ⁴ **Gateway:** 29 spans de servidor del gateway devueltos por la búsqueda de Tempo, de 50 pedidos a 1 por segundo (45 `GET` y 5 `POST`) a través del gateway con token. Se midió fuera del escalón de 10 req/s porque el límite del gateway (60 por minuto por organización) no deja pasar esa tasa con una sola organización. Visto desde el cliente: p50 de 4,8 ms y p99 de 16,3 ms.
- La métrica `http.client.request.duration` (Prometheus) dio valores del mismo orden para Integración → Operaciones (p50 ≈ 18 ms y p99 ≈ 48 ms). Sus buckets (5, 10, 25, 50, 75, 100, 250 ms…) son gruesos para este rango, así que se usan los spans, que dan la duración exacta de cada llamada.

## Validación del resto de D5
| Decisión de ADR-004 | Evidencia | Resultado |
| --- | --- | --- |
| Un solo dueño del reintento | El gateway y NGINX no reintentan `POST`; Integración no reintenta hacia Operaciones (ADR-008) | **Se confirma, con una precisión.** Integración no reintenta (un span de cliente por pedido). NGINX sí reenvía un `POST` a la otra réplica, pero solo cuando **no pudo conectarse** a la primera: el pedido nunca llegó, así que no hay doble efecto. Se ve en el log como `replica=172.23.0.13:8085, 172.23.0.12:8085 tiempo=1.000, 0.002`. Además, el `Idempotency-Key` protege si alguna vez llegara dos veces |
| Outbox y pendientes de envío | Con RabbitMQ detenido, `POST /v2/traslados` sigue respondiendo `202`; al volver, los pendientes se publican (`rumbo_outbox_pendientes` vuelve a 0) | **No se confirma: falla la implementación, no la decisión.** Con RabbitMQ detenido, `POST /v2/traslados` respondió **`502`**. Al cerrarse el canal, el consumidor de estados de Integración termina, y `main` corta el proceso (`os.Exit(1)`) en vez de reconectar. Docker lo reinicia, pero el servicio espera a RabbitMQ antes de abrir el puerto HTTP (`ConectarConReintentos`); Operaciones hace lo mismo. Al volver RabbitMQ, los servicios se recuperan en ~20 s, pero **NGINX siguió respondiendo `502`**, porque resolvió las IP de las réplicas al arrancar y los contenedores reiniciados tienen otras (.12 y .13 contra .9 y .3). Después de reiniciar el balanceador: `202`, y `rumbo_outbox_pendientes` en 0 en los tres procesos. Ver Decisión |
| Consumidores idempotentes y DLQ | Un mismo mensaje entregado dos veces produce un solo efecto; un payload inválido termina en `<cola>.dlq` | **Se confirma.** El mismo comando `integracion.solicitud_carga.recibida` (mismo `id` y `solicitud_id`), publicado dos veces en `operaciones.solicitudes-externas`, produjo **una** fila en MySQL y **un** log de "solicitud externa registrada"; el segundo se confirmó sin ir a la DLQ. Un mensaje con JSON inválido fue **directo** a `operaciones.solicitudes-externas.dlq`, sin reintentos (error permanente), con su `trace_id` en el log. La alerta "Mensajes en DLQ" se disparó a los 2 min, después de corregir su `for` (ver Decisión) |
| Correlación y trazas | Traza única del flujo principal (ADR-014) | **Se confirma.** Un `POST /v2/traslados` a través del gateway produjo una sola traza de 7 spans: gateway → Integración → `publicar integracion.solicitud_carga.recibida` → Operaciones `consumir …` → `publicar operaciones.solicitud.recibida` → Integración `consumir …`. Los logs de los tres servicios tienen el mismo `correlation_id` y `trace_id` ([evidencias](../evidencias/entrega-2.md)) |

## Decisión
**D5 se confirma: ningún timeout cambia.** Los p99 medidos están muy por debajo de los timeouts vigentes (41 ms contra 2 s; 142 ms contra 1 s). Por la regla, los vigentes se mantienen, porque no se ajustan hacia abajo con una sola medición. También se verificaron en funcionamiento el único dueño del reintento, los consumidores idempotentes, la DLQ y la correlación.

**La garantía del outbox ante una caída del broker no se cumple en la implementación actual.** La decisión de ADR-004 es correcta; lo que falta es que el proceso sobreviva para usar el outbox. Quedan dos ajustes para los dueños de cada componente, registrados como deuda en la sección 10 de `ARCHITECTURE.md`:
1. **Integración y Operaciones:** que la caída del consumidor o del relay no termine el proceso. Hay que reconectar con backoff y abrir el puerto HTTP sin esperar a RabbitMQ (teórico 7: contener la perturbación y conservar las capacidades que no dependen de la dependencia caída).
2. **Balanceador (ADR-009):** volver a resolver los nombres de las réplicas (`resolver 127.0.0.11 valid=10s;` y `resolve` en los `server` del upstream), para que el reinicio de una réplica no deje a NGINX apuntando a una IP vieja.

Se corrigió además la alerta "Mensajes en DLQ" de ADR-014 (`infra/grafana/alertas/rumbo-alertas.yaml`). Su condición mira los últimos 5 min, así que con un solo mensaje es verdadera exactamente 5 min, y con `for: 5m` nunca llegaba a dispararse: se observó `Pending` de 15:56:40 a 16:01:40 UTC y la vuelta a `Normal`. Con `for: 1m` pasó a `Firing` 2 min después del mensaje (16:06:39 → 16:08:40).

## Nota sobre el clima
El p99 del clima se midió **contra el mock** (Prism, local). Cuando el grupo proveedor publique su URL real, hay que repetir la medición con la misma regla.
