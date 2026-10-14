# Rumbo — Arquitectura

> Explica la arquitectura general mediante texto y diagramas. Debe permitir reconocer los servicios, sus responsabilidades, los datos que administra cada uno, sus comunicaciones y la distribución de los componentes (consigna §3).
> Las decisiones y sus justificaciones están en [`adr/`](adr/README.md); acá se describe el resultado.

## 1. Diagrama de contexto
[Rumbo — C4 Nivel 1: Contexto (PDF)](<diagramas/Rumbo - C4 Nivel 1 Contexto.pdf>)

- Rumbo tiene cuatro tipos de usuario, que lo usan a través de la web: **cliente** (de una organización), **operador**, **chofer** y **administrador**.
- Se integra con dos sistemas de otros grupos:
  - el **sistema de gestión de almacenamiento** (otro grupo) consume la capacidad publicada **"Solicitar traslado"** (API REST `/v2`): trasladar una cosa desde un origen hasta un destino;
  - Rumbo consume las **condiciones climáticas** del **sistema de aeropuerto**.

## 2. Diagrama de contenedores
[Rumbo — C4 Nivel 2: Contenedores (PDF)](<diagramas/Rumbo - C4 Nivel 2 Contenedores.pdf>)

- Una **aplicación web** y un **API Gateway** como punto de entrada único (token firmado, ruteo, límite de pedidos, correlation id).
- **Cinco microservicios en Go:** Identidad y clientes, Operaciones (núcleo), Recursos, Notificaciones e Integración de carga.
- **Almacenamiento:** una base de datos por microservicio: dos en **MySQL** y tres en **MongoDB**. Cada servicio accede solo a la suya.
- **RabbitMQ** para la mensajería asíncrona.
- El **motor de búsqueda** figura como pendiente (D6). La caché (D7) y el balanceo de carga (D12) se agregan cuando se decidan.

## 3. Servicios
Cinco microservicios detrás de un API Gateway. Justificación en [ADR-001](adr/ADR-001-limites-de-los-servicios.md) (D1).

| Servicio | Responsabilidad | Datos de los que es dueño |
| --- | --- | --- |
| **Identidad y clientes** | Quién es cada usuario, qué puede hacer y a qué organización y padrón pertenece | Usuarios, roles y credenciales; organizaciones cliente; padrón de personas transportadas |
| **Operaciones** | Recibir lo que hay que trasladar, planificarlo, asignarlo y seguir su ejecución | Solicitudes, planes, traslados y tramos, recorridos y paradas, asignaciones, incidentes |
| **Recursos** | Con qué y con quién se traslada, y cuánto le corresponde a cada chofer | Choferes (jornada, ausencias), vehículos (capacidad, accesibilidad, propietario), actividad, tarifas, preliquidaciones |
| **Notificaciones** | Hacer llegar a cada usuario sus avisos | Bandeja de avisos por usuario |
| **Integración de carga** | Atender el contrato público "Solicitar traslado" (v2) y traducirlo al modelo interno | Solicitudes externas (id externo ↔ interno, idempotencia, último estado conocido) |

- La operación crítica (**asignación**) vive entera en Operaciones.
- Operaciones consulta a Recursos (disponibilidad y vehículo) y a Identidad y clientes (padrón, del que guarda una copia en el traslado).
- Recursos y Notificaciones reciben eventos de Operaciones (recorrido finalizado, avisos).
- La capacidad publicada entra por el Gateway a **Integración de carga**, que entrega las solicitudes a Operaciones y recibe los cambios de estado por eventos. Las decisiones de negocio las toma Operaciones.
- La capacidad consumida (clima) sale desde Operaciones.
### 3.1 Organización interna de cada servicio (bajo nivel)
La consigna pide al menos dos estilos de arquitectura distintos. Justificación en [ADR-010](adr/ADR-010-estilos-internos.md) (D2); esta es la estructura con la que se empezó:

