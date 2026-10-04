# Teórico 8 — Estilos de arquitectura de backend

Fuente (PDF de la cátedra, ya no se guarda local): `estilos-de-arquitectura-de-backend.pdf`

## Contexto
- Las clases anteriores distribuyeron la plataforma (Catálogo, Búsqueda, Órdenes, Pagos), agregaron un borde de acceso y contuvieron fallos. Pregunta nueva: **¿cómo se organiza el interior de cada servicio** para que cambiar la base, el framework o un proveedor no obligue a reescribir las reglas de negocio?
- **MVC y capas siguen siendo válidos:** con pocas reglas y consultas simples, `Handler → Service → Repository` + base relacional puede ser lo más claro. Evaluar otras estructuras cuando aparecen invariantes, lecturas muy distintas de las escrituras, historial a reconstruir o dependencias externas que cambian.
- Cada estilo responde una pregunta distinta: **DDD** (entender el negocio, reglas, límites) · **Hexagonal / Clean** (dirigir dependencias hacia el núcleo) · **CQRS** (separar lectura/escritura) · **Event Sourcing** (hechos como registro canónico) · **Reactivo** (sistemas responsivos/elásticos por mensajes; programación reactiva = composición asíncrona y I/O no bloqueante).
- **No se adoptan en conjunto:** DDD con CRUD relacional; Hexagonal sin DDD; CQRS sin Event Sourcing; o capas solas si alcanza.

## Capas, dominio y dependencias
- **MVC:** modelo (datos/comportamiento), vista (presentación; en un backend, el JSON), controlador (interpreta la entrada; en HTTP lo hace en parte el Handler).
- **Capas:** `HTTP Handler → Servicio de aplicación → Repositorio → Base de datos`. Ayuda a localizar código, pero **los nombres de carpetas no garantizan una frontera**: si el servicio importa tipos del ORM, las reglas quedan atadas a la persistencia aunque exista `domain/`; si las reglas viven en el Handler, la capa de servicios no protege nada.
- **Flujo de ejecución** (quién invoca a quién) ≠ **dependencias del código** (quién necesita conocer los tipos de quién para compilar). La ejecución puede ir hacia afuera mientras las dependencias apuntan al núcleo mediante interfaces. Hexagonal y Clean lo hacen explícito.
- **Arquitectura** (elementos, responsabilidades, límites) ≠ **DDD** (modelado del dominio) ≠ **CQRS/ES** (tratamiento de lecturas, escrituras y estado) ≠ **reactivo** (modelo de programación) ≠ **framework** (mecanismo). Gin puede actuar como adaptador pero no decide si hay MVC, Hexagonal o Clean: eso lo definen las dependencias que arma el equipo.

## DDD (Domain-Driven Design)
- Para cuando las reglas son lo bastante complejas como para que un modelo técnico genérico produzca errores o cambios costosos. Se trabaja con expertos del dominio.
- **Lenguaje ubicuo:** vocabulario compartido en código, charlas y docs. "Orden confirmada" ≠ "pago autorizado"; llamar a ambos `success` borra una distinción que importa para cancelar, facturar o notificar. **El modelo no es el esquema de la base.**
- **Subdominio:** parte del problema con objetivos y reglas propios. **Contexto delimitado (bounded context):** dónde aplica un modelo y su lenguaje. Ej.: Órdenes (orden y reglas de confirmación) ↔ Pagos (intento y autorización) ↔ Facturación (comprobante, reglas fiscales), integrados por contratos.
- Un contexto marca **límites de modelo, no necesariamente servicios ni bases separadas** (en un monolito son módulos). Separarlo físicamente exige además considerar autonomía, operación, volumen, consistencia y ownership.
- **Traducir lo que se recibe** al modelo propio: si Pagos expone un `PaymentStatus` técnico, Órdenes lo mapea a sus estados de negocio (evita que se filtre vocabulario externo).

