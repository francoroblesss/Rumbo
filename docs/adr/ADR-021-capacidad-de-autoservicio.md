# ADR-021 — Capacidad publicada de autoservicio: reservar una franja (D8, reemplaza parcialmente a ADR-002, ADR-004, ADR-006, ADR-008, ADR-009, ADR-014 y ADR-019)

- **Fecha:** 2026-10-28
- **Estado:** aceptada
- **Reemplaza a:**
  - de [ADR-002](ADR-002-contrato-propio.md) y [ADR-006](ADR-006-contrato-generico-v2.md): el **flujo** de la capacidad. Antes era `202` con la solicitud `RECIBIDA`, la revisión del operador, la `CONTRAPROPUESTA` (también cuando no se pedía hora), la respuesta del consumidor y la cancelación tardía pendiente de aprobación;
  - de [ADR-019](ADR-019-contrato-sin-version-en-la-ruta.md): la lista de operaciones del contrato 3.0.0 (`respuesta-contrapropuesta` deja de existir);
  - de [ADR-004](ADR-004-comunicacion-entre-servicios.md):
    - el **comando** `integracion.solicitud_carga.recibida` en la cola `operaciones.solicitudes-externas`;
    - la "pendiente de envío" con su relay en Integración;
    - la alternativa descartada "Integración llamando por HTTP a Operaciones para crear la solicitud", que ahora es la decisión;
  - de [ADR-008](ADR-008-resiliencia.md): las filas de Integración → Operaciones y Integración → RabbitMQ;
  - de [ADR-009](ADR-009-balanceo.md): el reclamo atómico del relay entre las dos réplicas, que ya no existe;
  - de [ADR-014](ADR-014-observabilidad.md): el recorrido de la traza del flujo principal.
- **Sigue vigente:**
  - REST con OpenAPI, la capacidad genérica (`cosa`, `origen`, `destino`), rutas sin versión y semver del contrato en el documento (ADR-019);
  - el token del gateway, `Idempotency-Key` en todo `POST`, Problem Details, seguimiento por consulta y mock con Prism;
  - Integración de carga como servicio dueño del contrato (ADR-001), en capas (ADR-010), con dos réplicas balanceadas (ADR-009) y caché de la consulta por id (ADR-007).

## Contexto
- El contrato 3.0.0 obligaba al consumidor a **negociar**:
  1. pedía un traslado y recibía `202 RECIBIDA`;
  2. esperaba (consultando cada minuto) a que **un operador de Rumbo** lo revisara;
  3. si recibía una `CONTRAPROPUESTA`, tenía que contestarla en menos de 24 h con otro endpoint;
  4. una cancelación tardía quedaba `PENDIENTE_APROBACION` hasta que decidiera el operador.
- Revisándolo después de la Entrega 2, el grupo concluyó que **ese ida y vuelta no corresponde a una capacidad publicada**:
  - el sistema del otro grupo tiene que poder **solo llamar endpoints** y obtener una respuesta definitiva;
  - no tiene que depender de una decisión manual, ni de Rumbo ni propia;
  - además, con un operador en el medio, la capacidad no funciona sin personal de Rumbo conectado, y el consumidor no puede incorporarla a un flujo automático (consigna §4.1: "una forma definida de utilizarla").
- **El otro grupo solo integró contra el mock** (Prism): cambiar el contrato no rompe nada desplegado, igual que en ADR-006 y ADR-019.
- **Teórico 9:**
  - para elegir la forma de comunicación importan "quién inicia" y "si hace falta respuesta inmediata";
  - REST modela **recursos de negocio**, y un `POST` que crea algo lleva clave de idempotencia;
  - ante un timeout el resultado es incierto: se reintenta con la misma clave o se reconcilia consultando.
- **Teórico 4:** lo asincrónico desacopla en el tiempo, pero "no elimina la responsabilidad de informar estado". Con la cola, el consumidor nunca sabía en el momento si había lugar.

## Decisión
1. **La capacidad pasa a ser "Reservar traslado" (contrato 4.0.0), de autoservicio.** Rumbo publica **franjas de llegada** con cupo y el consumidor reserva una. Todas las respuestas son definitivas:

   | Método y ruta | Qué hace | Respuesta |
   | --- | --- | --- |
   | `POST /auth/token` | Token (sin cambios) | `200` |
   | `GET /disponibilidad?fecha=&peso_kg=` | Franjas de la fecha que todavía se pueden reservar, con su lugar libre | `200` |
   | `POST /traslados` (`Idempotency-Key`) | Reserva `cosa` de `origen` a `destino` en la franja `franja_inicio` | `201 PROGRAMADO`, o `409 SIN_DISPONIBILIDAD`, o `422` |
   | `GET /traslados/{id}` y `GET /traslados?actualizado_desde=&estado=` | Estado del traslado | `200` |
   | `POST /traslados/{id}/cancelacion` (`Idempotency-Key`) | Cancela en el momento si no empezó y devuelve el lugar de la franja | `200 CANCELADO`, o `409` si ya empezó |

   - El recurso que ve el consumidor es el **traslado**, con su estado (`PROGRAMADO → ASIGNADO → EN_CURSO → COMPLETADO`, o `CANCELADO`, `NO_REALIZADO`, `INTERRUMPIDO`).
   - Desaparecen `RECIBIDA`, `CONTRAPROPUESTA`, `RECHAZADA`, `respuesta-contrapropuesta` y `PENDIENTE_APROBACION`.
   - Se mantuvo `PROGRAMADO` como estado inicial, en lugar de inventar uno nuevo ("confirmado"), porque es el estado que ya tiene un traslado comprometido (dominio §10).
