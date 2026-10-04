# Teórico 4 — Comunicación asíncrona

Fuente (PDF de la cátedra, ya no se guarda local): `comunicacion-asincrona.pdf`

## Síncrono vs asíncrono
- Síncrono: respuesta inmediata, pero **acoplamiento temporal** y latencia acumulada.
- Asíncrono: separa **aceptación** de **procesamiento**. Tres momentos: **aceptación → registro durable → finalización**.
- Criterio: *¿puede el usuario/negocio esperar para conocer este resultado?* Notificaciones, analítica, IA → async. Reserva de inventario, autorización de pago, permisos → normalmente sync.
- **202 Accepted** + `job_id` durable + estados (`pendiente`, `en_proceso`, `completado`, `fallido`). 202 sin persistir = trasladar incertidumbre.

## Modelo de mensajería
Productor → Mensaje → Broker → Destino (cola/suscripción) → Consumidor.
- El **broker no es fuente de verdad** del negocio.
- Desacoples: **espacial**, **temporal**, **de ritmo** (backlog; no crea capacidad infinita).
- **Mensaje como contrato** autocontenido y versionado: `id`, `tipo`, `version`, `ocurrido_en`, `correlation_id` + datos. Agregar campos opcionales = compatible; cambiar significado/eliminar = rompe.
- **Evento** (hecho pasado, `compra.confirmada`, 0..N interesados) vs **Comando** (pide acción, un responsable) vs **Tarea** (unidad de trabajo, un worker).

## Topologías
- **Cola de trabajo / consumidores competidores:** cada mensaje a un consumidor lógico; escalar agregando workers. FIFO no garantiza orden de efectos con requeue + varios consumidores.
- **Pub/Sub:** una cola por suscriptor lógico (fanout/topic), y dentro de cada cola consumidores compiten. El productor no conoce a los destinos. También sirve para **invalidar cachés**.
- **Orden:** entrega ≠ procesamiento ≠ efectos. Si importa, **clave de partición** por agregado; tolerar mensajes tardíos (versiones). No usar un canal global.
- **RabbitMQ:** exchange (`direct` clave exacta, `fanout` a todos, `topic` patrón `compra.*`) + bindings + queues. **Prefetch** (QoS): 1 = fair dispatch pero menos throughput; 0 = sin límite (peligroso).
- **Kafka:** log retenido + polling con offset propio. No es "mejor" universalmente.

## Entrega e idempotencia
- At-most-once (pierde) · **at-least-once** (duplica — la más usada) · exactly-once (difícil extremo a extremo).
- **ACK después del efecto durable.** ACK antes = pérdida lógica.
- Idempotencia: tabla de mensajes procesados, **restricción única** (ej. `entrada.compra_id`), **máquina de estados** (solo `pendiente → emitida`), **clave idempotente** hacia APIs externas. El registro de duplicado va en la **misma transacción** que el efecto.

## Transactional Outbox (ciclo operativo)
Tx local guarda entidad + evento → relay reintenta publicar → marca publicado. Relay puede duplicar ⇒ consumidores idempotentes.
Relay: **Polling Publisher** (simple, demora de sondeo) o **CDC** (Debezium; más infra).

## Reintentos, venenosos, DLQ
- Clasificar: transitorio (timeout, saturación) vs permanente (payload inválido, versión incompatible).
- Retry con límite, **backoff exponencial + jitter**, motivo registrado. Nunca infinito sin demora.
- **Mensaje venenoso** → **DLQ**. La DLQ **no es papelera**: dueño, alerta, retención, procedimiento de reproceso.

## Observabilidad
`correlation_id` propagado; métricas: profundidad y antigüedad de cola, tasa pub/consumo, tiempo hasta el efecto, en vuelo, reintentos, DLQ, **consumer lag**. Backpressure: agregar consumidores / limitar producción / degradar (si el límite es una API externa, escalar consumidores empeora). SLO de negocio ("99 % de entradas emitidas en < 2 min").

## Puntos clave para defender
Async no elimina responsabilidad de informar estado · cola de trabajo vs pub/sub · paralelismo ≠ orden · at-least-once ⇒ idempotencia · ACK tras efecto · Outbox no elimina duplicados · DLQ con proceso operativo.
