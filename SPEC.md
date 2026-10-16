# Rumbo — Especificación de alcance y requisitos

> Define las funcionalidades, las reglas de negocio y los criterios de aceptación del sistema (consigna §3).
> Cada funcionalidad (`F-xx`) indica qué reglas (`R-xx`) aplica; los criterios de aceptación (`CA-xx`) referencian ambas.
> Versión 1 — Entrega 1 (2026-10-04). Revisión 2026-10-11: capacidad publicada genérica (F-28, ADR-006).

## 1. Alcance

### 1.1 Qué es Rumbo
Rumbo es una plataforma única de gestión operativa de **traslados programados de personas o carga**. Rumbo aporta los choferes y los vehículos y presta el servicio a organizaciones cliente: clínicas (pacientes), colegios (alumnos), empresas privadas (empleados) y empresas de stock (carga entre sucursales). La acción principal es **planificar y asignar recorridos** respetando disponibilidad, capacidad, accesibilidad y horarios comprometidos, y luego ejecutarlos.

### 1.2 Incluido
- Padrón de personas transportadas de cada cliente.
- Solicitudes de traslado puntuales y recurrentes, con revisión, contrapropuesta de horario y rechazo.
- Planes recurrentes con generación automática de traslados, suspensiones y recorrido modelo.
- Planificación de recorridos y asignación de chofer y vehículo con validaciones.
- Ejecución del recorrido por el chofer (paradas, abordajes, ausencias, demoras, km) e incidentes.
- Modificaciones y cancelaciones con plazo de 24 h.
- Avisos dentro de Rumbo para clientes, choferes y operadores.
- Búsqueda de traslados.
- Preliquidación mensual de choferes.
- Capacidad publicada para otros sistemas: **Solicitar traslado** de una cosa desde un origen hasta un destino (`docs/contracts/`, versión 2).
- Consumo de condiciones climáticas del sistema de aeropuerto (flujo a definir con su contrato).

### 1.3 Fuera de alcance
- Facturación y cobros a clientes, incluidas multas por cancelación.
- Mantenimiento mecánico de vehículos.
- RRHH integral (impuestos, aportes, recibos legales, vacaciones), pago a choferes y emisión de recibos.
- Rescate o transbordo durante el viaje: solo se registra el incidente.
- Seguimiento GPS en tiempo real.
- Cálculo automático de rutas, tiempos y distancias.
- Inventario de la carga del cliente.
- Vehículos o choferes aportados por clientes.
- Auto-registro de clientes y cuentas para pasajeros.
- Venta del sistema o esquema multiempresa.

## 2. Actores y roles
| Rol | Descripción |
| --- | --- |
| **Cliente** | Usuario de una organización cliente. Solo ve los datos de su organización. |
| **Operador** | Personal de Rumbo que revisa solicitudes, planifica, asigna y controla la operación. |
| **Chofer** | Conductor de Rumbo que ejecuta los recorridos. |
| **Administrador** | Personal de Rumbo que administra usuarios, clientes, recursos y tarifas, y cierra preliquidaciones. Puede hacer todo lo del operador. |
| **Sistema externo de stock** | Sistema de otro grupo que usa la capacidad publicada como un cliente de tipo stock. |

- Una persona puede tener más de un rol.
- La persona transportada **no** es usuaria del sistema.

## 3. Funcionalidades

### Cliente
| Id | Funcionalidad | Reglas |
| --- | --- | --- |
| F-01 | **Gestionar el padrón:** alta, modificación y consulta de las personas de su organización (datos, domicilio, necesidades según el tipo de servicio). El operador también puede hacerlo en su nombre. | R-01 |
| F-02 | **Solicitar un traslado puntual:** personas del padrón o carga, origen, destino, fecha, hora pedida, tolerancia opcional y si es solo ida, ida y vuelta o vuelta a confirmar. | R-02, R-03, R-04, R-05 |
| F-03 | **Solicitar un traslado recurrente:** igual que F-02, con días de la semana y vigencia (desde y, opcionalmente, hasta). | R-02, R-03, R-04, R-05 |
| F-04 | **Responder una contrapropuesta** de horario (aceptar o rechazar) antes de que venza. | R-07, R-08 |
| F-05 | **Pedir la modificación** de un traslado o de un tramo. | R-15, R-16 |
| F-06 | **Cancelar** una solicitud, un traslado o un tramo. | R-15, R-16, R-17 |
| F-07 | **Consultar el estado** de sus solicitudes y traslados. | R-01 |

