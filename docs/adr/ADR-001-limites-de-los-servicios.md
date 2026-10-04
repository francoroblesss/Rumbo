# ADR-001 — Límites de los servicios (D1)

- **Fecha:** 2026-10-04
- **Estado:** aceptada (preliminar para la Entrega 1; se valida en la Entrega 2)
- **Revisión 2026-10-04:** se agrega el servicio **Integración de carga**, que separa la capacidad publicada en su propio microservicio, por indicación del profesor en clase. La revisión se hizo antes de entregar el ADR, por eso no se creó uno nuevo que lo reemplace.

## Contexto
Rumbo gestiona traslados programados de personas o carga con choferes y vehículos propios (ver `SPEC.md`). La consigna exige **al menos tres microservicios y un API Gateway** (§2.1). Hay que decidir cómo se separan las responsabilidades, cómo se relacionan los servicios y quién es dueño de cada dato.

Puntos de partida:
- En clase, el profesor sugirió preliminarmente tres servicios: **Personas**, **Insumos y gestión de vehículos** y **Notificaciones**. Lo hizo antes de conocer el detalle del dominio. Ninguno de los tres contiene solicitudes, planes, traslados, recorridos ni asignaciones, que son el comportamiento principal.
- La operación con garantías de consistencia es **asignar chofer y vehículo a un recorrido**: no puede haber compromisos superpuestos ni ocupación por encima de la capacidad.
- En clase, el profesor indicó que la **funcionalidad publicada** para el otro grupo conviene **separarla en otro microservicio**, simple.

## Criterios
1. **Un servicio = una capacidad de negocio**, con una responsabilidad que se pueda nombrar en una oración (teórico 1).
2. **Database per service:** cada dato tiene un único dueño. Los demás acceden solo por la API o los eventos de ese dueño, y referencian por ID lógico, sin claves foráneas entre servicios (teórico 2).
3. **Lo que debe ser atómico vive en el mismo servicio** (teórico 2). Así la operación crítica no necesita transacciones distribuidas.
4. **No crear un monolito distribuido** (teórico 1): ningún par de servicios debe necesitar desplegarse junto.
5. **Complejidad mínima:** separar solo donde hay un motivo concreto: distinto dueño del dato, distinto ritmo de cambio o distinta tolerancia a la consistencia.

## Decisión
Cinco microservicios detrás de un **API Gateway**:

| Servicio | Responsabilidad (una oración) | Datos de los que es dueño |
| --- | --- | --- |
| **Identidad y clientes** | Saber quién es cada usuario, qué puede hacer y a qué organización y padrón pertenece. | Usuarios, roles y credenciales; organizaciones cliente (con tipo de servicio); padrón de personas transportadas (datos, domicilio, necesidades). |
| **Operaciones** | Recibir lo que hay que trasladar, planificarlo, asignarlo y seguir su ejecución. | Solicitudes y contrapropuestas; planes recurrentes, suspensiones y recorridos modelo; traslados y tramos; recorridos y paradas; **asignaciones**; incidentes y demoras. |
| **Recursos** | Administrar con qué y con quién se trasladan, y cuánto le corresponde a cada chofer. | Choferes (jornada habitual, ausencias, vehículo propio); vehículos (capacidad de pasajeros y kg, adaptado, propietario, activo o baja temporal); actividad registrada; tarifas; preliquidaciones. |
| **Notificaciones** | Hacer llegar a cada usuario los avisos que le corresponden. | Bandeja de avisos por usuario (leído o no). |
| **Integración de carga** | Atender el contrato público "Solicitar traslado de carga" y traducirlo al modelo interno. | Solicitudes externas: id externo ↔ id interno, claves de idempotencia, último estado conocido para responder al consumidor. |

Correspondencia con la sugerencia del profesor: **Personas → Identidad y clientes** · **Insumos y gestión de vehículos → Recursos** · **Notificaciones → Notificaciones**. Se agrega **Operaciones**, que contiene el núcleo del dominio, e **Integración de carga**, que aísla la capacidad publicada.

### Relaciones entre servicios (preliminar; los mecanismos concretos van en D5)
| Desde → hacia | Para qué | Naturaleza |
| --- | --- | --- |
| Gateway → Identidad y clientes | Login y validación de identidad en el borde | Síncrona |
| Operaciones → Identidad y clientes | Al aceptar una solicitud, toma una **copia de los datos de las personas** (domicilio, necesidades) | Síncrona; la copia queda en el traslado |
| Operaciones → Recursos | Al asignar, consulta la disponibilidad del chofer (jornada y ausencias) y los datos del vehículo (capacidad, adaptado, activo) | Síncrona (primera versión) |
| Recursos → Operaciones | Ausencia de chofer o baja de vehículo registrada → los recorridos afectados quedan **"requiere revisión"** | Asíncrona (evento) |
| Operaciones → Recursos | Recorrido finalizado (chofer, vehículo, horas, km) → se registra la actividad para la preliquidación | Asíncrona (evento) |
| Operaciones / Recursos → Notificaciones | Hechos que generan avisos (solicitud aceptada o contrapropuesta, recorrido asignado, incidente, etc.) | Asíncrona (eventos) |
| Grupo externo → Gateway → Integración de carga | Capacidad publicada "Solicitar traslado de carga" (D8) | Síncrona |
| Integración de carga → Operaciones | Entregar cada solicitud nueva para que la revise un operador; reenviar respuestas a contrapropuestas y cancelaciones (Operaciones decide) | Asíncrona para la entrega de solicitudes; la decisión sobre cada cambio la toma Operaciones (mecanismo en D5) |
| Operaciones → Integración de carga | Cambios de estado de solicitudes y traslados de origen externo, para responder el polling | Asíncrona (eventos) |
| Operaciones → sistema del aeropuerto | Capacidad consumida (clima); la llamada sale de Operaciones, no del frontend ni del gateway (consigna §4.1) | Síncrona (D9) |

