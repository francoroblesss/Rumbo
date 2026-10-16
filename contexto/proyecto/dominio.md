# Rumbo — Dominio y alcance

> Fuente: `Rumbo.docx` + `Rumbo - Ampliaciones del dominio.docx` (ya borrados; todo su contenido de dominio está acá), con todos sus pendientes cerrados por el grupo el 3/10/2026.
> Cuando este archivo y los .docx difieren, **manda este archivo**. Solo contiene dominio y alcance; nada técnico (servicios, tecnologías y almacenamiento van en los ADR).

## 1. Qué es Rumbo
Rumbo es **una única plataforma** de gestión operativa de **traslados programados de personas o carga**. Rumbo aporta los **choferes** y los **vehículos** y presta el servicio a **clientes organizacionales** (clínicas, colegios, empresas, empresas de stock). Con Rumbo se administran los clientes y lo que hay que trasladar, se generan traslados recurrentes o puntuales, se planifican recorridos, se asignan choferes y vehículos, se gestiona la ejecución y los incidentes de cada traslado y se registra la actividad para la preliquidación de los choferes.

El negocio en una línea: **qué** se traslada + **desde dónde** + **hacia dónde** + **cuándo** + **con qué vehículo** + **quién lo conduce** + **cómo se ejecutó**.

- Proyecto académico: **una sola plataforma**. No se plantea venderlo, ni varias instalaciones, ni un esquema multiempresa comercial.
- No es Uber: no conecta oferta y demanda de distintas empresas, no vende pasajes ni viajes sueltos al público.
- La **acción principal** es **planificar y asignar recorridos** respetando disponibilidad, capacidad, accesibilidad y horarios comprometidos, y luego ejecutarlos.

## 2. Tipos de servicio
Un mismo modelo de traslados para los cuatro servicios. Los paneles comparten funciones y cambian los formularios y opciones según el tipo de cliente.

| Servicio | Cliente | Qué se traslada | Forma habitual |
| --- | --- | --- | --- |
| **Pacientes** | Clínicas | Pacientes (pueden tener necesidades como silla de ruedas) | Recurrente (ej. diálisis L/M/V) y puntual |
| **Escolar** | Colegios | Alumnos (cada uno con su parada asignada) | Recurrente (L a V) |
| **Empresa privada** | Empresas | Empleados (ej. edificio donde trabajan) | Puntual (ej. 5 empleados al aeropuerto) y recurrente |
| **Stock** | Empresas de stock (incluido el **sistema del otro grupo**) | Carga entre sucursales | Puntual |

**Todos los servicios usan choferes y vehículos de Rumbo.** Los clientes no aportan vehículos ni choferes.

## 3. Actores y roles
| Rol | Quién es | Qué puede hacer |
| --- | --- | --- |
| **Cliente** | Usuario de una organización cliente | Mantener el **padrón** de personas de su organización; crear solicitudes (puntuales o recurrentes); responder contrapropuestas; pedir modificaciones y cancelaciones; ver el estado **solo de su organización**. No elige chofer ni vehículo ni arma recorridos. |
| **Operador** | Personal de Rumbo | Revisar solicitudes (aceptar / contraproponer / rechazar); crear y modificar planes recurrentes y suspensiones; cargar solicitudes y personas en nombre de un cliente; armar recorridos; asignar y reasignar choferes y vehículos; aprobar cambios con menos de 24 h; cancelar; registrar y resolver incidentes. |
| **Chofer** | Conductor de Rumbo | Ver sus recorridos; iniciar y cerrar un recorrido; marcar llegada a parada, abordaje y ausencia por persona; reportar incidentes y demoras; informar los km al cerrar. |
| **Administrador** | Personal de Rumbo | Usuarios y roles; alta de clientes (organizaciones) y sus usuarios; choferes; vehículos; tarifas de preliquidación; revisar y corregir actividad; cerrar preliquidaciones. Además puede hacer todo lo del operador. |