### Operador
| Id | Funcionalidad | Reglas |
| --- | --- | --- |
| F-08 | **Revisar una solicitud:** aceptarla, contraproponer otro horario o rechazarla con motivo. Puede cargar solicitudes en nombre de un cliente. | R-06, R-07, R-09 |
| F-09 | **Gestionar planes recurrentes:** crear el plan al aceptar una solicitud recurrente, armar su recorrido modelo, registrar suspensiones, modificar el plan y cancelar o modificar una ocurrencia. | R-10, R-11, R-12, R-13 |
| F-10 | **Planificar recorridos:** agrupar tramos, ordenar las paradas y estimar la hora de cada parada. | R-18, R-19, R-20 |
| F-11 | **Asignar y reasignar chofer y vehículo** a un recorrido o a un tramo. | R-19 a R-24 |
| F-12 | **Programar una vuelta a confirmar:** fijar el horario y asignarla antes de ejecutarla. | R-05, R-19 a R-24 |
| F-13 | **Aprobar o rechazar cambios y cancelaciones con menos de 24 h.** | R-16 |
| F-14 | **Resolver traslados que requieren revisión y conflictos de horario.** | R-13, R-25 |
| F-15 | **Registrar y resolver incidentes**, incluida la reasignación antes de iniciar. | R-30, R-31 |

### Chofer
| Id | Funcionalidad | Reglas |
| --- | --- | --- |
| F-16 | **Ver "Mi recorrido de hoy"** con sus paradas y horarios. | — |
| F-17 | **Registrar la ejecución:** iniciar el recorrido, marcar llegada a la parada, abordaje o ausencia por persona, parada completada y cerrar el recorrido informando los km. | R-26, R-27, R-28 |
| F-18 | **Reportar un incidente o una demora.** | R-29, R-30 |

### Administrador
| Id | Funcionalidad | Reglas |
| --- | --- | --- |
| F-19 | **Gestionar usuarios y roles.** | — |
| F-20 | **Gestionar clientes** (organizaciones con su tipo de servicio) y sus usuarios. | R-01 |
| F-21 | **Gestionar choferes:** jornada semanal habitual, ausencias y vehículo propio. | R-21, R-22 |
| F-22 | **Gestionar vehículos:** patente, tipo, capacidad de pasajeros y de carga, adaptado para silla de ruedas, propietario, activo o baja temporal. | R-22, R-23 |
| F-23 | **Gestionar tarifas** (por hora y por km según quién aporta el vehículo). | R-33 |
| F-24 | **Revisar, corregir y cerrar la preliquidación** mensual de cada chofer. | R-33 a R-37 |

### Transversales
| Id | Funcionalidad | Reglas |
| --- | --- | --- |
| F-25 | **Generar traslados de los planes** para los próximos 7 días (automático). | R-10, R-12 |
| F-26 | **Bandeja de avisos** para cliente, chofer y operador. | R-38 |
| F-27 | **Buscar traslados** con texto libre, filtros, orden y paginación. | R-39, R-40 |
| F-28 | **Capacidad publicada "Solicitar traslado"** para sistemas externos: trasladar una **cosa** desde un **origen** hasta un **destino** (tres datos obligatorios; hora y demás opcionales). Contrato en `docs/contracts/` (versión 3.0.0, rutas sin versión: `/traslados`). | R-02, R-04, R-06 a R-09, R-15 a R-17 |
| F-29 | **Alerta climática en traslados al aeropuerto** con la capacidad de clima del sistema de aeropuerto (consulta al aceptar y al iniciar el recorrido) | R-41 |

## 4. Reglas de negocio

### Clientes y solicitudes
- **R-01** Un cliente solo ve y modifica datos de su propia organización.
- **R-02** Una solicitud indica personas del padrón del cliente **o** carga, nunca ambas.
- **R-03** En la **ida**, la hora pedida es la de **llegada al destino**. En la **vuelta**, es la de **recogida**.
- **R-04** El cliente puede indicar una tolerancia en minutos. Sin tolerancia, la hora es estricta.
- **R-05** Un traslado puede ser solo de ida, de ida y vuelta, o de ida con **vuelta a confirmar**. La vuelta a confirmar no se ejecuta hasta que el operador fija su horario y la asigna.
- **R-06** Recibir una solicitud no la confirma. El horario recién es un compromiso cuando la solicitud queda `ACEPTADA`.
- **R-07** Rumbo **nunca cambia el horario pedido sin el acuerdo del cliente**. Si no puede cumplirlo, contrapropone otro o rechaza. Ante falta de disponibilidad, se prefiere contraproponer antes que rechazar.
- **R-08** Una contrapropuesta **vence a las 24 h**. Sin respuesta, la solicitud pasa a `RECHAZADA_POR_CLIENTE` y se libera el horario propuesto.
- **R-09** Un rechazo de Rumbo siempre lleva motivo.
- **R-14** Para la carga son obligatorios la descripción, la cantidad de bultos y el peso total (kg); el volumen es opcional.

