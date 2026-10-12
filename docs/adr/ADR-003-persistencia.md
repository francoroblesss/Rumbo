# ADR-003 — Persistencia (D3)

- **Fecha:** 2026-10-04
- **Estado:** aceptada (versión inicial para la Entrega 1; se valida en la Entrega 2) — **reemplazada parcialmente por [ADR-005](ADR-005-mysql-como-motor-relacional.md)**: el motor relacional pasa de PostgreSQL a MySQL. El resto sigue vigente.
- **Revisión 2026-10-04:** se agrega el almacenamiento de Integración de carga (ADR-001).
- **Revisión 2026-10-04:** en PostgreSQL, Operaciones y Recursos pasan de dos *schemas* de una misma base a **dos bases de datos separadas**, por la indicación del profesor en clase de tener **una base de datos por microservicio, no una compartida**. Se mantiene una instancia por motor.

## Contexto
ADR-001 definió cinco servicios, cada uno **único dueño de sus datos** (database per service). La consigna (§2.8) pide que cada servicio elija el almacenamiento que mejor responda a sus patrones de acceso, que el sistema use **al menos dos tipos diferentes (uno relacional y uno no relacional)** y que exista al menos una operación que no pueda duplicar ni perder información. El backend es en **Go**.

Criterios del teórico 2:
- **SQL:** relaciones, JOINs, ACID estricto ("finanzas, reservas"), auditoría.
- **Documental:** dato jerárquico o semiestructurado, que se lee como unidad y con esquema variable.
- **Clave-valor:** caché y sesiones.
- Elegir por estructura del dato, consultas, patrón de acceso, consistencia y madurez del equipo, **nunca "porque escala"**.
- No hace falta una instancia física por servicio: alcanza con un schema o una base separada.

Indicación del profesor en clase: **dos tipos de base de datos** y **una base de datos por microservicio, no una compartida**.

## Decisión

### Resumen
| Servicio | Tipo | Motor | Dónde |
| --- | --- | --- | --- |
| **Operaciones** | Relacional | **PostgreSQL** | Base `operaciones` |
| **Recursos** | Relacional | **PostgreSQL** | Base `recursos` |
| **Identidad y clientes** | Documental | **MongoDB** | Base `identidad` |
| **Notificaciones** | Documental | **MongoDB** | Base `notificaciones` |
| **Integración de carga** | Documental | **MongoDB** | Base `integracion` |

- **Una base de datos por microservicio**, con **un usuario por servicio** que solo puede conectarse a la suya. Así la base misma impide que un servicio lea los datos de otro. Las bases viven en **una instancia por motor** (un PostgreSQL con dos bases y un MongoDB con tres).
- Drivers en Go: `pgx` (PostgreSQL) y `go.mongodb.org/mongo-driver` (el mismo de la clase 2 del práctico).
- **Lo que no es persistencia de negocio** no está acá: el índice de búsqueda (D6), la caché (D7) y el broker (D5) son copias o canales derivados, nunca fuente de verdad.

### Operaciones — PostgreSQL
**Por qué relacional:** la asignación es una **reserva de recursos en el tiempo**, el caso que el teórico 2 da para ACID estricto. El modelo tiene muchas relaciones: solicitud → traslados → tramos ↔ paradas ↔ recorrido ↔ asignación. Además, en la **misma transacción local** que el cambio de negocio tienen que guardarse las claves de idempotencia del contrato (ADR-002) y el **outbox** de eventos (teórico 2: guardar y publicar no es atómico; se resuelve con outbox).

**Por qué PostgreSQL y no MySQL:** PostgreSQL tiene **exclusion constraints** sobre rangos de tiempo. La base misma rechaza dos asignaciones activas superpuestas del mismo chofer o del mismo vehículo, incluido el margen mínimo entre recorridos. La garantía de la operación crítica no depende de que el código de la aplicación tome bien un bloqueo. El detalle va en D4.
```sql
-- Idea (detalle en D4); requiere la extensión btree_gist
EXCLUDE USING gist (chofer_id  WITH =, periodo WITH &&) WHERE (activa)
EXCLUDE USING gist (vehiculo_id WITH =, periodo WITH &&) WHERE (activa)
-- periodo = tstzrange(inicio_recorrido, fin_recorrido + margen_minimo)
```

**Datos (tablas principales):** `solicitudes` (con la copia de los datos de las personas y la carga), `contrapropuestas`, `planes`, `suspensiones`, `recorridos_modelo`, `traslados`, `tramos`, `tramo_personas` (abordó o ausente), `recorridos`, `paradas`, `asignaciones`, `incidentes`, `claves_idempotencia`, `outbox`. Los IDs de clientes, personas, choferes y vehículos son **referencias lógicas** a otros servicios, sin claves foráneas.

