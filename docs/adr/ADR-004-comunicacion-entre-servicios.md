# ADR-004 — Comunicación entre servicios (D5)

- **Fecha:** 2026-10-04
- **Estado:** aceptada (versión inicial para la Entrega 1; se valida en la Entrega 2)

## Contexto
ADR-001 definió cinco servicios y sus relaciones, y ADR-003 su persistencia. La consigna (§2.5) pide comunicación **síncrona** con timeout, manejo de errores, respuestas tardías o ausentes y política de reintentos, y comunicación **asíncrona** por mensajería, con publicación y consumo de un evento de dominio y tratamiento de mensajes fallidos.

Criterios de los teóricos:
- **Sync o async** (teórico 4): ¿el negocio puede esperar para conocer el resultado? Lo que condiciona una decisión inmediata va síncrono; avisos, analítica e indexación, asíncronos.
- **Toda llamada remota con timeout y deadline propagado** (teórico 7). Un timeout no es un fracaso de negocio: el resultado puede ser desconocido.
- **Reintentos** solo en operaciones seguras o idempotentes, con backoff exponencial y jitter, y **un único dueño del reintento**, la capa inmediatamente superior (teórico 7).
- **Mensajería** (teórico 4): evento (hecho, 0..N interesados) ≠ comando (pide una acción, un responsable). Entrega at-least-once, ACK después del efecto, consumidores idempotentes, outbox para no perder la intención, y DLQ con dueño y procedimiento.

## Decisión

### 1. Interacciones síncronas (HTTP/JSON)
En Go, cada llamada saliente usa `http.NewRequestWithContext` sobre el `context` de la request entrante, de modo que el **deadline se propaga** y no se reinicia en cada salto. Además, cada cliente HTTP tiene su propio timeout.

| Llamada | Para qué | Timeout | Reintento | Si falla, tarda o no responde |
| --- | --- | --- | --- | --- |
| Cliente → **API Gateway** → servicio | Toda request externa (frontend y grupo consumidor) | Deadline total 5 s | **El gateway no reintenta** | 502 si no puede conectar, 504 si vence el tiempo. El reintento le corresponde al cliente (con `Idempotency-Key` en los POST del contrato) |
| Gateway: validar token | Autenticación en el borde | — (local) | — | El token firmado se valida **localmente** con la clave pública, sin llamar a Identidad en cada request. Inválido → 401 |
| Gateway → **Identidad**: `POST /v1/auth/token` | Emitir tokens (usuarios y grupo consumidor) | 1 s | No | 503 al cliente |
| **Operaciones → Recursos**: disponibilidad de chofer y datos de vehículo | Validar una asignación | 800 ms | 1 reintento (lectura idempotente), backoff 100–200 ms con jitter, dentro del deadline | **Se rechaza la asignación** ("no se pudo verificar disponibilidad, reintentar"). Fail-closed: no se asigna sin verificar (teórico 7) |
| **Operaciones → Identidad**: datos de las personas | Copiarlos al aceptar una solicitud | 800 ms | 1 reintento (lectura), backoff con jitter | **No se acepta**: la solicitud sigue `RECIBIDA` y el operador reintenta. No se planifica sin domicilio ni necesidades |
| **Integración de carga → Operaciones**: responder contrapropuesta o cancelar | Operaciones decide (24 h, vencimiento, estado) | 2 s | **No reintenta**; propaga el `Idempotency-Key` del consumidor | Responde **503** (reintentable según el contrato). Si fue un timeout, el resultado es desconocido: el consumidor reintenta con la **misma clave** y Operaciones, idempotente, no aplica el cambio dos veces |
| **Operaciones → sistema del aeropuerto** (clima) | Capacidad consumida | Se define en D9 | Se define en D9 | Se define en D9/D10 |

- **Un solo dueño del reintento por cadena:** los servicios internos solo reintentan **una vez** las lecturas propias. Las escrituras no se reintentan dentro del sistema; las reintenta el cliente original con idempotencia. Así se evita la amplificación de reintentos.
- Circuit breaker, bulkhead y degradación ante dependencias lentas van en D10.