| Servicio | Estilo | Estructura |
| --- | --- | --- |
| **Operaciones** | **Hexagonal** (teórico 8): es el núcleo, tiene las reglas e invariantes más complejas y la mayor cantidad de dependencias externas (MySQL, Recursos, Identidad, clima, broker) | `dominio/` (entidades y reglas, sin tags json/db) · `aplicacion/` (casos de uso) · `puertos/` (contratos de entrada y salida) · `adaptadores/` (`api` HTTP, `mysql`, `mensajeria`, `recursos`, `identidad`, `clima`) |
| **Identidad y clientes**, **Recursos**, **Notificaciones**, **Integración de carga** | **Capas** (Handler → Service → Repository, estilo del práctico): lógica más simple y una sola base cada uno | `controllers/` · `services/` · `repositories/` · `models/` · `database/` · `messaging/` (si usa el broker) · `clients/` (Integración → Operaciones) |
| **API Gateway** | Reverse proxy con middlewares | `proxy/` (ruteo, 502/504, deadline de 5 s) · `correlacion/` · `autenticacion/` y `limite/` (pendientes, con librerías) |

Cada servicio es un módulo Go independiente (`servicios/<servicio>/`) con su `Dockerfile`, su puerto y su configuración por variables de entorno. Lo que no es negocio y comparten todos (observabilidad, mensajería con reintentos y DLQ, identidad propagada por el gateway y errores Problem Details) está en el módulo **`servicios/comun`**, y el contrato interno entre servicios en [`interfaces-internas.md`](interfaces-internas.md). `main.go` arma las dependencias a mano (infraestructura → repositorios o adaptadores → servicios o casos de uso → handlers).
<!-- Pendiente por servicio: API expuesta y eventos publicados/consumidos en detalle (D5, Entrega 2). -->

## 4. Datos y persistencia
Detalle en [ADR-003](adr/ADR-003-persistencia.md) (D3); el motor relacional es MySQL desde [ADR-005](adr/ADR-005-mysql-como-motor-relacional.md).

| Servicio | Almacenamiento | Motivo principal |
| --- | --- | --- |
| Operaciones | MySQL, base `operaciones` | Reserva de recursos con ACID; muchas relaciones; transacción con bloqueo por recurso (`SELECT … FOR UPDATE`) contra asignaciones superpuestas; outbox e idempotencia en la misma transacción |
| Recursos | MySQL, base `recursos` | Preliquidación (montos, correcciones, cierres): finanzas y auditoría |
| Identidad y clientes | MongoDB, base `identidad` | Padrón con esquema variable por tipo de servicio (discriminador + JSON Schema) |
| Notificaciones | MongoDB, base `notificaciones` | Avisos independientes, leídos por usuario y fecha |
| Integración de carga | MongoDB, base `integracion` | Solicitud externa leída como unidad; solicitud, idempotencia y "pendiente de envío" en una escritura atómica por documento |

- **Una base de datos por microservicio**, cada una con su propio usuario. Las bases viven en una instancia por motor.
- El índice de búsqueda, la caché y el broker son derivados, no fuente de verdad.
<!-- Operación con garantías de consistencia: ver D4. -->

## 5. Comunicación entre servicios
Detalle en [ADR-004](adr/ADR-004-comunicacion-entre-servicios.md) (D5).

**Síncrona (HTTP/JSON, deadline propagado con `context`):**
| Llamada | Timeout | Si falla |
| --- | --- | --- |
| Cliente → Gateway → servicio | 5 s total; el gateway no reintenta | 502 / 504; reintenta el cliente con `Idempotency-Key` |
| Operaciones → Recursos (disponibilidad, vehículo) | 800 ms, 1 reintento con jitter | Se rechaza la asignación (fail-closed) |
| Operaciones → Identidad (datos de personas) | 800 ms, 1 reintento con jitter | La solicitud no se acepta; sigue `RECIBIDA` |
| Integración de carga → Operaciones (contrapropuesta, cancelación) | 2 s, sin reintento | 503 al consumidor, que reintenta con la misma clave |