### Patrones tácticos (no se aplican todos ni se mapean mecánicamente a tablas)
| Patrón | Qué es |
| --- | --- |
| **Entidad** | Identidad que persiste aunque cambien atributos (la Orden sigue siendo la misma al pasar de Pendiente a Confirmada). |
| **Objeto de valor** | Definido por sus valores, sin identidad (`Monto` = cantidad + moneda). Suele ser inmutable. |
| **Agregado** | Límite de consistencia: el estado que protege una regla en una operación. Se modifica **solo vía la raíz**. |
| **Invariante** | Regla que debe cumplirse al aceptar una transición (una orden cancelada no vuelve a confirmada). |
| **Servicio de dominio** | Regla de dominio que no pertenece naturalmente a una entidad u objeto de valor. |
| **Servicio de aplicación** | Coordina el caso de uso (cargar agregado → invocar comportamiento → guardar) **sin esconder reglas de dominio**. |
| **Repositorio** | Recupera/guarda **agregados completos**, uno por raíz (una línea de orden se persiste vía la Orden). No expone SQL ni ORM. |
| **Evento de dominio** | Hecho significativo (`OrdenConfirmada`). Un registro o cambio de campo no es automáticamente un evento de dominio. |

- Agregado ≠ tabla ≠ grafo grande de objetos: acotado para que sus reglas entren en una operación consistente. Entre agregados se coordina **por identificadores y contratos**.
- **Tres límites que se confunden:** agregado = **consistencia transaccional**; contexto delimitado = **lenguaje y modelo**; microservicio = **despliegue y ejecución**. Pueden coincidir, pero ninguno implica a los otros.
- **Regla: un solo agregado por transacción** (consistencia inmediata). Si otro agregado debe reflejar el cambio (ej. ajustar cupo tras confirmar una orden) → **eventos de dominio + consistencia eventual**, no ampliar la transacción.
- Identificar un contexto no obliga a desplegarlo como microservicio.

## Arquitectura Hexagonal (Ports & Adapters)
- Protege el **núcleo** (casos de uso + reglas) de los detalles que entran o salen.
- **Puerto de entrada** (primary/driving): capacidad que ofrece el sistema (confirmar orden); el adaptador **la invoca**. **Puerto de salida** (secondary/driven): necesidad del núcleo (guardar orden, pedir inferencia); el adaptador **la implementa**.
- **Adaptadores:** Handler HTTP (entrada); cliente SQL o SDK de IA (salida). En Go el puerto suele ser una **interfaz**; dónde vive puede variar, lo importante es que **el núcleo no dependa de tipos del adaptador**.
- **Inversión de dependencias:** el núcleo expresa lo que necesita con términos propios; cambiar el proveedor A por B = otro adaptador. La política de negocio decide qué hacer si la recomendación falla. **Un contenedor de DI no garantiza por sí solo el aislamiento.**
- **Una interfaz se justifica** si delimita una variación relevante o permite sustituir una dependencia externa. Interfaz por cada función sin necesidad = indirección y mantenimiento sin beneficio.
- **Pruebas:** casos de uso con adaptadores falsos (sin base ni proveedor); adaptadores con pruebas de traducción, configuración y respuestas inválidas; más **tests de integración** de las piezas reales.
- **Costo:** contratos, conversiones, más piezas. Si la lógica termina repartida entre Handler, adaptador y repositorio, la separación es solo nominal.

### Correspondencia con el proyecto de DSW (Go + Gin + GORM)
| Concepto en DSW | Rol en Hexagonal |
| --- | --- |
| Handler de Gin | Adaptador de entrada |
| Interfaz del caso de uso (ej. `ConfirmarOrden`) | Puerto de entrada |
| Interfaz de acceso a datos (ej. `OrderRepository`) | Puerto de salida |
| Implementación con GORM sobre MySQL | Adaptador de salida |
| Struct de dominio **sin tags** `gorm`/`json` | Entidad o agregado |
| Struct con tags `gorm:"..."` / `json:"..."` | Modelo de la capa externa — **no debería cruzar al núcleo** |

`Handler → Service → Repository` por sí solo **no es Hexagonal**. El cambio no es agregar una carpeta `domain/`: es que el núcleo dependa de contratos útiles para sus casos de uso y que Gin/GORM se conecten a ellos.

