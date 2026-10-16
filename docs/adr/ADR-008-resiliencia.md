# ADR-008 — Resiliencia ante fallos de dependencias (D10)

- **Fecha:** 2026-10-14
- **Estado:** aceptada

## Contexto
La consigna (§2.9) pide identificar las dependencias de cada servicio, definir qué pasa cuando una falla o responde lento, implementar al menos un mecanismo de protección y demostrarlo con una caída provocada (registrada en `docs/POSTMORTEM.md`). El teórico 7 da los criterios:
- la lentitud puede ser peor que un error;
- timeout y deadline en toda llamada remota;
- reintentos solo en operaciones seguras;
- **circuit breaker** para dejar de insistir sobre una dependencia caída;
- degradación **semánticamente honesta**: no convertir errores en `200 OK`.

## Decisión

### Dependencias y comportamiento ante su falla
| Servicio | Dependencia | Si falla o tarda | Mecanismo |
| --- | --- | --- | --- |
| Integración de carga | **Operaciones** (HTTP: responder contrapropuesta, cancelar) | Responde `503 SERVICIO_NO_DISPONIBLE` con `Retry-After`. El consumidor reintenta con la misma `Idempotency-Key` | Timeout de 2 s + **circuit breaker** |
| Integración de carga | RabbitMQ (publicar el comando, recibir estados) | **Sigue aceptando solicitudes** (`202`): quedan pendientes de envío y el relay las publica cuando el broker vuelve. El consumidor de estados reconecta al reiniciar | Pendiente de envío + relay con reintentos (ADR-004) |
| Integración de carga | Memcached | Lee de MongoDB; nunca un error al consumidor | Degradación (ADR-007) |
| Integración de carga | MongoDB (su base) | `503 SERVICIO_NO_DISPONIBLE` | Timeout del driver |
| Operaciones | Sistema de aeropuerto (clima) | Sigue sin el dato: alerta "clima no verificado" (fail-open) | Timeout + reintento + breaker + caché (D9) |
| Operaciones | MySQL (su base) | `503` en sus endpoints; los mensajes se reintentan y, agotados, van a la DLQ | Timeout + reintentos de mensajería |
| Todos | RabbitMQ al arrancar | Reintenta la conexión con backoff y jitter | `mensajeria.ConectarConReintentos` |

### Circuit breaker en Integración → Operaciones
- Librería **`github.com/sony/gobreaker/v2`**. Un breaker llamado `operaciones` envuelve todas las llamadas HTTP de Integración a Operaciones.
- **Cuenta como falla:** timeout, error de conexión y respuestas `5xx`. **No cuenta:** `4xx`. Un `404` o un `409` son respuestas válidas del negocio, no un problema de la dependencia (teórico 7: validación ≠ fallo).
- **Se abre** cuando, en una ventana de 60 s, hay al menos 10 llamadas con 50 % o más de fallas, **o** 5 fallas consecutivas.
- **Abierto:** responde de inmediato `503 SERVICIO_NO_DISPONIBLE` con `Retry-After: 30`, sin llamar a Operaciones. Así no se acumulan requests esperando a una dependencia caída.
- **Semiabierto** a los **30 s**: deja pasar **1** llamada de prueba. Si sale bien se cierra; si no, vuelve a abrirse.
- **No reintenta.** El reintento le corresponde al consumidor (un solo dueño del reintento, ADR-004).
- **Observable:** `rumbo_circuito_estado{dependencia="operaciones"}` (0 cerrado, 1 semiabierto, 2 abierto) y un log en cada cambio de estado.

## Alternativas consideradas
- **Solo timeout, sin breaker.** Descartada: con Operaciones caída, cada request esperaría los 2 s completos y se acumularían conexiones (teórico 7: la lentitud se propaga).
- **Reintentar en Integración.** Descartada: amplifica la carga sobre la dependencia que está fallando y duplica la responsabilidad del reintento.
- **Responder `200` con el último estado conocido al fallar una cancelación.** Descartada: no es un fallback honesto. El consumidor creería que canceló sin que haya pasado.
- **Implementar el breaker a mano.** Descartada: existe una librería madura (regla del grupo: no reinventar).

## Consecuencias
- **Ganamos:**
  - una caída de Operaciones no tumba la capacidad publicada, que sigue aceptando solicitudes y respondiendo el seguimiento desde su copia;
  - el consumidor recibe señales claras (`503` + `Retry-After`).
- **Pagamos:**
  - mientras el breaker está abierto, también se rechazan pedidos que podrían haber funcionado;
  - hay que calibrar los umbrales con las pruebas de carga (D13).

## Cómo se verifica
- **Tests:** con un servidor falso que responde `500`, el breaker se abre después de 5 fallas consecutivas y la siguiente llamada falla sin llegar al servidor; un `409` no cuenta como falla.
- **Caída provocada:** con `docker compose stop operaciones`, `POST /v2/traslados` sigue devolviendo `202` y `POST …/cancelacion` devuelve `503` con `Retry-After`. Al levantar Operaciones, las solicitudes pendientes llegan y el breaker se cierra.
