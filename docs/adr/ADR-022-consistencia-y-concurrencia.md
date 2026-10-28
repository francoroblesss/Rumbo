# ADR-022 — Consistencia y concurrencia (D4)

- **Fecha:** 2026-10-28
- **Estado:** aceptada
- **Relacionado:**
  - [ADR-005](ADR-005-mysql-como-motor-relacional.md): transacción InnoDB con `SELECT … FOR UPDATE` por recurso; dejó para D4 el orden de los bloqueos y el reintento;
  - [ADR-021](ADR-021-capacidad-de-autoservicio.md): la reserva de franja, que es la operación crítica de la capacidad publicada.

## Contexto
- La consigna pide:
  - §2.8: al menos **una operación que no pueda duplicar ni perder información**;
  - D4: las operaciones con garantías especiales y cómo se tratan la **concurrencia**, la **duplicación**, la **pérdida** y las **fallas parciales**.
- En Rumbo hay dos recursos escasos que dos pedidos simultáneos no pueden tomar a la vez:
  1. **el lugar de una franja** (capacidad publicada, ADR-021): si quedan 2 lugares y llegan 5 reservas juntas, entran exactamente 2. Es el caso del ejemplo de la consigna ("dos personas no pueden tomar el mismo turno");
  2. **el chofer y el vehículo** al asignar un recorrido (dominio §8): no pueden tener dos recorridos superpuestos. Lo implementa el grupo con los recorridos, siguiendo esta misma decisión.
- Además, una misma intención puede llegar varias veces (reintento después de un timeout, dos réplicas de Integración) y un evento puede publicarse dos veces (outbox at-least-once).
- **Teórico 2:**
  - SQL con ACID para reservas;
  - "lo que debe ser atómico va en el mismo servicio";
  - outbox en la misma transacción local;
  - consumidores idempotentes.
- **Teórico 7:** la clave de idempotencia guarda clave, parámetros y resultado, y hay que definir su alcance, su vigencia y qué pasa con la concurrencia.
- **Teórico 8:** el control optimista (versión esperada) acepta una de dos escrituras concurrentes y obliga a la otra a recargar y volver a evaluar.

## Decisión

### 1. Operaciones con garantías
| Operación | Garantía | Dónde |
| --- | --- | --- |
| **Reservar una franja** | Nunca se reserva más que el cupo; un mismo pedido nunca reserva dos veces; si se confirma, el evento no se pierde | Operaciones, una transacción MySQL |
| **Cancelar una reserva** | El lugar se devuelve exactamente una vez | Operaciones, la misma transacción que la cancelación |
| **Transiciones de una solicitud** (aceptar, contraproponer, cancelar) | Dos actores no aplican transiciones a la vez sobre la misma solicitud | Operaciones, `SELECT … FOR UPDATE` de la solicitud (ya implementado) |
| **Asignar chofer y vehículo** | Un recurso no queda en dos recorridos superpuestos | Operaciones, se implementa con los recorridos (punto 5) |

Todas viven en **Operaciones**, que es dueño de solicitudes, franjas, traslados y asignaciones (ADR-001), así que **no hay transacciones distribuidas ni sagas**: alcanza con una transacción local de MySQL.

### 2. Concurrencia: bloqueo pesimista por fila
- **Reserva**, en una transacción:
  1. resolver la clave de idempotencia;
  2. validar la franja;
  3. tomar la fila de la franja con `INSERT … ON DUPLICATE KEY UPDATE` y después `SELECT … FOR UPDATE`;
  4. verificar el lugar y descontarlo;
  5. insertar la solicitud y el traslado;
  6. dejar el evento en el outbox y guardar la respuesta de la clave.

  Al confirmar se libera el bloqueo. Dos reservas sobre la misma franja **se ordenan en el bloqueo**: la segunda espera y, cuando entra, ve el cupo que dejó la primera. Reservas en franjas distintas no se esperan entre sí.
- **Por qué `ON DUPLICATE KEY UPDATE` y no `INSERT IGNORE`:**
  - si la fila ya existe, `INSERT IGNORE` toma un bloqueo **compartido**, y dos transacciones con el compartido que piden el exclusivo (`FOR UPDATE`) se esperan entre sí (**deadlock**);
  - `ON DUPLICATE KEY UPDATE` toma el **exclusivo** de entrada.
- **Orden de los bloqueos (para que no haya deadlocks):**
  - una transacción que bloquea más de una fila lo hace siempre en el mismo orden: **solicitud → franja** (cancelar) y **chofer → vehículo** (asignar);
  - la reserva solo bloquea la franja.
- **Última defensa en la base:** restricciones `CHECK` en `franjas`: lo reservado nunca supera el cupo ni es negativo. Si el código fallara, la base rechaza la escritura y la transacción se deshace.

### 3. Duplicación
- **`Idempotency-Key` en dos niveles:**
  - Integración la busca en su copia y, si ya está, responde lo mismo sin llamar a Operaciones (índice único `organizacion_id + clave` en MongoDB);
  - Operaciones la guarda **en la misma transacción** que la reserva (clave primaria `organizacion_id + endpoint + clave`).
- **Dos pedidos con la misma clave al mismo tiempo** (dos réplicas):
  - los dos pueden pasar la búsqueda, pero solo uno logra insertar la clave;
  - el otro choca con la clave primaria, su transacción se deshace entera (incluido el lugar que había tomado) y el consumidor recibe `503`;
  - su reintento devuelve la reserva original. **Nunca hay dos reservas para una clave.**