## Clean Architecture
- **Anillos** (de adentro hacia afuera): **Entidades y reglas del dominio → Casos de uso → Adaptadores de interfaz → Frameworks y dispositivos**.
- **Regla de dependencias:** el código depende **solo hacia adentro**. Lo interior no importa frameworks, controladores HTTP, tipos de ORM ni SDKs. La ejecución cruza en sentido inverso mediante interfaces, parámetros y objetos definidos adentro.
- **Dos niveles de reglas:** **entidades** = reglas de la empresa (válidas aunque este sistema no existiera; ej. una orden confirmada no se factura por un monto distinto al pagado). **Casos de uso** = reglas de esta aplicación (qué pasos sigue `ConfirmarOrden`, qué valida antes, qué hace con el resultado). Criterio: si cambiaría en cualquier sistema de órdenes → entidad; si depende de cómo esta app resolvió el flujo → caso de uso.
- **Cruzar límites sin acoplar:** el caso de uso define un **output boundary** (interfaz de salida) y un **presenter**/adaptador externo lo implementa (JSON, vista…). El caso de uso nunca conoce el formato HTTP ni las anotaciones del serializador.
- **Vs Hexagonal:** misma intención (aislar el núcleo); Hexagonal enfatiza puertos/adaptadores, Clean enfatiza anillos + regla de dependencia. **No exigen una estructura fija de carpetas.** DDD puede aportar el modelo de los anillos internos, pero son decisiones independientes.

## CQRS (Command Query Responsibility Segregation)
| Operación | Ejemplo | Responsabilidad |
| --- | --- | --- |
| **Comando** | `ConfirmarOrden` | Validar reglas, proteger invariantes, aceptar o rechazar el cambio. |
| **Consulta** | `ConsultarHistorialDeOrden` | Devolver una vista con lo que necesita la pantalla o consumidor, sin modificar estado. |

- **CQS** = separación a nivel función/objeto; **CQRS** la lleva a modelos y responsabilidades. Tener `CommandHandler`/`QueryHandler` **no implica dos bases de datos**.
- **Adopción por etapas:** (1) separar código de comandos y de consultas con modelo y base compartidos → (2) modelos distintos (escritura = invariantes y transiciones; lectura = proyecciones por vista) → (3) **almacenes separados solo si** escalado, tecnología, seguridad o volumen lo justifican.
- Un modelo de escritura puede alimentar **varios modelos de lectura** (historial del cliente ≠ panel interno).
- **Proyección:** transforma cambios de escritura en una vista para consultas. Si se actualiza asincrónicamente → **consistencia eventual** (el comando tuvo éxito pero la lectura todavía muestra lo anterior). El contrato/UI debe contemplarlo: devolver el nuevo estado en la respuesta del comando o dar una garantía explícita; mostrar una vista atrasada como actual puede llevar a repetir operaciones.
- **No exige mensajería.** Si se usan eventos/colas para otro almacén, aparecen: fallos de publicación, duplicados, reintentos, orden, idempotencia, observación de proyecciones.
- Lecturas >> escrituras: separar permite optimizar; el escalado independiente requiere además separar almacenes o despliegues. Lectura puede usar caché o réplicas de solo lectura; **los comandos vuelven a validar invariantes en el modelo de escritura**.
- **Conviene** cuando lectura y escritura difieren en forma, complejidad, rendimiento, permisos o volumen. Si un CRUD alcanza, CQRS duplica modelos, conversiones y diagnóstico.

## Event Sourcing
- **CQRS ≠ Event Sourcing:** se adoptan por separado o combinados. Combinados: el **event store es la fuente de verdad** del lado de escritura y sus eventos alimentan proyecciones (sin exigir almacenes separados).
- CRUD guarda el estado actual (`Orden.estado = "Confirmada"`) y pierde cómo se llegó. ES guarda una **secuencia durable de eventos** del agregado; el estado se **reconstruye aplicando los eventos en orden (replay)**.
- **Evento** = algo que ocurrió con significado (`OrdenCreada`, `PagoConfirmado`, `OrdenCancelada`), no `EstadoActualizado`. **Comando** = intención; puede rechazarse y entonces no produce evento.
- Los eventos persistidos **no son automáticamente mensajes públicos**: para integrar contextos se publican contratos externos distintos, con su propia evolución.
- **Flujo de escritura:** (1) cargar stream (o snapshot + eventos posteriores) → (2) reconstruir estado → (3) comando al agregado, verificar invariantes → (4) generar eventos → (5) **anexar con la versión esperada**.
- **Versión esperada = control optimista de concurrencia:** de dos escrituras sobre la misma versión, se acepta una; la otra falla por conflicto → **recargar y volver a evaluar, no repetir a ciegas** (la regla puede ya no permitirlo). ES no elimina la contención.
- **Snapshot:** estado en una versión para no reproducir todo; **no reemplaza al stream**. Consultas sobre muchos agregados → proyecciones / vistas materializadas (reconstruibles desde eventos, con costo; si son async, eventualmente consistentes).
- **Eventos = contratos duraderos:** el código nuevo debe leer eventos viejos (versiones o transformaciones al leer). No se reescriben; un error se corrige con un **hecho compensatorio**. Guardar hechos con significado, no volcar todo como auditoría técnica.