- Una misma persona puede tener **más de un rol**. Eso no implica dar los mismos permisos a todos.
- **No hay auto-registro**: el administrador da de alta cada organización (con su tipo de servicio) y sus usuarios. El sistema del otro grupo es un cliente de tipo stock con credenciales propias.
- **Persona transportada ≠ cliente**: el pasajero (paciente, alumno, empleado) **no tiene cuenta** ni solicita nada. Pide siempre la organización.

## 4. Conceptos del dominio
| Concepto | Definición |
| --- | --- |
| **Cliente** | Organización que contrata a Rumbo, con un tipo de servicio (pacientes, escolar, empresa privada, stock). |
| **Persona transportada** | Integrante del padrón de un cliente: nombre, documento, teléfono/contacto, domicilio, observaciones y necesidades (silla de ruedas, parada asignada, edificio de trabajo, etc.). La carga el cliente desde su panel o el operador en su nombre. |
| **Carga** | Lo que se traslada en un servicio de stock: **descripción, cantidad de bultos y peso total (kg)** obligatorios; volumen opcional; referencia externa del cliente. Rumbo **no gestiona el inventario** del cliente, solo registra qué traslada. |
| **Solicitud** | Pedido del cliente: qué personas (de su padrón) o qué carga, origen, destino, hora pedida (+ tolerancia opcional), ida sola o ida y vuelta, y fecha puntual **o** recurrencia (días de la semana + desde/hasta). |
| **Plan recurrente** | Lo crea el **operador** al aceptar una solicitud recurrente. Define qué traslados se repiten y desde cuándo; a partir de él Rumbo genera los traslados concretos. |
| **Traslado** | Movimiento concreto en una fecha: **una o varias personas (o una carga) con el mismo origen y destino**. Puede ser solo de ida o de ida y vuelta. 8 pacientes con 8 domicilios = 8 traslados; 5 empleados de la oficina al aeropuerto = 1 traslado de 5 personas. |
| **Tramo** | La **ida** o la **vuelta** de un traslado. Ida y vuelta pertenecen al **mismo traslado**, pero cada tramo tiene su propio estado, horario y asignación. |
| **Recorrido** | Ejecución ordenada de paradas por **un chofer con un vehículo**. Agrupa tramos de varios traslados (ej. buscar a Juan, Pedro y Marta y llevarlos a la clínica). **Traslado ≠ recorrido.** |
| **Recorrido modelo** | Recorrido base de un plan recurrente (paradas, orden, chofer y vehículo habituales). Rumbo lo replica en cada día generado. |
| **Parada** | Lugar del recorrido donde se recoge o se deja a personas o carga, con su hora estimada. |
| **Chofer** | Conductor de Rumbo, con disponibilidad semanal habitual, ausencias y opcionalmente **un vehículo propio**. |
| **Vehículo** | Patente, tipo, capacidad de pasajeros, si está adaptado para silla de ruedas, capacidad de carga (kg), propietario (**un chofer** o **Rumbo**) y si está activo o con baja temporal. |
| **Asignación** | Chofer + vehículo que ejecutan un recorrido. |
| **Incidente** | Hecho anómalo registrado: tipo, tramo o recorrido afectado, descripción y resultado. |
| **Actividad del chofer** | Horas y km de cada recorrido; base de la preliquidación. |
| **Preliquidación** | Cálculo mensual de lo que corresponde a cada chofer, revisado y cerrado por el administrador. |

## 5. Recursos: choferes y vehículos
- Hay dos modalidades: el **chofer conduce su propio vehículo** o el **chofer conduce un vehículo de Rumbo**.
- Un chofer tiene **0 o 1 vehículo propio** y además puede ser asignado a vehículos de Rumbo. Al asignar, el operador elige el vehículo (por defecto el propio, si tiene). **Un vehículo propio solo lo maneja su dueño.**
- **Disponibilidad del chofer:** jornada semanal habitual (días y franjas) menos ausencias puntuales registradas (enfermedad, licencia).
- **Disponibilidad del vehículo:** activo, sin baja temporal y sin otro recorrido superpuesto.
- **Flota sí, mantenimiento no:** Rumbo conoce los vehículos como recursos, pero no gestiona service, reparaciones, taller ni repuestos. Una baja temporal solo marca que el vehículo no está disponible.