### Planes recurrentes
- **R-10** Un plan tiene fecha de inicio obligatoria y fecha de fin opcional. Rumbo genera los traslados de los próximos 7 días.
- **R-11** El plan tiene un recorrido modelo (paradas, orden, chofer y vehículo habituales) que se replica cada día generado.
- **R-12** En un período de suspensión no se generan traslados. No hay calendario de feriados: un feriado se resuelve cancelando la ocurrencia.
- **R-13** Si se modifica un plan, desde la fecha en que rige:
  - los traslados generados sin asignar se actualizan solos;
  - los asignados quedan "requiere revisión";
  - los en curso o terminados no cambian.

### Modificaciones y cancelaciones
- **R-15** Con 24 h o más de anticipación, el cliente puede modificar o cancelar. Si el cambio afecta horarios o recursos, se revisa la disponibilidad.
- **R-16** Con menos de 24 h, el cambio o la cancelación es una excepción que aprueba o no un operador. La cancelación tardía siempre queda registrada. El plazo se aplica a cada tramo.
- **R-17** Un tramo ya iniciado no se puede cancelar: lo que ocurra se registra como incidente.

### Planificación y asignación
- **R-18** Un recorrido solo agrupa tramos **del mismo cliente** y **del mismo tipo** (personas o carga).
- **R-19** Cada parada debe cumplir la hora comprometida dentro de su tolerancia.
- **R-20** Los tiempos de cada parada los estima el operador.
- **R-21** Un chofer solo puede asignarse dentro de su jornada habitual y si no tiene una ausencia registrada en ese horario.
- **R-22** Un chofer o un vehículo **no puede tener dos recorridos superpuestos**. Entre el fin de un recorrido y el inicio del siguiente del mismo chofer o vehículo debe haber un margen mínimo configurable (por ejemplo, 15 min). Varios traslados a la vez sí pueden ir en el mismo vehículo si están en el mismo recorrido. Solo se asignan vehículos activos y sin baja temporal.
- **R-23** **Capacidad por tramo:** entre cada par de paradas consecutivas, la ocupación a bordo no puede superar la capacidad del vehículo. Se cuentan personas, incluidos los acompañantes, o kg de carga. Una persona en silla de ruedas exige un vehículo adaptado.
- **R-24** Un chofer tiene como máximo un vehículo propio, que solo maneja él, y puede manejar vehículos de Rumbo. Ida y vuelta se asignan por separado: pueden tener chofer o vehículo distintos.
- **R-25** Al cambiar un horario, Rumbo detecta los recorridos en conflicto y el operador los resuelve.
- **R-41** En un traslado con origen o destino en el aeropuerto (`SACO`, marcado por el operador al aceptar) y hora comprometida dentro de las próximas 6 h, Rumbo consulta la operabilidad del aeropuerto al aceptar y al iniciar el recorrido: si no es operable, el traslado queda con una alerta climática con sus motivos, se avisa al operador y al cliente, y el operador decide mantener, proponer otro horario o cancelar; si el servicio de clima no responde, se sigue sin el dato y la alerta queda "clima no verificado".

### Ejecución e incidentes
- **R-26** En un traslado de varias personas, la ausencia se registra **por persona**. El tramo se completa con quienes viajaron y queda `NO_REALIZADO` solo si no viajó nadie.
- **R-27** Un recorrido puede finalizar aunque alguna parada haya tenido ausencias.
- **R-28** Al cerrar el recorrido, el chofer informa los km recorridos.
- **R-29** La demora no es un estado: es una condición con minutos de retraso, registrada sobre el tramo o el recorrido.
- **R-30** Todo incidente registra tipo, tramo o recorrido afectado, descripción y resultado. Registrar un incidente no es lo mismo que resolver sus consecuencias.
- **R-31** Antes de iniciar, si falta el chofer o se avería el vehículo, el operador reasigna con las mismas validaciones (R-21 a R-24). Durante el viaje, una avería deja el tramo y el recorrido `INTERRUMPIDO`; el rescate se coordina fuera del sistema.
- **R-32** El resultado de un traslado se deriva de sus tramos (ver §5.2). Una vuelta cancelada nunca aparece como realizada.