- **La misma clave con otro cuerpo:** `409 CLAVE_IDEMPOTENCIA_REUTILIZADA` (el hash del cuerpo normalizado se compara en los dos servicios).
- **Eventos duplicados:** el consumidor de Integración aplica una vista solo si su `agregado_version` es mayor que la guardada (ADR-004), así que un duplicado o un evento viejo no tiene efecto.

### 4. Pérdida y fallas parciales
| Falla | Qué pasa | Por qué no se pierde ni se duplica nada |
| --- | --- | --- |
| Operaciones falla antes de confirmar | Rollback: no hay reserva, no se tomó lugar, no hay evento | Todo está en una transacción |
| Se corta la respuesta después de confirmar (timeout) | El consumidor recibe `504` o `503` | Reintenta con la misma clave y recibe la misma reserva |
| Integración no guarda su copia después de confirmar | `503` al consumidor | El reintento trae de Operaciones la misma reserva y la copia se guarda |
| RabbitMQ caído | La reserva se confirma igual; el evento espera en el outbox | El relay lo publica cuando vuelve (at-least-once) |
| Integración no recibe un evento a tiempo | Su copia queda atrasada | Se corrige con el próximo evento; nunca retrocede, porque solo aplica versiones mayores |

### 5. Asignación de chofer y vehículo (la implementa el grupo con los recorridos)
Misma receta que la reserva, como fijó ADR-005:
1. En una transacción, bloquear con `SELECT … FOR UPDATE` la fila del **chofer** y después la del **vehículo** (filas de bloqueo por recurso, creadas con el recurso). Siempre en ese orden.
2. Con el bloqueo tomado, verificar que no haya otro recorrido activo del mismo recurso que se superponga, contando el margen mínimo (dominio §8).
3. Guardar la asignación y su evento en el outbox; al confirmar se liberan los bloqueos.
4. Si MySQL detecta un deadlock (error 1213) o vence la espera del bloqueo (`innodb_lock_wait_timeout`, error 1205), se reintenta la transacción completa **una vez**. Si vuelve a fallar, `503` reintentable.
5. Un test de integración contra MySQL real con asignaciones simultáneas del mismo chofer, igual que el de la reserva.

## Alternativas consideradas
- **Control optimista con versión** (`UPDATE franjas SET … WHERE inicio = ? AND version = ?`, teórico 8). Descartada para las franjas:
  - una franja popular es justamente donde chocan muchas reservas;
  - con optimista, cada choque es un fallo que hay que recargar y reintentar;
  - con pesimista, las reservas esperan su turno unos milisegundos y no se pierde trabajo.

  El optimista conviene cuando los choques son raros; acá no lo son.
- **Decremento atómico sin bloqueo explícito** (`UPDATE franjas SET reservados = reservados + 1 WHERE inicio = ? AND reservados < cupo`). Descartada:
  - sirve para un solo contador, pero la regla tiene dos (traslados y kg) y un error distinto según el caso (`SIN_DISPONIBILIDAD` o `EXCEDE_CAPACIDAD`);
  - la regla quedaría en el SQL y no en el dominio.

  El bloqueo deja la regla en `Franja.Reservar`, que se prueba sin base de datos.
- **Bloqueo distribuido** (Redis o `GET_LOCK` de MySQL). Descartada: todo vive en una base, y `FOR UPDATE` se libera solo al confirmar o deshacer (ADR-005).
- **Serializar las reservas en una cola con un único consumidor.** Descartada: devuelve la espera que ADR-021 sacó, y el bloqueo por fila ya serializa solo lo que compite.
- **Nivel de aislamiento `SERIALIZABLE`.** Descartada: bloquea más de lo necesario (toda lectura) para obtener lo mismo que el `FOR UPDATE` de una fila.

## Consecuencias
- **Ganamos:**
  - la garantía de no reservar de más y no duplicar se prueba con un test de concurrencia contra MySQL real;
  - la regla está en el dominio y la base es la última defensa.
- **Pagamos:**
  - las reservas sobre **la misma franja** se atienden de a una. Cada transacción dura unos milisegundos, y la capacidad por franja (4 traslados) hace que la cola de espera sea corta. Se mide con k6 en D13;
  - la fila de la franja se crea en la primera reserva: la primera vez hay un `INSERT` más;
  - el cupo es **por franja y para todos los consumidores externos juntos**: no mira los vehículos reales (ADR-021, limitaciones conocidas).

## Cómo se verifica
- `adaptadores/mysql/mysql_test.go`, contra MySQL 8.4 real (testcontainers):
  - **`TestReservasConcurrentes`**: 20 reservas a la vez sobre una franja con lugar para 3. Entran **exactamente 3**, las otras 17 reciben `SIN_DISPONIBILIDAD` y **ninguna** termina en deadlock ni en otro error. La fila de la franja queda con 3 traslados y 900 kg, y hay 3 traslados guardados.
  - El mismo pedido (misma clave) lanzado 5 veces a la vez **reserva una sola vez**.
  - Una cancelación tardía devuelve el lugar y queda marcada.
  - **`TestTransaccionSeDeshace`**: una reserva que falla después de crear y bloquear la franja no deja ni la fila ni un lugar tomado.
- `aplicacion/solicitudes_test.go`: idempotencia (misma clave y mismo cuerpo, misma clave con otro cuerpo), franja llena, reintento con la franja llena y cancelación que devuelve el lugar.
- `dominio/franja_test.go`: reglas del cupo (traslados y kg), calendario (alineación, anticipación y horizonte) y cancelación externa sin aprobación.