2. **Franjas** (calendario de Operaciones, configurable por variables de entorno `FRANJA_*`):
   - franjas de **2 h entre las 08:00 y las 20:00** (hora de Argentina);
   - cada franja tiene lugar para **4 traslados y 2000 kg**;
   - se reserva con **2 h de anticipación** como mínimo y hasta **30 días** adelante.
   - La franja es la ventana de **llegada**: Rumbo se compromete a que la cosa llegue antes de `franja.fin`. Ese compromiso es lo que después respeta el operador al armar el recorrido.
   - No hay que cargar franjas a mano: existen todas las del horario, y la fila de una franja se crea cuando alguien reserva en ella por primera vez.
3. **Errores nuevos o con otro significado:**
   - `409 SIN_DISPONIBILIDAD`: la franja no tiene lugar. El consumidor consulta la disponibilidad y elige otra franja, con otra clave;
   - `422 ANTICIPACION_INSUFICIENTE` y `HORARIO_EN_EL_PASADO`: la franja empieza en menos de 2 h o ya empezó;
   - `422 EXCEDE_CAPACIDAD`: el peso supera el cupo total de una franja;
   - `422 DATOS_INVALIDOS` con `campo: franja_inicio`: no es el inicio de una franja reservable.
4. **Cancelación sin aprobación:**
   - se aplica siempre que el traslado no haya empezado;
   - si faltan menos de 24 h, queda marcada `tardia: true` para el operador (dominio §9: "la cancelación siempre se registra").
   - El circuito de aprobación del operador sigue existiendo solo para las solicitudes de los clientes de la aplicación web.
5. **Comunicación interna (reemplaza la parte de ADR-004):** la reserva pasa a ser **síncrona**.
   - Integración llama a Operaciones con `POST /internas/reservas` y `GET /internas/disponibilidad`, con el mismo cliente que ya tenía: timeout de 2 s, **circuit breaker** y sin reintento propio, porque el dueño del reintento es el consumidor con su clave.
   - Integración **reenvía la `Idempotency-Key` del consumidor**, y Operaciones la resuelve **dentro de la transacción de la reserva**.
   - Operaciones decide en una sola transacción: bloquea la franja, verifica y descuenta el lugar, crea la solicitud aceptada con su traslado y deja el evento `operaciones.solicitud.aceptada` en el outbox. Concurrencia y garantías en [ADR-022](ADR-022-consistencia-y-concurrencia.md) (D4).
   - Se eliminan el comando `integracion.solicitud_carga.recibida`, la cola `operaciones.solicitudes-externas` con su DLQ, y en Integración la "pendiente de envío", su relay, el reclamo entre réplicas y la métrica `rumbo_outbox_pendientes{servicio="integracion"}`.
   - **Integración ya no publica mensajes.** Sigue consumiendo los eventos de Operaciones para actualizar su copia (read model, teórico 5); el evento de dominio publicado y consumido (consigna §2.5) se mantiene.
6. **Copia local en Integración:** se guarda **después** de que Operaciones confirma, con la clave y la respuesta.
   - Un reintento con la misma clave se responde desde la copia, sin llamar a Operaciones (funciona aunque Operaciones esté caída).
   - Si Integración no llega a guardar la copia, responde `503`. El reintento del consumidor obtiene de Operaciones **la misma reserva**, porque su clave quedó guardada en la misma transacción, y esta vez la copia se guarda.
7. **Traza del flujo principal** (reemplaza la de ADR-014):

   gateway → Integración `POST /traslados` → Operaciones `POST /internas/reservas` → (relay del outbox) `publicar operaciones.solicitud.aceptada` → Integración `consumir operaciones.solicitud.aceptada`.

## Alternativas consideradas
- **Mantener el `202` y que Operaciones acepte o rechace sola, sin contrapropuesta.** Descartada:
  - el consumidor seguiría sin saber en el momento si tiene el traslado, y tendría que consultar hasta que cambie el estado;
  - aceptar o rechazar "sola" necesita la misma regla de cupo que la reserva, así que la cola no agrega nada salvo la espera.