**Asíncrona (RabbitMQ):**
- Eventos en exchanges `topic` por productor (`rumbo.operaciones`, `rumbo.recursos`), con una cola por servicio consumidor.
- Un comando en cola de trabajo: `operaciones.solicitudes-externas`.
- **Evento de dominio principal:** `operaciones.recorrido.finalizado` → Recursos registra la actividad.
- Garantías: outbox, entrega at-least-once, ACK después del efecto, consumidores idempotentes, 3 reintentos con backoff y jitter, DLQ por cola con dueño y alerta.

**Correlación:** el `X-Correlation-ID` se asigna en el gateway y se propaga en HTTP y en los mensajes.

## 6. Integración con otros grupos
- **Capacidad publicada — Solicitar traslado** ([ADR-002](adr/ADR-002-contrato-propio.md), versión vigente en [ADR-006](adr/ADR-006-contrato-generico-v2.md)): API REST **`/v2`** descrita en OpenAPI 3.1 ([`contracts/`](contracts/README.md)). Es genérica: tres datos obligatorios (`cosa`, `origen`, `destino`). Entra por el API Gateway (token firmado) y la atiende el servicio **Integración de carga**, que la traduce al modelo interno y la entrega a Operaciones. El procesamiento es asíncrono (`202` + estado `RECIBIDA`); sin hora pedida, Rumbo propone una (`CONTRAPROPUESTA`). El seguimiento es por consulta, los `POST` son idempotentes y los errores usan Problem Details. Hay un mock generado con Prism. La versión 1.0.0 (`/v1`, específica de carga) quedó reemplazada sin haber tenido consumidores.
- **Rutas del gateway:** contrato público en `/v2/auth/` y `/v2/traslados`; API interna de la aplicación web en `/api/v1/<servicio>/`.
- **Capacidad consumida — Clima del sistema de aeropuerto** ([ADR-012](adr/ADR-012-consumo-clima.md)): Operaciones consulta la operabilidad del aeropuerto (`GET /v1/airports/SACO/conditions`, contrato fijado en [`contracts/consumidos/`](contracts/consumidos/README.md)) al aceptar un traslado al aeropuerto dentro de las próximas 6 h; si no es operable, el traslado queda con alerta climática y decide el operador. Es fail-open (si el proveedor falla, la alerta queda "clima no verificado" y la aceptación sigue), con timeout de 1 s, un reintento solo ante errores transitorios, circuit breaker (5 fallas, 30 s), caché local de 5 min y test de contrato con `kin-openapi`.

## 7. Búsqueda, caché y balanceo de carga
<!-- Ver D6, D7 y D12. -->

## 8. Resiliencia y observabilidad
<!-- Ver D10 y D11. -->

## 9. Distribución y despliegue
**Local (procedimiento único, consigna §2.12):** `docker compose up --build` levanta todo en contenedores:

| Contenedor | Puerto publicado | Notas |
| --- | --- | --- |
| `gateway` | **8080** | Único punto de entrada |
| `identidad`, `operaciones`, `recursos`, `notificaciones`, `integracion` | — | Solo accesibles a través del gateway (puertos internos 8081 a 8085) |
| `mysql` | 13306 (en el host; 3306 dentro de Docker) | Bases `operaciones` y `recursos` |
| `mongo` | 27017 | Bases `identidad`, `notificaciones` e `integracion` |
| `rabbitmq` | 5672 · 15672 (panel) | Broker |

Los scripts de `infra/` crean, al iniciar por primera vez, una base de datos y un usuario por servicio. Los secretos salen de `.env`, que no se versiona.

**Nube:** la capacidad publicada (Integración de carga) tiene que quedar accesible con una URL pública para la Presentación Grupal. En clase se pidió usar servicios simples y gratuitos (por ejemplo Render, Railway, Vercel o Netlify), no AWS. Pendiente de decidir.

## 10. Limitaciones conocidas y deuda técnica aceptada
<!-- Obligatoria y actualizada para la Presentación Grupal. -->