### 2. Interacciones asíncronas (RabbitMQ)
**Envoltorio de todo mensaje** (contrato versionado, teórico 4):
```json
{
  "id": "uuid",
  "tipo": "operaciones.solicitud.aceptada",
  "version": 1,
  "ocurrido_en": "2026-10-15T09:40:00-03:00",
  "correlation_id": "uuid de la request que lo originó",
  "agregado_id": "sol-7f3a9c",
  "agregado_version": 4,
  "datos": { }
}
```
- Agregar campos opcionales es compatible. Cambiar el significado o quitar campos implica una nueva `version`.
- `agregado_version` permite descartar mensajes tardíos o fuera de orden.

**Topología:**
- **Eventos (pub/sub):** un exchange `topic` por servicio productor (`rumbo.operaciones`, `rumbo.recursos`). Cada servicio interesado tiene **su propia cola** con los bindings que necesita. El productor no conoce a los consumidores.
- **Comando (cola de trabajo):** `operaciones.solicitudes-externas`. Un único responsable (Operaciones) y consumidores que compiten.
- **QoS (prefetch)** acotado en cada consumidor, como en el práctico.

| Mensaje | Tipo | Productor → consumidores | Efecto |
| --- | --- | --- | --- |
| `integracion.solicitud_carga.recibida` | **Comando** | Integración de carga → Operaciones | Crea la solicitud interna (idempotente por id externo) |
| `operaciones.solicitud.{aceptada, contrapropuesta, rechazada, vencida, cancelada}` | Evento | Operaciones → Integración*, Notificaciones | Integración actualiza su copia del estado; Notificaciones avisa al cliente |
| `operaciones.traslado.estado_cambiado` (asignado, en curso, completado, no realizado, interrumpido, cancelado, demora) | Evento | Operaciones → Integración* | El consumidor externo ve el avance del traslado |
| `operaciones.recorrido.{asignado, reasignado, cancelado, horario_cambiado}` | Evento | Operaciones → Notificaciones | Avisa al chofer |
| `operaciones.recorrido.finalizado` (chofer, vehículo, horas, km) | Evento de dominio | Operaciones → Recursos | Registra la actividad para la preliquidación |
| `operaciones.{incidente.reportado, traslado.requiere_revision, cambio_tardio.solicitado}` | Evento | Operaciones → Notificaciones | Avisa a los operadores |
| `recursos.chofer.ausencia_registrada`, `recursos.vehiculo.baja_registrada` | Evento | Recursos → Operaciones | Marca los recorridos afectados "requiere revisión" |

\* Integración de carga solo procesa los mensajes de solicitudes de origen externo (filtra por `datos.origen = "externo"`). Los demás los confirma sin efecto.

**Publicación sin perder eventos (teórico 2 y 4):**
- **Operaciones y Recursos:** **transactional outbox**. El cambio de negocio y la fila del evento se guardan en la misma transacción de PostgreSQL. Un relay (polling publisher) publica con *publisher confirms* y recién entonces marca la fila como publicada.
- **Integración de carga:** la solicitud y la marca `pendiente_envio` están en el mismo documento de MongoDB (atómico, ADR-003). Un relay envía las pendientes y desmarca al recibir la confirmación.
- El relay puede publicar dos veces, así que **todos los consumidores son idempotentes**.

**Consumo (at-least-once):**
- **ACK solo después del efecto durable.** Un ACK antes del efecto es una pérdida lógica.
- **Idempotencia por consumidor:**

| Consumidor | Cómo evita duplicados |
| --- | --- |
| Operaciones | Tabla `mensajes_procesados`, insertada en la misma transacción que el efecto. Además, el id externo es único en las solicitudes |
| Recursos | Restricción única sobre `recorrido_id` en la actividad (ADR-003) |
| Notificaciones | Índice único (`evento_id`, `destinatario`) |
| Integración de carga | Aplica un cambio solo si `agregado_version` es mayor que la guardada: los duplicados y los mensajes tardíos se descartan |

- **Avisos sin consultar a Identidad:** los eventos llevan `organizacion_id` y, cuando corresponde, el chofer. El chofer se identifica con el id de su usuario. Notificaciones dirige el aviso a la organización, al chofer o al rol operador sin llamadas síncronas.

