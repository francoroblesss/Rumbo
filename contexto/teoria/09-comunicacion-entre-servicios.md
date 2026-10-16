# Teórico 9 — Comunicación entre servicios

Fuente (PDF de la cátedra, ya no se guarda local): `comunicacion-entre-servicios.pdf` (Clase 09, 14/10/2026)

## Contexto
- Entre servicios hay una red: la respuesta puede **tardar, perderse o no llegar aunque el servidor haya completado la operación**. Los participantes se despliegan y evolucionan por separado. Una llamada remota **no es una llamada local con otra dirección**: suma contrato, protocolo y costo operativo.
- Tres familias, **no son alternativas del mismo nivel** y se combinan:
  - **petición/respuesta** (REST, gRPC, GraphQL);
  - **canal persistente** de actualización (SSE del servidor al cliente, WebSockets en ambos sentidos);
  - **notificación HTTP** (webhook), que la inicia quien detecta el evento.
- Preguntas para elegir: quién consume y quién inicia, si hace falta respuesta inmediata, dirección y volumen de los datos, cómo evoluciona el contrato, interoperabilidad y experiencia de desarrollo, y costo operativo.

## Contratos y evolución compatible
- Un contrato define operaciones o eventos, campos, formatos, **errores**, autenticación y **semántica** ("qué significa pago aprobado"). No es solo un JSON de ejemplo.
- **Compatible hacia atrás:** productor nuevo con consumidor viejo (el sentido habitual en APIs). **Compatible hacia adelante:** consumidor nuevo con productor viejo. En la bibliografía de esquemas (Avro, Protobuf) las etiquetas se usan al revés: importa **nombrar la combinación de versiones**, no la etiqueta.
- **Agregar un campo opcional** es compatible solo si los consumidores toleran campos desconocidos (un parser estricto lo rechaza).
- **Agregar un valor a una enumeración puede romper** al consumidor que asume que conoce todos los valores.
- Cambio aditivo prudente:
  1. los consumidores toleran la forma nueva;
  2. se publica el cambio sin retirar lo anterior;
  3. se observa el uso y se migra;
  4. se retira lo viejo cuando no quedan dependencias.
- Cambio incompatible: migración coordinada o **versiones que conviven**. El número de versión identifica el contrato, **no vuelve compatible un cambio**.
- **Dónde va la versión:**
  - en la **ruta** (`/v2/orders`): visible y fácil de enrutar, pero duplica recursos y lógica;
  - en un **header o media type**: no cambia la URI, pero se ve menos en herramientas, logs y cachés;
  - por **fecha** (Stripe): fija el contrato, pero hay que mantener varios comportamientos.

  Siempre hay que definir la migración, la convivencia, cómo detectar consumidores pendientes y cuándo retirar.
- **OpenAPI** (HTTP), **Protocol Buffers** (gRPC) y **esquema GraphQL** hacen explícito el contrato. Documentación, código generado y linters **no reemplazan una política de evolución** ni verifican la semántica.
- **Operación:** métricas de volumen, latencia y errores para distinguir lenta / rechaza / cuota; **correlation id** entre servicios; los logs **no copian credenciales ni payloads sensibles**.

## REST sobre HTTP
- Es un estilo, no "JSON por HTTP": las URI identifican **recursos de negocio**, los métodos expresan operaciones, y los códigos de estado y headers son parte del contrato. No exponer las tablas internas.

| Método | Semántica | Propiedad |
| --- | --- | --- |
| GET | Leer | Seguro e idempotente |
| PUT | Crear o reemplazar un recurso conocido | Idempotente según el contrato |
| DELETE | Eliminar | Idempotente respecto del efecto (la respuesta puede variar) |
| POST | Crear o pedir una operación | **No idempotente por defecto** |