## 6. Flujo de una solicitud
```
RECIBIDA
  │ el operador revisa disponibilidad
  ├─> ACEPTADA              hay disponibilidad en el horario pedido → se crean los traslados
  │                         (o el plan, si es recurrente); el horario pasa a ser un COMPROMISO
  ├─> CONTRAPROPUESTA       Rumbo propone otro horario
  │     ├─> ACEPTADA        el cliente acepta la alternativa
  │     └─> RECHAZADA_POR_CLIENTE   el cliente la rechaza, o no responde en 24 h (vencida)
  ├─> RECHAZADA             Rumbo no puede atenderla (con motivo)
  └─> CANCELADA             el cliente la retira antes de que sea aceptada
```
- Ante falta de disponibilidad, Rumbo **prefiere contraproponer antes que rechazar**. El rechazo sigue siendo posible.
- **Rumbo nunca cambia unilateralmente el horario pedido.** Si propone otro, el cliente acepta o rechaza.
- La contrapropuesta **vence a las 24 h** sin respuesta; al vencer, Rumbo libera el horario propuesto.
- El mismo ciclo aplica a **todos los clientes**, incluido el sistema del otro grupo (que también puede recibir y responder contrapropuestas).
- **Horario pedido:** en la **ida** es la hora de **llegada al destino** (turno, escuela, vuelo); en la **vuelta** es la hora de **recogida**. El cliente puede indicar una **tolerancia** en minutos (ej. ±30). Sin tolerancia, la hora es estricta.

## 7. Planes recurrentes
- El cliente pide la necesidad recurrente (ej. "estos 8 pacientes, L/M/V, llegar 07:00 a la clínica, vuelta 12:00"). Al aceptarla, el **operador crea el plan**.
- **Vigencia:** fecha de inicio obligatoria y fecha de fin opcional.
- **Generación:** Rumbo genera los traslados concretos de los **próximos 7 días** (ventana móvil).
- **Recorrido modelo:** el operador arma una vez el recorrido base (paradas, orden, chofer y vehículo habituales) y Rumbo lo replica cada día. Las excepciones (ausencias, reasignaciones) se ajustan día por día.
- **Suspensiones:** rango de fechas con motivo (vacaciones, internación, etc.); no se generan traslados en ese período. **No hay calendario de feriados**: un feriado se resuelve cancelando la ocurrencia puntual.
- **Una ocurrencia** se puede cancelar o modificar sin alterar el plan.
- **Modificación del plan** (horario, domicilio, destino), con una fecha desde la cual rige:
  - los traslados ya generados **sin asignar** se actualizan solos;
  - los **asignados** quedan marcados **"requiere revisión"** y el operador decide si mantiene el recurso, reasigna o resuelve el conflicto;
  - los **en curso o terminados** no se tocan.

## 8. Planificación y asignación (la acción principal)
- El operador agrupa tramos en **recorridos**, ordena las paradas, estima la **hora de cada parada** y asigna **chofer + vehículo**.
- **Solo se agrupa** en un recorrido lo que es **del mismo cliente** y **del mismo tipo** (personas **o** carga, nunca mezcladas).
- **Validaciones al asignar:**
  1. **Horario comprometido:** cada parada cumple la hora pedida dentro de su tolerancia.
  2. **Sin compromisos incompatibles:** un chofer o un vehículo no puede tener dos recorridos superpuestos. Entre el fin de un recorrido y el inicio del siguiente del mismo chofer o vehículo debe haber un **margen mínimo configurable** (ej. 15 min). Varios traslados a la vez **sí** pueden ir en el mismo vehículo si están en el mismo recorrido.
  3. **Disponibilidad:** el recorrido cae dentro de la jornada del chofer, el chofer no tiene ausencia y el vehículo está activo.
  4. **Capacidad por tramo:** entre cada par de paradas consecutivas, la ocupación a bordo (personas, contando **acompañantes**, o kg de carga) no puede superar la capacidad del vehículo. Si unos bajan antes de que suban otros, se aprovecha el lugar.
  5. **Accesibilidad:** una persona en silla de ruedas exige un vehículo adaptado.