**Patrones de acceso:**
| Acceso | Soporte |
| --- | --- |
| Solicitud por id, con su traslado | Clave primaria |
| Cambios para el consumidor externo (`actualizado_desde`) | Índice (`organizacion_id`, `actualizado_en`) |
| Solicitudes pendientes de revisión, contrapropuestas por vencer | Índice por (`estado`, fecha) |
| Tramos de una fecha para planificar | Índice por (`fecha`, `estado`) |
| "Mi recorrido de hoy" del chofer | Índice por (`chofer_id`, `fecha`) |
| Agenda de un chofer o vehículo en un rango (conflictos) | Índice GiST sobre `periodo`, el mismo del constraint |
| Asignar: validar superposición, capacidad por tramo y horarios en una transacción | Exclusion constraints + transacción local |
| Recorridos afectados por la ausencia de un chofer | Índice por (`chofer_id`, `periodo`) |
| Búsqueda con texto y filtros combinados sobre miles de traslados | **No acá**: índice de búsqueda derivado (D6) |

### Recursos — PostgreSQL
**Por qué relacional:** la preliquidación maneja **montos, correcciones con motivo y cierres que no se reabren** (finanzas y auditoría). Hay que cruzar la actividad con las tarifas y con quién aporta el vehículo. La jornada semanal y las ausencias son consultas por rango de fechas.

**Datos:** `choferes` (con referencia a su usuario), `jornadas`, `ausencias`, `vehiculos`, `tarifas` (con vigencia), `actividades`, `preliquidaciones`, `ajustes`, `mensajes_procesados`.

**Patrones de acceso:**
| Acceso | Soporte |
| --- | --- |
| Disponibilidad de un chofer en un rango (jornada − ausencias) | Índices por (`chofer_id`, día) y (`chofer_id`, rango de ausencia) |
| Datos de un vehículo (capacidad, adaptado, activo, propietario) | Clave primaria |
| Registrar la actividad de un recorrido finalizado (consumidor de eventos) | **Restricción única** sobre `recorrido_id`: un evento duplicado no registra dos veces (teórico 4) |
| Preliquidación de un chofer en un mes | Índice (`chofer_id`, `fecha`) sobre actividades + tarifas vigentes |
| Cerrar una preliquidación | Transacción local; período cerrado inmutable, correcciones como `ajustes` del período siguiente |

### Identidad y clientes — MongoDB
**Por qué documental:** el **padrón tiene esquema variable según el tipo de servicio**: el paciente lleva silla de ruedas, el alumno su parada asignada, el empleado su edificio. Cada persona se lee y se escribe como una unidad, sin JOINs. Se usa un **campo discriminador** (`tipo`) y validación con JSON Schema por tipo (teórico 2: "esquema flexible ≠ sin diseño").

**Colecciones:**
- `organizaciones`: tipo de servicio y credenciales de API embebidas, con el secreto guardado como hash.
- `usuarios`: email, hash de contraseña, roles embebidos, organización.
- `personas`: padrón **referenciado** por `organizacion_id`. No va embebido en la organización porque crece sin límite (teórico 2).

**Patrones de acceso:**
| Acceso | Soporte |
| --- | --- |
| Login por email; token de API por `client_id` | Índices únicos `usuarios.email` y `organizaciones.api.client_id` |
| Padrón de una organización (listar, buscar por apellido o documento) | Índices (`organizacion_id`, `apellido`) y único (`organizacion_id`, `documento`) |
| Datos de varias personas al aceptar una solicitud (copia en Operaciones) | Consulta por `_id` con `$in` |

### Notificaciones — MongoDB
**Por qué documental:** los avisos son **independientes y sin relaciones**. Se escriben de a uno (al consumir eventos) y se leen siempre por usuario y fecha. Cada aviso es un documento.

**Colección:** `avisos` (`destinatario` = usuario, organización o rol operador; `tipo`, `mensaje`, `referencia`, `leido`, `creado_en`, `evento_id`). El destinatario sale del evento, sin consultar a Identidad (ADR-004).

**Patrones de acceso:**
| Acceso | Soporte |
| --- | --- |
| Bandeja de un usuario (sus avisos + los de su organización o rol), no leídos primero, recientes primero | Índice (`destinatario`, `leido`, `creado_en` desc) |
| Marcar como leído | Actualización de un documento (atómica por documento) |
| Consumir un evento sin duplicar el aviso | Índice único sobre (`evento_id`, `usuario_id`) |

### Integración de carga — MongoDB
**Por qué documental:** cada solicitud externa se lee y se escribe **como una unidad** (el consumidor consulta una solicitud o la lista de las que cambiaron). MongoDB es **atómico por documento** (teórico 2), y eso permite guardar en una sola escritura atómica, sin transacciones multi-documento:
- la solicitud tal como llegó;
- su clave de idempotencia;
- la marca de "pendiente de entregar a Operaciones" (cumple la función de outbox: la intención de entregarla no se pierde aunque el servicio caiga justo después).