- **Idempotencia = efecto de repetir**, no respuestas idénticas (RFC 9110 §9.2.2). Si un POST puede duplicar un efecto, el contrato ofrece una **clave de idempotencia** para distinguir un retry de una operación nueva.
- **Paginación y filtros son parte del contrato**: una colección nunca devuelve resultados ilimitados.
- Encaja con clientes heterogéneos y operaciones que se entienden como recursos. Costo: mantener convenciones, evolucionar recursos publicados, varias llamadas o campos de más en algunas pantallas.

## gRPC (RPC)
- La operación remota se presenta como un método: el **stub generado** serializa, envía y convierte la respuesta. Parece local, pero **conserva latencia y fallas de red**.
- `.proto` (Protocol Buffers) para describir y serializar, sobre HTTP/2.
- Cuatro formas: **unary**, **server streaming**, **client streaming** y **bidireccional**.
- Cambios con reglas propias: **reservar número y nombre** de un campo eliminado. Un contrato que compila no garantiza que dos versiones desplegadas se entiendan.
- Bueno entre servicios con toolchain compartido. Para navegadores hace falta gRPC-Web u otra frontera. El formato binario **no prueba por sí solo** que la aplicación sea más rápida.

## GraphQL
- Lenguaje de consulta y runtime sobre un **esquema tipado** (no es transporte ni base de datos). Los resolvers buscan los datos. `query` lee y `mutation` cambia.
- El consumidor elige los campos. Una sola interacción visible **no es una sola lectura** en el servidor: hay riesgo de **N+1**.
- Lo que el servidor tiene que hacer:
  - limitar profundidad, cantidad y costo de las consultas;
  - aplicar **rate limit por consumidor**;
  - aplicar **autorización por objeto y campo**;
  - agrupar cargas;
  - tener una caché pensada para consultas, variables y permisos (**no alcanza con cachear una URL**).
- Útil si varias experiencias necesitan proyecciones distintas y el equipo puede gobernar esquema y límites. Si no, **REST simple es más predecible**.

## Server-Sent Events (SSE)
- Flujo **servidor → cliente** sobre HTTP (`text/event-stream`, `EventSource` en el navegador). Para enviar comandos, el cliente usa **otra request HTTP**.
- Evento: `event`, `id`, `data` (y `retry`), cerrado por una línea vacía. Al reconectar, el navegador manda **`Last-Event-ID`** y el servidor puede reanudar si guardó el historial.
- **No es una cola durable**: el estado de negocio se persiste aparte y el cliente **consulta para reconciliarse** después de una desconexión.
- Usos: estado de una compra, progreso de un trabajo, fragmentos de una respuesta de IA. Evita el polling, pero mantiene conexiones abiertas que proxies y balanceadores tienen que soportar.
- **Seguridad:** sigue haciendo falta autenticación y autorización por recurso.
  - `EventSource` nativo **no permite el header `Authorization`**: usar sesión o cookie, o consumir el flujo con `fetch` (y reconectar a mano).
  - **No poner tokens de larga duración en la URL** (quedan en los logs).

| | Dirección | Conexión | Cuándo | Límite |
| --- | --- | --- | --- | --- |
| **Polling** | Cliente pregunta | Requests separadas | Cambios poco frecuentes, simplicidad | Consultas sin novedades |
| **SSE** | Servidor → cliente | Flujo HTTP abierto | Actualizaciones unidireccionales frecuentes | Sin canal inverso ni historial garantizado |
| **WebSockets** | Bidireccional | Canal persistente | Interacción en vivo en ambos sentidos | Conexión, autenticación y reconexión más complejas |

**No usar WebSockets para actualizar una pantalla si el cliente no responde por ese canal.**