- Al **cambiar un horario** (por solicitud, plan o incidente), Rumbo **detecta los conflictos** (ej. "Carlos ya tiene otro recorrido a esa hora") y el operador los resuelve.
- **Ida y vuelta se asignan por separado:** la vuelta puede hacerla otro chofer o vehículo. Un regreso pendiente no obliga al chofer de la ida a esperar.
- **Vuelta a confirmar:** un traslado puede incluir una vuelta **con horario pendiente** (ej. no se sabe cuándo termina el tratamiento). Es distinto de "sin vuelta". El horario se confirma después, y el operador debe programarla y asignarla antes de ejecutarla.
- Los **tiempos** (duración de tramos y viajes entre paradas) los **estima el operador**. Rumbo no calcula rutas ni distancias (ver consultas al profesor).

**Clima en traslados al aeropuerto.** Una ubicación puede marcarse como el aeropuerto (Córdoba, `SACO`); en esta etapa lo marca el operador al aceptar la solicitud. Para un traslado con origen o destino en el aeropuerto cuya hora comprometida esté dentro de las próximas 6 h, Rumbo consulta la operabilidad del aeropuerto (al aceptar y al iniciar el recorrido). Si no es operable, el traslado queda con una alerta climática (con sus motivos), se avisa al operador y al cliente, y el operador decide mantener, proponer otro horario o cancelar. Si el servicio de clima no responde, se sigue sin el dato y la alerta queda "clima no verificado".

## 9. Modificaciones y cancelaciones
- **24 h o más de anticipación:** el cliente puede pedir el cambio o cancelar. Si el cambio afecta horarios o recursos, se revisa la disponibilidad.
- **Menos de 24 h:** es una **excepción**, y el operador la aprueba o no.
- La cancelación **siempre se registra**, aunque sea tardía, para que el chofer no intente un servicio que el cliente ya avisó que no necesita.
- El plazo se aplica **a cada tramo**. Así se puede cancelar la vuelta después de haber hecho la ida.
- Un **tramo ya iniciado no se cancela**: lo que pase se registra como incidente.
- **No hay multas ni cobros** por cancelación.

## 10. Estados de la operación
Se distinguen tres ciclos: la **solicitud** (qué pasó con el pedido, §6), el **traslado y sus tramos** (qué pasó con el servicio) y el **recorrido** (cómo avanzó la ejecución de sus paradas).

**Tramo (ida o vuelta):**
| Estado | Significado |
| --- | --- |
| `PENDIENTE_HORARIO` | Solo para la vuelta a confirmar: falta definir el horario |
| `PROGRAMADO` | Tiene fecha y hora y está en un recorrido |
| `ASIGNADO` | Su recorrido tiene chofer y vehículo |
| `EN_CURSO` | El chofer lo inició |
| `COMPLETADO` | Llegó al destino |
| `CANCELADO` | Cancelado antes de iniciar (por el cliente o el operador) — final |
| `NO_REALIZADO` | No viajó nadie (ausencia) o no se pudo retirar la carga — final |
| `INTERRUMPIDO` | Empezó y no terminó (ej. avería) — final |

**Traslado (resultado derivado de sus tramos):**
| Resultado | Cuándo |
| --- | --- |
| `EN_PROCESO` | Mientras algún tramo no llegó a un estado final |
| `COMPLETADO` | Al menos un tramo se completó y el resto está completado o cancelado (ej. ida hecha, vuelta cancelada por el cliente: **completado**, no incompleto) |
| `COMPLETADO_PARCIAL` | Al menos un tramo se completó y otro quedó `NO_REALIZADO` o `INTERRUMPIDO` |
| `CANCELADO` | Todos los tramos se cancelaron |
| `NO_REALIZADO` | Ningún tramo se completó y hubo ausencia |
| `INTERRUMPIDO` | Ningún tramo se completó y alguno quedó interrumpido |