**Colección:** `solicitudes_externas` (`organizacion_id`, `referencia_externa`, el pedido, `clave_idempotencia` + hash del cuerpo + respuesta, `id_interno` en Operaciones, `estado` y `traslado` según el último evento recibido con su `version`, `pendiente_envio`, `creado_en`, `actualizado_en`).

**Patrones de acceso:**
| Acceso | Soporte |
| --- | --- |
| `POST` con `Idempotency-Key`: devolver la misma respuesta si se repite | Índice único (`organizacion_id`, `clave_idempotencia`) |
| `GET /{id}` del consumidor | `_id` + filtro por `organizacion_id` |
| Polling por `actualizado_desde` | Índice (`organizacion_id`, `actualizado_en`) |
| Solicitudes pendientes de entregar a Operaciones (reintento) | Índice parcial sobre `pendiente_envio = true` |
| Aplicar un cambio de estado sin retroceder por eventos tardíos | Actualización condicional por `version` (teórico 5: aplicar solo si la versión del evento es mayor) |

## Alternativas consideradas
- **MySQL en lugar de PostgreSQL.** Es el más conocido por el equipo (DSW), pero no tiene exclusion constraints: la no superposición habría que garantizarla en la aplicación, con transacción y bloqueo por recurso (`SELECT ... FOR UPDATE`). Es más código y más fácil de romper en la operación más crítica.
- **Todo en PostgreSQL** (con JSONB para el padrón). Funcionaría, pero no cumple el requisito de dos tipos de almacenamiento.
- **Operaciones en MongoDB.** Descartada: las relaciones entre solicitudes, tramos, paradas y asignaciones exigirían `$lookup` frecuentes (señal de mal modelado, teórico 2) o transacciones multi-documento, que son caras. Además, la no superposición no se puede expresar como restricción.
- **Recursos en MongoDB** (chofer con jornada y ausencias embebidas). Descartada: las ausencias crecen sin límite y la preliquidación pide garantías y auditoría de tipo financiero.
- **Notificaciones en Redis.** Descartada: la bandeja tiene que persistir, y el teórico 2 ubica clave-valor en caché y sesiones.
- **Dos schemas en una misma base de PostgreSQL** (versión anterior de este ADR). Descartada por la indicación del profesor de tener una base por microservicio.
- **Una instancia (contenedor) por servicio.** Descartada por el costo de levantar cinco motores en las notebooks. La regla de una base por microservicio se cumple igual: cada base es independiente y tiene su propio usuario.
- **Event Sourcing para el ciclo de vida del traslado** (teórico 8). Descartada por ahora: el dominio pide estados y resultados, no reconstruir decisiones a partir de un historial de eventos. Suma versionado de eventos, proyecciones y operación de un event store sin un problema concreto que resolver.

## Consecuencias / limitaciones aceptadas
- **Instancia compartida por motor:** si cae el PostgreSQL, caen a la vez Operaciones y Recursos (lo mismo con MongoDB para Identidad, Notificaciones e Integración de carga). Lo que se comparte es la falla y los recursos, no el despliegue ni los datos. Aceptable para el proyecto.
- **Sin JOINs entre servicios:** las respuestas compuestas se arman por API (API Composition, teórico 2), y Operaciones guarda una copia de los datos de las personas que puede quedar desactualizada a propósito (ADR-001).
- **La disponibilidad de choferes y vehículos no está protegida por la base de Operaciones:** la jornada y las ausencias son de Recursos. La carrera se acepta y se resuelve por evento ("requiere revisión"), como dice ADR-001.
- **MongoDB sin transacciones multi-documento:** cada operación de Identidad, de Notificaciones y de Integración de carga toca un solo documento.
- **El estado de una solicitud externa es una copia:** Integración de carga lo actualiza por eventos de Operaciones, así que puede atrasarse unos segundos.
- **El equipo conoce menos PostgreSQL** que MySQL: la parte nueva es la exclusion constraint, y es acotada.
- **Retención:** sin política de borrado por ahora (avisos, outbox ya publicado, claves de idempotencia vencidas). Se define junto con capacidad (D13).

## Cómo se verifica
- Cada servicio se conecta con su propio usuario y no puede conectarse a la base de otro servicio.
- Insertar dos asignaciones activas superpuestas del mismo chofer o vehículo falla en la base (test de integración contra PostgreSQL real, consigna §2.11).
- Un evento de "recorrido finalizado" o de aviso entregado dos veces produce un solo registro.
- Las consultas frecuentes de cada servicio usan índice (verificar con `EXPLAIN`).