### Event store ≠ message broker
| | Event store | Message broker |
| --- | --- | --- |
| Rol | Fuente de verdad del agregado | Distribuye mensajes entre servicios |
| Lectura | Por stream de un agregado, secuencia ordenada | Por tópico, partición y posición del consumidor; el orden depende del broker y el particionado |
| Concurrencia | Puede verificar la versión esperada al anexar | No ofrece control de versión esperada; se resuelve en la app u otra garantía |
| Retención | Historia necesaria para reconstruir, con políticas explícitas | Configurable (retener/compactar), pero eso no lo convierte en fuente de verdad |

Kafka puede distribuir eventos a proyecciones, pero una cola o log de mensajes no sustituye las garantías de un event store.

- **Conviene:** consultar cómo evolucionó una entidad, auditar decisiones, reconstruir estados, varias vistas del mismo historial (si el valor supera costos de versionado, conflictos, proyecciones, operación).
- **No conviene:** CRUD simple sin necesidad de historial (persistencia más difícil de cambiar); datos de referencia casi inmutables. Si se exige lectura inmediata y las proyecciones son async → aceptar la demora o actualizar sincrónicamente (ES no impone lecturas async).
- **Se puede aplicar a una parte** (ciclo de vida de la orden) y dejar el resto con estado actual (perfiles, preferencias).

## Arquitectura reactiva y programación reactiva
- **Reactive System** (Reactive Manifesto): responde oportunamente, se adapta a la carga, contiene fallos, se comunica por **paso asíncrono de mensajes**. **Programación reactiva:** modelo para componer resultados asíncronos y flujos (procesar cada elemento a medida que llega). Uno no implica el otro.
- **I/O no bloqueante:** el hilo atiende otro trabajo mientras espera; muchas esperas concurrentes con menos hilos, **si todo el stack lo soporta**.
- **Streams reactivos:** Publisher (productor) / Subscriber (consumidor); 0, 1 o N resultados en el tiempo; operadores para transformar, componer, propagar errores y cancelaciones.
- **Backpressure:** coordinación de demanda consumidor↔productor (regular, rechazar, almacenar con límite u otra política explícita). Sin ella, el trabajo se acumula en memoria hasta saturar.
- Más I/O en paralelo **no** = más CPU simultánea ni dependencia lenta más rápida. Se pierde la ventaja si hay operaciones bloqueantes en el hilo de I/O, la base no tiene cliente no bloqueante o el CPU monopoliza los ejecutores.
- **Trade-offs:** útil con mucho I/O concurrente, flujos continuos y ajuste de demanda. Cuesta en composición, fallos, cancelación, propagación de contexto, stack traces y pruebas. Con poca concurrencia, carga de CPU o flujo simple → **imperativo es más claro y suficiente**.
- **No reemplaza resiliencia:** un stream no elimina timeouts, circuit breakers ni límites de concurrencia; reactivo dentro de un proceso ≠ cola durable ni define la comunicación entre servicios.
- **Go:** el runtime usa un **netpoller**: una goroutine que espera I/O de red se suspende y el hilo del SO ejecuta otra. Un Handler de Gin conserva código imperativo sin retener necesariamente un hilo, aunque goroutine, request y conexiones siguen ocupando recursos y no toda llamada/driver usa el netpoller.
- JVM: **Reactor** (composición y backpressure); **virtual threads** de Java = I/O masivo con código imperativo, pero sin composición de flujos ni control de demanda de Reactive Streams.