- El detalle de cada tramo se conserva: en el ejemplo, "ida completada, vuelta cancelada por decisión del cliente". La vuelta cancelada nunca aparece como realizada.
- **Ausencia por persona:** en un traslado de varias personas se registra quién abordó y quién no. El tramo se completa con quienes viajaron; queda `NO_REALIZADO` solo si no viajó nadie.
- **Demora:** no es un estado, es una **condición**: minutos de retraso registrados sobre el tramo o el recorrido.

**Recorrido:**
| Estado | Significado |
| --- | --- |
| `EN_PLANIFICACION` | El operador está armando las paradas |
| `PLANIFICADO` | Paradas y horarios definidos |
| `ASIGNADO` | Chofer y vehículo confirmados |
| `EN_CURSO` | El chofer pulsó "Iniciar recorrido" |
| `FINALIZADO` | El chofer lo cerró; puede finalizar aunque alguna parada haya tenido ausencias |
| `CANCELADO` | Cancelado antes de iniciar |
| `INTERRUMPIDO` | No pudo completarse (ej. avería) |

**Parada:** `PENDIENTE` → `LLEGADA` → `COMPLETADA`, o `OMITIDA` (por ausencia o cancelación).

## 11. El día del traslado (chofer)
El chofer ve **"Mi recorrido de hoy"** (ej. "Ruta 08 — Carlos: 07:00 Juan Pérez, 07:15 María González, 07:30 Pedro López, 08:00 Centro Médico") y registra:
- iniciar recorrido;
- llegué a la parada;
- persona abordó / persona ausente;
- parada completada;
- reportar incidente o demora;
- cerrar el recorrido informando los km.

**No hay seguimiento GPS en tiempo real** (fuera de alcance; es candidato a bonus).

## 12. Incidentes
- Rumbo **registra** cada incidente: tipo (ausencia de pasajero, avería, chofer ausente, demora, cancelación tardía, otro), tramo o recorrido afectado, descripción y resultado.
- **Antes de iniciar:** si falta el chofer o se averió el vehículo, el **operador reasigna** chofer o vehículo, con las mismas validaciones de §8.
- **Durante el viaje** (avería con pasajeros a bordo): se registra, el tramo y el recorrido quedan `INTERRUMPIDO`, y el **rescate o transbordo se coordina fuera del sistema**.
- **Registrar un incidente ≠ resolver sus consecuencias.** Hay que distinguir una vuelta cancelada por el cliente de un servicio interrumpido por avería.

## 13. Avisos
Por ahora los avisos llegan **solo dentro de Rumbo**: cada usuario los ve en su bandeja al ingresar (ver consultas al profesor).
- **Al cliente:** cambios de su solicitud (aceptada, contrapropuesta, rechazada, vencida) y confirmación de modificaciones y cancelaciones. El avance de la ejecución **no se avisa**; el cliente lo consulta en su panel.
- **Al chofer:** recorrido nuevo asignado, reasignación, cancelación o cambio de horario de una parada.
- **Al operador:** solicitudes nuevas, traslados que requieren revisión por cambio de plan, incidentes reportados, pedidos de cambio con menos de 24 h y contrapropuestas respondidas o vencidas.

## 13 bis. Búsqueda de traslados
- **Quién:** el operador y el administrador buscan entre **todos** los traslados; el cliente, **solo** entre los de su organización.
- **Texto libre:** nombre de la persona transportada, dirección de origen o destino, descripción de la carga, referencia externa.
- **Filtros:** cliente, tipo de servicio, rango de fechas, estado, chofer, vehículo, origen o destino.
- **Orden:** por fecha y hora (por defecto) o por relevancia del texto. Resultados paginados.
- La búsqueda **ayuda a encontrar**: cualquier acción sobre un traslado (cancelar, reasignar) se valida contra el dato real, no contra el resultado de la búsqueda.