### Preliquidación
- **R-33** Preliquidación mensual por chofer: `horas × tarifa_hora + km × tarifa_km`. La tarifa por km depende de si el vehículo es propio del chofer (tarifa más alta) o de Rumbo.
- **R-34** Las horas se cuentan desde "Iniciar recorrido" hasta "Cerrar recorrido" e incluyen las esperas y las paradas con ausencia.
- **R-35** No hay adicional por traslado: un recorrido con varios traslados se paga una vez.
- **R-36** Ante una cancelación tardía, si el chofer ya había iniciado el recorrido, se reconoce lo trabajado; si no, nada.
- **R-37** El administrador puede corregir horas o km con motivo antes de cerrar. Un período cerrado **no se reabre**: los errores posteriores se ajustan en el período siguiente. Cerrar no es pagar.

### Avisos y búsqueda
- **R-38** Los avisos llegan solo dentro de Rumbo:
  - **Al cliente:** cambios de su solicitud y confirmación de modificaciones y cancelaciones.
  - **Al chofer:** asignación, reasignación, cancelación o cambio de horario de sus recorridos.
  - **Al operador:** solicitudes nuevas, traslados que requieren revisión, incidentes, cambios con menos de 24 h y contrapropuestas respondidas o vencidas.
- **R-39** En la búsqueda, el cliente solo encuentra traslados de su organización; el operador y el administrador, todos.
- **R-40** La búsqueda ayuda a encontrar: cualquier acción sobre un traslado se valida contra el dato real, no contra el resultado de la búsqueda.

## 5. Estados y transiciones

### 5.1 Solicitud
```
RECIBIDA ──operador──> ACEPTADA                   (crea traslados o plan; horario = compromiso)
         ──operador──> CONTRAPROPUESTA ──cliente acepta────────> ACEPTADA
                                       ──cliente rechaza o 24 h──> RECHAZADA_POR_CLIENTE
         ──operador──> RECHAZADA                  (con motivo)
         ──cliente───> CANCELADA                  (antes de ser aceptada)
```

### 5.2 Tramo y traslado
**Tramo (ida o vuelta):**
`PENDIENTE_HORARIO` (solo vuelta a confirmar) → `PROGRAMADO` → `ASIGNADO` → `EN_CURSO` → `COMPLETADO`. Finales alternativos: `CANCELADO` (antes de iniciar), `NO_REALIZADO` (no viajó nadie o no se pudo retirar la carga) e `INTERRUMPIDO` (empezó y no terminó).

**Traslado (derivado de sus tramos):**
| Resultado | Condición |
| --- | --- |
| `EN_PROCESO` | Algún tramo no llegó a un estado final |
| `COMPLETADO` | Al menos un tramo completado y el resto completado o cancelado |
| `COMPLETADO_PARCIAL` | Al menos un tramo completado y otro `NO_REALIZADO` o `INTERRUMPIDO` |
| `CANCELADO` | Todos los tramos cancelados |
| `NO_REALIZADO` | Ningún tramo completado y hubo ausencia |
| `INTERRUMPIDO` | Ningún tramo completado y alguno interrumpido |

### 5.3 Recorrido y parada
**Recorrido:** `EN_PLANIFICACION` → `PLANIFICADO` → `ASIGNADO` → `EN_CURSO` → `FINALIZADO`. Alternativos: `CANCELADO` (antes de iniciar) e `INTERRUMPIDO`.

**Parada:** `PENDIENTE` → `LLEGADA` → `COMPLETADA`, o `OMITIDA` (ausencia o cancelación).

## 6. Criterios de aceptación