## Frameworks y bibliotecas (referencias)
Reducen implementación, **no sustituyen el razonamiento** sobre reglas, límites y costos.

| Herramienta | Qué facilita | Qué sigue siendo decisión del equipo |
| --- | --- | --- |
| Gin (Go) | Recibir requests HTTP y generar respuestas | Mantener el Handler como adaptador, definir casos de uso y dependencias internas |
| Watermill (Go) | Abstracciones de mensajería y building blocks de CQRS sobre Pub/Sub y routers | Contratos del dominio y separar modelos; no provee event store ni decide si hace falta CQRS |
| Fx (Go) | Inyección de dependencias y ciclo de vida; ensambla componentes al iniciar | Qué depende de qué; ensambla el grafo, no decide la dirección de dependencias |
| Wire (Go, archivado) | Referencia histórica: DI por generación de código en compilación | **Archivado en 2025**; no presentarlo como opción activa |
| Axon Framework (JVM) | Building blocks para DDD, CQRS y ES (comandos, eventos, consultas, event stores) | Límites, modelo, eventos relevantes y si la complejidad justifica ES |
| Spring Modulith (JVM) | Organizar y verificar módulos de una app Spring Boot orientada a dominio | Definir módulos y relaciones; un modulith no separa despliegues |
| jMolecules (Java) | Expresar conceptos de DDD y estilos como Hexagonal en código, con reglas de verificación | Crear límites válidos y corregir dependencias que rompen reglas |
| Spring WebFlux / Reactor (JVM) | Stack web no bloqueante, Reactive Streams con `Mono`/`Flux` | Evitar I/O bloqueante; política de concurrencia, errores y demanda |
| Quarkus / Mutiny (JVM) | APIs reactivas; combinar código reactivo e imperativo | Aplicarlo donde se justifica y sostenerlo en todas las dependencias |

La arquitectura no exige ninguna de estas herramientas: lo que facilitan puede resolverse a mano en cualquier plataforma.

## Casos reales
- **Netflix Studio Workflows + Hexagonal:** app interna con datos en múltiples sistemas/protocolos, fuentes que iban a migrar del monolito. Aislaron reglas de negocio de las fuentes; cambiar una lectura de API JSON a GraphQL = **otro adaptador para el contrato existente, en dos horas**. Resultado situado en un cambio concreto.
- **Uber DOMA** (Domain-Oriented Microservice Architecture): agrupar servicios relacionados en **dominios con gateways e interfaces definidos** (inspirado en DDD, Clean, SOA, diseño por interfaces). A escala no basta con servicios chicos: dependencias y ownership deben formar una estructura comprensible. **No implica que una plataforma estudiantil lo necesite.**

## Diseño de referencia: el interior de Órdenes
Necesidades: confirmar órdenes, consultar historial de un cliente, incorporar recomendaciones de un proveedor de IA. Se comparan alternativas sin fijar una combinación de antemano.
1. **Modelar:** Órdenes controla su ciclo de vida (no cancelar después de despachar); Pagos, autorización/devolución; Recomendaciones, sugerencias (juntos o separados según despliegue). `Orden` = agregado; `Monto` y `DirecciónDeEntrega` = objetos de valor. La raíz valida cada cambio; **la base no debe permitir transiciones inválidas solo porque la columna acepta cualquier string**.
2. **Puertos y adaptadores:** `ConfirmarOrden` por puerto de entrada; guardado por puerto de salida de repositorio; Handler HTTP, repositorio SQL y cliente de IA = adaptadores. Operación propia ("recomendar productos a partir de ids") separada del contrato de cada proveedor; las reglas de Órdenes deciden qué pasa si la recomendación falta o es inválida. Ni Hexagonal ni Clean exigen un servicio por módulo ni abstracciones que no protegen cambios reales.
3. **¿CQRS?** Si escritura e historial difieren en forma, reglas o carga → modelos separados (al principio misma base y transacción; separar almacenes agrega sincronización y consistencia eventual). Si la consulta devuelve lo mismo que el modelo de escritura → **repositorio CRUD compartido es más simple**.
4. **¿Event Sourcing?** Si hay que reconstruir las decisiones del ciclo de vida → persistir `OrdenCreada`, `PagoConfirmado`, `OrdenCancelada`; event store como fuente de verdad; historial proyectado. Cada append verifica versión esperada (conflicto → recargar y revalidar). Si solo hace falta el estado actual → **persistirlo directamente puede bastar**.
5. **¿Reactivo?** Si Recomendaciones recibe muchas solicitudes simultáneas e I/O-bound → cliente no bloqueante, con demanda limitada. No acelera una inferencia lenta ni debe bloquear recursos centrales de Órdenes. Con volumen moderado → **integración imperativa con timeout**. Se decide con medidas reales, **no por el uso de IA**.