## WebSockets
- Canal persistente bidireccional (RFC 6455): un handshake HTTP que se actualiza a WebSocket. Sirve para chat, colaboración e interacción en vivo.
- La aplicación resuelve apertura, cierre, autenticación, suscripciones, reconexión y **recuperación del estado perdido**. No hay historial durable.
- El constructor nativo no acepta headers: autenticar con sesión/cookie o con un **ticket de corta duración**. **Validar `Origin`** complementa la autenticación, no la reemplaza.
- Ejemplo: **Slack Socket Mode**, un WebSocket entre servidores en lugar de exponer una URL pública.

## Webhooks
- El **proveedor** que detecta el cambio hace un POST a un endpoint registrado. No hay canal abierto.
- El endpoint es una **frontera pública**:
  - **validar la firma y la frescura** antes de confiar en el payload (ej. `x-signature` de Mercado Pago);
  - **registrar de forma durable y responder rápido**, y procesar después.
- Puede llegar **duplicado o fuera de orden**: deduplicar por la identidad del evento y aplicar el cambio de forma idempotente.
- **No reemplaza la fuente de verdad:** la decisión sale del **estado vigente del recurso**. Ante dudas, consultar al proveedor y validar la transición.

## IA como servicio externo
- **Síncrono** cuando el resultado hace falta ya, con presupuesto de latencia (red + espera + generación). **Diferido** cuando se puede mostrar "pendiente". **Streaming** (OpenAI Responses API usa SSE) baja el tiempo hasta el primer fragmento, no el tiempo total, y no es procesamiento diferido.
- **Costo variable:** solicitudes, tamaño de entrada y salida, modelo y retries. Usar precios vigentes y el patrón real de consumo.
- **Cuotas:** no asumir capacidad ilimitada. `429` + **respetar `Retry-After`** cuando el proveedor lo documenta (Mercado Pago), y decidir qué ve el usuario sin capacidad.
- **Puerto y adaptador:** el dominio no importa el SDK ni sus errores; el adaptador traduce. Pero **no promete sustitución transparente**: los proveedores difieren en capacidades, calidad, límites y costos. Abstraer lo que necesita el caso de uso y documentar lo que varía.
- **Minimización de datos:** mandar solo lo necesario. Revisar retención, registros y acceso a las credenciales (no entrenar ≠ no retener).

## Casos reales
- **GitHub GraphQL:** la API REST pedía varias llamadas o devolvía campos de más; GraphQL deja elegir campos a cambio de gobernar el costo de las consultas.
- **Google / gRPC:** sale de Stubby, la infraestructura interna de RPC de Google. No prueba que RPC sea siempre mejor.
- **Stripe:** releases mensuales compatibles y releases mayores incompatibles; política de versionado previsible.
- **Slack Socket Mode:** WebSocket en lugar de una URL pública; la URL de conexión se renueva y la app confirma la recepción.
- **Mercado Pago Orders:** webhook `order.processed` con `x-signature`; validar e idempotencia.
- **OpenAI Responses API:** streaming por SSE.
- **Wayfair:** IA en flujos existentes con validación gradual y **confirmación humana** para cambios de baja confianza o alto riesgo.

## Caso aplicado: compra con pago, webhook, SSE e IA
1. **Vista de la orden:** REST (o GraphQL si hay varias proyecciones y se puede gobernar). No hace falta usar los dos.
2. **Crear el pago:** `POST` con **`X-Idempotency-Key`**.
   - Primero se persiste la asociación orden–clave con el estado **pendiente**.
   - Ante un **timeout, el resultado es incierto**: se hace un retry con la **misma clave y el mismo payload**, o se **reconcilia consultando** el estado de la order.
   - No suponer que la repetición devuelve la respuesta original (puede dar `409`).
   - Una operación nueva lleva una clave nueva.
3. **Notificación:** el webhook se valida (firma, frescura, identidad), se deduplica y se decide con el estado vigente.
4. **Pantalla:** se **persiste primero** y después se publica por SSE. Si se corta, el cliente consulta la fuente persistida.
5. **Llamadas internas:** REST o gRPC según el contrato, la interoperabilidad y las herramientas. "Ser microservicios" no decide nada.
6. **Recomendaciones con IA** detrás de un adaptador, con presupuesto de latencia y costo, y solo el contexto necesario.