**Mensajes fallidos:**
- **Error transitorio** (base o dependencia caída, timeout): hasta **3 reintentos** con backoff exponencial y jitter (por ejemplo ~1 s, ~4 s, ~16 s), usando una cola de reintento con TTL que devuelve el mensaje a la cola principal (dead-letter exchange). El motivo se registra en cada reintento.
- **Error permanente** (payload inválido, versión no soportada) o reintentos agotados: el mensaje va a la **DLQ de esa cola** (`<cola>.dlq`).
- **La DLQ no es una papelera** (teórico 4): cada una tiene como dueño el equipo del servicio consumidor y una alerta cuando recibe mensajes (D11). El reproceso se hace manualmente, después de corregir la causa, reenviando a la cola principal; como los consumidores son idempotentes, reprocesar es seguro.

### 3. Correlación
- Toda request recibe un `correlation_id` en el gateway (header `X-Correlation-ID`), o conserva el que traiga.
- Se propaga en cada llamada HTTP interna y en el envoltorio de cada mensaje.
- Se registra en todos los logs, para seguir una operación de punta a punta (base de D11).

## Alternativas consideradas
- **Todo síncrono** (por ejemplo, Operaciones llamando por HTTP a Notificaciones y a Recursos al finalizar un recorrido). Descartada: suma acoplamiento temporal y latencia a operaciones que no necesitan esperar, y una caída de Notificaciones bloquearía la operación.
- **Integración de carga llamando por HTTP a Operaciones para crear la solicitud.** Descartada: si Operaciones está caída, la capacidad publicada no podría aceptar solicitudes. Con el comando en cola, Integración responde `202` igual y la solicitud se entrega cuando Operaciones vuelve.
- **Respuesta a contrapropuestas y cancelaciones por mensajería.** Descartada: el contrato devuelve la solicitud actualizada en el momento, y la decisión depende del estado actual en Operaciones. Se acepta la dependencia síncrona y se cubre con `503` y la idempotencia de punta a punta.
- **Copia local de la disponibilidad de choferes en Operaciones** (actualizada por eventos de Recursos). Descartada por ahora: la consulta síncrona es más simple y siempre está al día. Se puede revisar en D10 si la dependencia de Recursos resulta problemática.
- **Reintentos en cada capa** (gateway, servicios y clientes). Descartada: amplifica la carga sobre la dependencia que justamente está fallando (teórico 7).
- **Kafka.** Descartada: no hace falta retener un log para reprocesar ni tenemos su volumen. RabbitMQ es el broker de la materia y cubre colas de trabajo, pub/sub, DLX y confirmaciones.
- **Publicar el evento directamente después de guardar** (dual write). Descartada: si el servicio cae entre la escritura y la publicación, el evento se pierde (teórico 2).

## Consecuencias
- **Ganamos:**
  - Las operaciones que no necesitan esperar (avisos, actividad, estado para el consumidor externo) no bloquean al usuario ni dependen de que el consumidor esté arriba.
  - La capacidad publicada sigue aceptando solicitudes aunque Operaciones esté caída.
  - No se pierden eventos y los duplicados no producen efectos dobles.
  - Una operación se puede seguir de punta a punta por su `correlation_id`.
- **Pagamos:**
  - Consistencia eventual: los avisos, la actividad y el estado que ve el consumidor externo llegan con algunos segundos de demora.
  - Hay que operar RabbitMQ, los relays del outbox y las DLQ.
  - Todos los consumidores necesitan lógica de idempotencia.
  - Operaciones depende de forma síncrona de Recursos e Identidad para asignar y aceptar (se trata en D10).

## Cómo se verifica
- Detener Notificaciones y finalizar un recorrido: la operación no se bloquea, y al levantar Notificaciones los avisos aparecen.
- Detener Operaciones y crear una solicitud externa: responde `202`; al levantar Operaciones, la solicitud aparece para revisión.
- Publicar dos veces el mismo `recorrido.finalizado`: Recursos registra una sola actividad.
- Publicar un mensaje con payload inválido: termina en la DLQ sin bloquear los siguientes.
- Con Recursos lento (más de 800 ms): la asignación se rechaza en el tiempo previsto y no queda colgada.