### Solicitudes
- **CA-01** (F-02, R-06) **Dado** un cliente con personas en su padrón, **cuando** crea una solicitud puntual válida, **entonces** queda `RECIBIDA`, no se crea ningún traslado todavía y el operador recibe un aviso.
- **CA-02** (F-08, R-07) **Dada** una solicitud `RECIBIDA` sin disponibilidad en el horario pedido, **cuando** el operador propone otro horario, **entonces** la solicitud queda `CONTRAPROPUESTA` con la hora propuesta y su vencimiento, y el horario original no cambia.
- **CA-03** (F-04, R-08) **Dada** una contrapropuesta sin respuesta, **cuando** pasan 24 h, **entonces** la solicitud queda `RECHAZADA_POR_CLIENTE` y el cliente recibe un aviso.
- **CA-04** (F-04) **Dada** una contrapropuesta vigente, **cuando** el cliente la acepta, **entonces** la solicitud queda `ACEPTADA` con la hora propuesta como compromiso y se crean sus traslados.
- **CA-05** (F-02, R-02, R-14) **Dada** una solicitud de carga sin peso, **cuando** se envía, **entonces** se rechaza indicando el dato faltante y no se registra.

### Planes
- **CA-06** (F-09, F-25, R-10) **Dado** un plan L/M/V vigente desde hoy, **cuando** corre la generación, **entonces** existen los traslados de los próximos 7 días que caen en lunes, miércoles o viernes, con el recorrido modelo aplicado.
- **CA-07** (F-09, R-12) **Dado** un plan con una suspensión del 10 al 20, **cuando** corre la generación, **entonces** no se crea ningún traslado en ese rango.
- **CA-08** (F-09, R-13) **Dado** un plan con traslados ya asignados la semana en curso, **cuando** el operador cambia el horario desde el miércoles, **entonces**:
  - los traslados de miércoles en adelante sin asignar toman el nuevo horario;
  - los asignados quedan "requiere revisión";
  - los anteriores al miércoles no cambian.

### Asignación
- **CA-09** (F-11, R-22) **Dado** un chofer asignado a un recorrido de 07:00 a 09:00 con margen de 15 min, **cuando** se lo intenta asignar a otro recorrido que empieza a las 09:10, **entonces** la asignación se rechaza por superposición.
- **CA-10** (F-11, R-22) **Dados** dos operadores que asignan **al mismo tiempo** el mismo vehículo a recorridos superpuestos, **cuando** ambos confirman, **entonces** solo una asignación queda registrada y la otra recibe un rechazo.
- **CA-11** (F-11, R-23) **Dado** un vehículo de 4 pasajeros y un recorrido donde suben 3 personas, bajan 2 y suben otras 3, **cuando** se asigna, **entonces** se acepta, porque la ocupación nunca supera 4 en ningún tramo.
- **CA-12** (F-11, R-23) **Dado** un recorrido con una persona en silla de ruedas, **cuando** se asigna un vehículo no adaptado, **entonces** la asignación se rechaza.
- **CA-13** (F-11, R-21) **Dado** un chofer con una ausencia registrada el día 15, **cuando** se lo intenta asignar ese día, **entonces** la asignación se rechaza.
- **CA-14** (F-11, R-18) **Dados** tramos de dos clientes distintos, **cuando** se intenta agruparlos en un recorrido, **entonces** se rechaza.

### Modificaciones y cancelaciones
- **CA-15** (F-06, R-15) **Dado** un traslado aceptado para dentro de 48 h, **cuando** el cliente lo cancela, **entonces** queda `CANCELADO`, se libera su lugar en el recorrido y el chofer recibe un aviso.
- **CA-16** (F-06, F-13, R-16) **Dado** un traslado para dentro de 6 h, **cuando** el cliente lo cancela, **entonces** la cancelación queda registrada y pendiente de aprobación del operador, que recibe un aviso.
- **CA-17** (F-06, R-17) **Dado** un tramo `EN_CURSO`, **cuando** el cliente intenta cancelarlo, **entonces** se rechaza.

### Ejecución y resultados
- **CA-18** (F-17, R-26, R-32) **Dado** un traslado de ida y vuelta con la ida `COMPLETADO`, **cuando** el cliente cancela la vuelta con anticipación, **entonces** el traslado queda `COMPLETADO`, con la ida completada y la vuelta cancelada.
- **CA-19** (F-17, R-26, R-32) **Dado** un traslado con la ida `COMPLETADO`, **cuando** el chofer marca la persona ausente en la vuelta, **entonces** la vuelta queda `NO_REALIZADO` y el traslado `COMPLETADO_PARCIAL`.
- **CA-20** (F-17, R-26) **Dado** un traslado de 5 empleados donde se presentan 4, **cuando** el chofer completa la parada, **entonces** el tramo se completa y queda registrada la ausencia de la quinta persona.
- **CA-21** (F-18, R-31) **Dado** un recorrido `EN_CURSO`, **cuando** el chofer reporta una avería, **entonces** el recorrido y sus tramos en curso quedan `INTERRUMPIDO`, el incidente queda registrado y el operador recibe un aviso.