## Trade-offs y criterios de decisión
| Pregunta | Alternativa que puede ayudar | Costos a justificar |
| --- | --- | --- |
| ¿Reglas ambiguas o que cambian junto con un lenguaje del dominio? | DDD (términos, invariantes, contextos) | Colaboración con expertos, aprendizaje del modelo, mantener límites explícitos |
| ¿El núcleo depende de una base, framework o proveedor que cambia? | Hexagonal o Clean (dependencias hacia las reglas) | Contratos, mapeos, estructura extra; interfaces sin variación = sobreingeniería |
| ¿Lecturas y escrituras con formas, reglas o cargas distintas? | CQRS (responsabilidades y, si hace falta, modelos) | Duplicación, proyecciones, consistencia eventual si se separan almacenes |
| ¿El historial es valioso o hay que reconstruir estado? | Event Sourcing | Evolución de eventos, conflictos concurrentes, proyecciones, operación del event store |
| ¿Mucho I/O concurrente y stack no bloqueante? | Programación reactiva | Complejidad de ejecución, depuración, backpressure, propagación de errores/contexto |

- Se pueden complementar, pero **el beneficio de cada una debe quedar visible**.
- **Explicitar qué no se optimiza:** sin necesidad de cambiar proveedores, el puerto genérico no aporta; si lectura y escritura comparten requisitos, CQRS suma pasos; si el historial no es central, el log append-only es carga; sin presión de I/O, reactivo complica más de lo que ayuda.

## Puntos clave para defender
- El estilo arquitectónico responde una pregunta distinta del modelo de dominio, la persistencia, el framework o el despliegue.
- DDD no obliga a usar microservicios ni es requisito de Hexagonal/Clean.
- **Agregado** protege invariantes en su límite de consistencia; no es una tabla ni un microservicio.
- Hexagonal y Clean mantienen las reglas independientes de transportes, bases y frameworks (**dependencias hacia adentro**).
- CQRS puede empezar con modelos lógicos distintos sobre **un mismo almacén**.
- Event Sourcing ≠ CQRS; requiere versiones de stream, concurrencia, evolución de eventos y proyecciones; sus eventos **no son mensajes de broker**.
- Reactivo: composición no bloqueante y backpressure si toda la cadena coopera; **no elimina latencia ni reemplaza resiliencia**.
- **MVC y CRUD siguen siendo válidos** cuando expresan el problema con menos complejidad.

## Relevancia para el TP final
La consigna pide **al menos dos servicios con patrones de arquitectura diferentes** (2.1), aplicados "de manera reconocible" en la Presentación Grupal, y el **ADR D2** (estilos elegidos, dependencias internas, adecuación al problema de cada servicio). Este apunte es la base para justificarlo: elegir el estilo según la pregunta que responde (no por moda) y explicitar qué costo se acepta.

## Referencias
Evans, *Domain-Driven Design* (2003) · Microsoft Learn (Domain Analysis, Tactical DDD, CQRS Pattern, Event Sourcing Pattern) · Cockburn, *Hexagonal Architecture* · Robert C. Martin, *The Clean Architecture* · Netflix Tech Blog, *Ready for changes with Hexagonal Architecture* · Uber Engineering, *Introducing DOMA* · Axon, Spring Modulith, jMolecules · Watermill, Fx, Wire · Go `internal/poll`; JEP 444 Virtual Threads · Apache Kafka · Spring WebFlux, Reactor · Quarkus, Mutiny · The Reactive Manifesto.