## 14. Actividad y preliquidación de choferes
- **Período mensual, por chofer:** `total = horas × tarifa_hora + km × tarifa_km`.
- `tarifa_km` depende de **quién aporta el vehículo**: el propio del chofer (tarifa más alta, porque cubre su vehículo) o uno de Rumbo. Las tarifas las define el administrador.
- **Horas:** desde "Iniciar recorrido" hasta "Cerrar recorrido". Incluye esperas y paradas con ausencia, que **se reconocen**.
- **Km:** los informa el chofer al cerrar el recorrido.
- **Sin adicional por traslado:** llevar 3 pacientes juntos es un solo recorrido y no se paga tres veces.
- **Cancelación tardía:** si el chofer ya había iniciado el recorrido, se reconoce lo trabajado; si no, nada.
- **Cierre:** el administrador revisa la actividad, puede **corregir horas o km con motivo** y **cierra** la preliquidación. Un período cerrado **no se reabre**: los errores posteriores se ajustan en el período siguiente. **Cerrar no es pagar ni emitir un recibo.**

## 15. Integración con otros grupos
- **Capacidad que Rumbo publica: "Solicitar traslado"** (genérica desde el 11/10/2026, ver registro de cambios). Un sistema externo pide **trasladar una cosa desde un origen hasta un destino**: solo `cosa` (descripción y cantidad), `origen` y `destino` son obligatorios; peso, volumen, hora, tolerancia y referencia externa son opcionales. Rumbo responde "solicitud recibida". **Si no se indicó hora, el operador propone una** (contrapropuesta, §6). Desde ahí sigue el flujo normal: revisión, planificación, asignación, ejecución y resultado. Adentro de Rumbo se trata como un traslado de carga. El otro sistema **inicia una operación real** dentro de Rumbo, que es lo que exige la consigna.
- **Capacidad que Rumbo consume:** la API de Clima del sistema de aeropuerto (contrato fijado en `docs/contracts/consumidos/`). Se usa en la regla de clima de §8 (ADR-012).

## 16. Fuera de alcance
- Facturación y cobros a clientes, incluidas multas por cancelación.
- Mantenimiento mecánico de vehículos (service, reparaciones, taller, repuestos).
- RRHH integral: impuestos, aportes, recibos legales, AFIP, vacaciones, obra social. Solo hay preliquidación.
- Pago a choferes y emisión de recibos.
- Rescate o transbordo durante el viaje (solo se registra).
- Seguimiento GPS en tiempo real (posible bonus).
- Cálculo automático de rutas, tiempos o distancias (por ahora; ver consultas).
- Inventario de la carga del cliente.
- Vehículos o choferes aportados por clientes.
- Auto-registro de clientes y cuentas para pasajeros.
- Venta del sistema, multiempresa comercial o instalaciones por empresa.

## 17. Consultas al profesor y dependencias externas
| # | Tema | Decisión vigente mientras tanto |
| --- | --- | --- |
| 1 | ¿Se puede calcular automáticamente tiempos y distancias con un servicio de mapas? | El operador estima los tiempos de cada parada. |
| 2 | ¿Los avisos deberían llegar también por email u otro canal externo? | Solo dentro de Rumbo (bandeja de notificaciones). |

## Registro de cambios
| Fecha | Cambio | Motivo |
| --- | --- | --- |
| 2026-10-11 | §15: la capacidad publicada pasa de "Solicitar traslado de carga" (empresa de stock; descripción, bultos, peso, hora y referencia obligatorios) a **"Solicitar traslado"** genérico (cosa, origen y destino; el resto opcional; sin hora, contrapropuesta) | Corrección de la Entrega 1 del profesor: "traslado = función con 3 parámetros: cosa, inicio y destino; más abstracto para que gestión de almacenamiento nos consuma" (ADR-006) |
| 2026-10-14 | §8: regla de clima para traslados al aeropuerto (consulta de operabilidad, alerta climática y decisión del operador) | Consumo de la capacidad de clima del otro grupo (D9, ADR-012) |