- **Reserva por hora exacta con tolerancia** (como pedía la 3.0.0). Descartada: Rumbo no calcula rutas (dominio §16) y no puede comprometer una hora exacta sin que el operador arme el recorrido. Una franja de 2 h es un compromiso que sí puede cumplir.
- **Calcular la disponibilidad con los choferes y vehículos reales** (pidiéndole a Recursos). Descartada por ahora:
  - suma una llamada síncrona más en el camino de la reserva;
  - un vehículo libre a las 10 no garantiza poder asignarlo sin armar el recorrido.

  El cupo por franja es la capacidad que Rumbo **decide ofrecer** a terceros, y el operador la respeta al planificar. Queda en las limitaciones conocidas.
- **Avisar los cambios con un webhook** en lugar de que el consumidor consulte. Descartada: el teórico 9 lo justifica cuando "el proveedor conoce el cambio antes" y el consumidor tiene un endpoint alcanzable, firma y deduplicación. Agrega una frontera pública en el sistema del otro grupo, sin un problema que lo pida: los cambios de estado son pocos por traslado.
- **Mantener la cola para la reserva y responder con una espera corta** (pedido/respuesta sobre RabbitMQ). Descartada: complica las dos puntas para obtener lo mismo que una llamada HTTP con timeout.

## Consecuencias
- **Ganamos:**
  - el consumidor integra la capacidad con cuatro llamadas y sin máquina de estados de negociación: reserva, consulta y cancela;
  - la respuesta es inmediata y definitiva, y el flujo funciona sin personal de Rumbo;
  - la operación crítica que pide D4 (que no se pierda ni se duplique, y que dos no tomen el mismo lugar) es la propia capacidad publicada, que el otro grupo usa de verdad;
  - Integración queda más simple (sin relay, sin reclamo entre réplicas, sin publicar).
- **Pagamos:**
  - **Se pierde el desacople temporal** (teórico 4) que ADR-004 había elegido: si Operaciones está caída o lenta, el consumidor recibe `503` (breaker abierto o timeout de 2 s) en vez de un `202`. Se acepta porque el consumidor necesita saber en el momento si tiene el lugar, y una respuesta "recibida, ya veremos" no le sirve. El `503` lleva `Retry-After` y el reintento con la misma clave es seguro.
  - Ante un timeout el resultado es incierto, como en el caso del pago del teórico 9. Se resuelve con la misma `Idempotency-Key` o consultando `GET /traslados`.
  - Es un cambio incompatible (versión mayor 4.0.0). Como el consumidor solo usó el mock, no hay convivencia: se le avisa y se actualiza el mock.
  - La regla de clima (ADR-012) se aplica cuando el operador acepta una solicitud de la web marcando el aeropuerto. Las reservas externas no pasan por el operador, así que la consulta de clima para ellas queda para el inicio del recorrido (dominio §8), que se implementa con los recorridos.
- **Cambios en el sistema:**
  - Operaciones:
    - dominio `Franja` y `Calendario`, `NuevaReservaExterna`, cancelación externa sin aprobación;
    - tabla `franjas` y columnas `traslados.franja_inicio` y `solicitudes.cancelacion_tardia` (migración `0002`);
    - endpoints internos de reserva y disponibilidad;
    - ya no consume el comando;
  - Integración: contrato 4.0.0, `GET /disponibilidad`, reserva síncrona, sin relay;
  - gateway: rutea `/disponibilidad` a Integración;
  - `docs/contracts/traslados.yaml` 4.0.0, con la 3.0.0 guardada como `traslados-v3.yaml`, `docs/contracts/README.md`, `docs/interfaces-internas.md`, `dominio.md`, `SPEC.md` y `ARCHITECTURE.md`.

## Cómo se verifica
- **Test de contrato de Integración** (`controllers/contrato_test.go`, `kin-openapi`): cada respuesta, incluidos `201`, `409 SIN_DISPONIBILIDAD`, `422` y `503`, cumple `traslados.yaml` 4.0.0.
- **Tests de aplicación de Operaciones** (`aplicacion/solicitudes_test.go`):
  - reservar descuenta el cupo y deja `operaciones.solicitud.aceptada` en el outbox;
  - un reintento devuelve la misma reserva aunque la franja se haya llenado;
  - una franja llena da `SIN_DISPONIBILIDAD` sin guardar nada;
  - cancelar devuelve el lugar.
- **Test de integración contra MySQL real** (`adaptadores/mysql`): ver ADR-022.
- **Con el sistema levantado:**
  - `GET /disponibilidad` y `POST /traslados` a través del gateway responden `200` y `201`;
  - cuando una franja se llena, la reserva siguiente da `409`;
  - en Tempo, la traza de un `POST /traslados` muestra el span de Operaciones dentro de la misma traza.