### Preliquidación
- **CA-22** (F-24, R-33, R-34) **Dado** un chofer con vehículo propio que en el mes hizo recorridos por 10 h y 200 km, **cuando** se calcula la preliquidación, **entonces** el total es `10 × tarifa_hora + 200 × tarifa_km_vehiculo_propio`.
- **CA-23** (F-24, R-37) **Dada** una preliquidación cerrada, **cuando** se detecta un error de km, **entonces** no se reabre y la corrección se registra como ajuste del período siguiente.
- **CA-24** (F-24, R-35) **Dado** un recorrido con 3 pacientes, **cuando** se registra la actividad, **entonces** cuenta como una sola actividad con las horas y km del recorrido.

### Búsqueda, avisos e integración
- **CA-25** (F-27, R-39) **Dado** un cliente, **cuando** busca traslados por un nombre que existe en otra organización, **entonces** no obtiene resultados de esa organización.
- **CA-26** (F-27) **Dados** más de 20 traslados que cumplen un filtro, **cuando** se busca, **entonces** los resultados vienen paginados y ordenados por fecha y hora.
- **CA-27** (F-28) **Dado** el sistema externo, **cuando** repite la misma solicitud con la misma clave de idempotencia, **entonces** se registra una sola solicitud y recibe la misma respuesta.
- **CA-28** (F-26, R-38) **Dado** un recorrido reasignado a otro chofer, **cuando** se confirma, **entonces** ambos choferes reciben un aviso en su bandeja.
- **CA-29** (F-28, R-07) **Dado** el sistema externo, **cuando** solicita un traslado solo con cosa, origen y destino (sin hora), **entonces** queda `RECIBIDA` y, al revisarla, el operador le propone un horario (`CONTRAPROPUESTA`) en lugar de fijarlo por su cuenta.
- **CA-30** (F-29, R-41) **Dado** un traslado al aeropuerto dentro de las próximas 6 h y el aeropuerto no operable, **cuando** el operador acepta la solicitud, **entonces** el traslado queda con alerta climática y sus motivos, y se avisa al operador y al cliente.
- **CA-31** (F-29, R-41) **Dado** el servicio de clima caído, **cuando** el operador acepta una solicitud al aeropuerto, **entonces** la aceptación se completa y el traslado queda "clima no verificado".

## 7. Bonus anunciados
Anunciados en la Entrega 2 (consigna §7). Se implementan para la Presentación Grupal; tienen que estar integrados, documentados, testeados y defendidos.
| Bonus | Qué se hace | Cómo se verifica |
| --- | --- | --- |
| **Actualizaciones en tiempo real** | El operador y el cliente ven en la aplicación web los cambios de estado de solicitudes y traslados apenas ocurren, sin recargar, a partir de los eventos que ya publica Operaciones | Un cambio de estado aparece en la pantalla abierta en menos de 2 s |
| **Tests de punta a punta (e2e)** | Tests automatizados del flujo principal a través del sistema levantado: solicitud por el contrato público → gateway → Integración → Operaciones → cambio de estado visible por consulta | Corren en el CI contra el sistema levantado con Docker Compose |

## 8. Registro de cambios
| Fecha | Cambio | Motivo |
| --- | --- | --- |
| 2026-10-11 | F-28: la capacidad publicada pasa de "Solicitar traslado de carga" (carga con bultos y peso obligatorios) a **"Solicitar traslado"** genérico: `cosa`, `origen` y `destino` obligatorios; sin hora pedida, Rumbo propone una con una contrapropuesta. Se agrega CA-29 | Corrección de la Entrega 1 (ADR-006) |
| 2026-10-16 | F-28: las rutas de la capacidad publicada dejan de llevar versión (`/v2/traslados` → `/traslados`); el contrato pasa a la versión 3.0.0 sin otros cambios | Indicación del profesor en clase (ADR-019) |
| 2026-10-14 | Se anuncian los bonus (§7): actualizaciones en tiempo real y tests de punta a punta | Consigna §7: los bonus se anuncian en la Entrega 2 |
| 2026-10-14 | F-29 pasa a "Alerta climática en traslados al aeropuerto". Se agregan R-41 (regla de clima: consulta de operabilidad, alerta climática y decisión del operador) y CA-30 y CA-31 | Consumo de la capacidad de clima del otro grupo (D9, ADR-012) |