### Decisiones que se derivan
- **La operación crítica es local a Operaciones.** La superposición de recorridos, la capacidad por tramo y los horarios comprometidos se validan con datos que son todos de Operaciones. De Recursos se consultan la jornada, las ausencias y la capacidad del vehículo, que cambian poco.
- **Copia de los datos de las personas en el traslado.** Un traslado conserva el domicilio y las necesidades con los que se planificó. Si cambia el padrón, no se alteran traslados ya aceptados: los cambios permanentes se aplican modificando el plan (`dominio.md` §7).
- **Carrera aceptada:** si se registra una ausencia de chofer después de asignarlo, el evento marca sus recorridos "requiere revisión" y el operador reasigna. No se bloquea Recursos durante la asignación.
- **La preliquidación vive en Recursos:** comparte sujeto (el chofer), dueño (el administrador) y datos (tarifa según quién aporta el vehículo). Recibe la actividad por eventos; la **consistencia eventual** es aceptable porque se cierra una vez por mes.
- **El sistema externo es un cliente más:** es una organización de tipo stock en Identidad y clientes, con credenciales propias.
- **Integración de carga es una capa de traducción, no de negocio:** no acepta, rechaza ni planifica. Valida la forma del pedido, garantiza la idempotencia, traduce el contrato público al modelo de Operaciones (teórico 8: un contexto traduce lo que recibe en vez de filtrar su vocabulario) y responde el polling con su propia copia del estado. Las reglas (24 h, vencimiento de la contrapropuesta, estados válidos) las decide Operaciones, el dueño del dato (teórico 5: "el servicio dueño valida antes de producir un efecto de negocio").
- **Fuera de los servicios:** búsqueda, caché y mensajería son infraestructura al servicio de estos límites (D6, D7, D5), no servicios de negocio.

## Alternativas consideradas
- **Los tres servicios del profesor, literalmente.** Descartada: el núcleo (solicitudes, recorridos, asignaciones) habría que repartirlo entre "Personas" e "Insumos", y la asignación quedaría partida entre servicios. Eso exige coordinación distribuida para la operación crítica.
- **Cinco servicios, con Liquidaciones aparte.** Descartada por ahora: la preliquidación tiene poco comportamiento propio y los mismos dueño y sujeto que Recursos. Separarla suma un servicio que operar sin un problema concreto que resolver. Se puede revisar si crece.
- **Preliquidación en Operaciones.** Descartada: Operaciones ya concentra el núcleo. La preliquidación cambia por motivos distintos (tarifas, cierres mensuales) y tiene otro dueño (el administrador).
- **Choferes en Identidad (por ser usuarios).** Descartada: la cuenta del chofer (login, rol) es identidad, pero su jornada, ausencias y vehículo son un recurso operativo. El chofer de Recursos referencia a su usuario por ID.
- **Capacidad publicada dentro de Operaciones** (versión anterior de este ADR). Descartada por indicación del profesor, y además porque:
  - el contrato público queda acoplado al modelo interno;
  - el polling del otro grupo carga al servicio núcleo;
  - para la Presentación Grupal habría que desplegar todo Operaciones en la nube para publicar la capacidad.
- **Integración de carga como fachada sin estado.** Descartada: cada consulta de polling llegaría a Operaciones, y si Operaciones cae, la capacidad deja de aceptar solicitudes.
- **Separar Planificación y Ejecución en dos servicios.** Descartada: comparten recorridos y asignaciones. Una reasignación por incidente modifica la planificación durante la ejecución, así que habría que desplegarlos juntos (monolito distribuido).

## Consecuencias
- **Ganamos:** la operación crítica es atómica dentro de un solo servicio; cada dato tiene un dueño claro; Notificaciones y la preliquidación, que toleran demora, se desacoplan por eventos; la capacidad publicada es un servicio chico que se despliega solo, mantiene el contrato estable aunque cambie el modelo interno y **sigue aceptando solicitudes aunque Operaciones esté caída** (las entrega cuando vuelve).
- **Pagamos:** Operaciones es el servicio más grande y concentra el riesgo (es candidato natural para un estilo interno más estructurado en D2); hay dependencias síncronas de Operaciones hacia Recursos e Identidad (si fallan, falla la asignación o la aceptación; se trata en D10); las copias de datos de personas pueden quedar desactualizadas a propósito; hay consistencia eventual en avisos, actividad y revisiones por ausencia; el estado que ve el consumidor externo es una copia que llega por eventos, así que puede atrasarse unos segundos respecto de Operaciones; hay un servicio más que operar.

## Cómo se verifica
- Cada servicio arranca solo, en su puerto, con su propio almacenamiento.
- Ningún servicio lee la base de otro.
- Una asignación concurrente del mismo chofer o vehículo, hecha desde dos operadores, solo confirma una (D4).
- Si se detiene Notificaciones o el consumidor de actividad, no se bloquea la operación: los avisos y la actividad se procesan al volver.