**Cada tecnología tiene que responder a una necesidad que justifique su costo; no hace falta que coexistan todas.**

## Trade-offs
| Opción | Elegir cuando | Ventaja | Costo / límite |
| --- | --- | --- | --- |
| REST/HTTP | API de recursos interoperable para clientes variados | Semántica HTTP conocida, herramientas, inspección | Convenciones y evolución; varias llamadas en algunas vistas |
| gRPC | Servicios con toolchain compatible, operaciones tipadas | Contrato explícito, streaming | Generación de código, coordinación de esquemas, adaptación web |
| GraphQL | Varias proyecciones de datos relacionados | El cliente declara los campos | Gobierno del esquema, autorización por campo, consultas costosas |
| SSE | El cliente solo recibe actualizaciones | Unidireccional, reconexión en `EventSource` | Conexión abierta, sin canal inverso ni historial |
| WebSockets | Mensajes bidireccionales en tiempo real | Canal interactivo | Ciclo de vida, reconexión, seguridad y estado a cargo de la app |
| Webhooks | El proveedor conoce el cambio antes | Sin polling | Endpoint alcanzable, autenticación, retries, duplicados, orden |
| IA externa | La capacidad justifica usarla | Inferencia sin operar el modelo | Latencia, costo variable, cuotas, datos compartidos |

## Puntos clave
- La red agrega latencia, fallas, serialización y evolución independiente: toda operación remota necesita un **contrato observable**.
- La compatibilidad depende de **qué versiones se comparan**. Versionar ayuda a coordinar la migración, la convivencia y el retiro, pero no hace compatible un cambio.
- **Un POST no es idempotente por defecto.** La clave de idempotencia sigue el contrato y acompaña la **misma operación lógica**.
- gRPC no es universalmente más rápido. GraphQL obliga a controlar autorización y costo. SSE es unidireccional y **sin header `Authorization` nativo**. WebSockets necesita validar `Origin` además de autenticar.
- Un webhook se valida, se deduplica y se decide con el estado vigente.
- La IA externa suma presupuestos de latencia y costo, cuotas y una frontera de datos. Un adaptador **no promete sustitución transparente**.

## Aplicación a Rumbo
- **Capacidad publicada (ADR-002/006):**
  - REST con OpenAPI 3.1. Primero la versión iba en la **ruta** (`/v1`, `/v2`); desde ADR-019 las rutas **no llevan versión** (`/traslados`), por indicación recibida. El contrato se versiona en su documento (`info.version`) y los cambios incompatibles se hacen con **migración coordinada**, que es la otra salida que da el teórico;
  - `Idempotency-Key` en los POST;
  - paginación y filtros en el listado;
  - seguimiento por **polling** (cambios poco frecuentes, consumidor de otro grupo).
- **Agregar valores a enums:** según este teórico puede romper consumidores. El contrato ya trata como incompatible **agregar estados** (§ Compatibilidad). Los `codigo` de error nuevos se aceptan como compatibles solo porque el consumidor decide por el **HTTP status** cuando no conoce el `codigo`; eso tiene que quedar dicho en el contrato.
- **Clima (ADR-012):** puerto `ConsultaClima` y adaptador que traduce el contrato del proveedor, como en §9.2.
- **Bonus de tiempo real (SPEC §7):** el cliente solo **recibe** cambios de estado → **SSE**, no WebSockets.
  - Se persiste primero (Operaciones) y después se publica.
  - Al reconectar, la web vuelve a consultar el estado.
  - `EventSource` no manda `Authorization`: hay que resolver la autenticación (cookie o `fetch` con streaming) y evitar el token en la URL.
  - Los proxies y NGINX tienen que dejar pasar el flujo (sin buffering y con un timeout de lectura largo).
