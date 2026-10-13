# Convenciones de código (derivadas del repo de la cátedra)

Referencias concretas (código resumido en `practica/`): Clase 2 demo (`practica/clase-2-mongodb-repository-pattern.md`), Clase 3 demo (`practica/clase-3-cache.md`), Clase 5 actividad 2 resuelta (`practica/clase-5-solr.md`), Clase 4 resueltas (`practica/clase-4-rabbitmq.md`).

> **Prioridad:** estas convenciones toman estilo, librerías y estructura del práctico. Si una regla del práctico contradice al teórico (`teoria/`), **gana el teórico** (ej.: el práctico ignora errores de `Set`, usa `context.TODO()` y no hace idempotencia ni jitter; en el proyecto se siguen las reglas de los teóricos 4 y 7).

## Stack y versiones
| Pieza | Paquete / imagen | Versión vista en clase → **usada en Rumbo** |
| --- | --- | --- |
| Go | — | `go 1.25.0` (algunas act. `1.22`) → **`go 1.26.0`** (lo exige OpenTelemetry) |
| HTTP | `github.com/gin-gonic/gin` | v1.10 – v1.12 → **v1.12** |
| MongoDB | `go.mongodb.org/mongo-driver` (v1) · imagen `mongo:4.4.6` | v1.17.9 → **v1.17 · imagen `mongo:7`** |
| MySQL | `database/sql` + `github.com/go-sql-driver/mysql` · imagen `mysql:8.4` | (MySQL en DSW con GORM) → **go-sql-driver/mysql v1.10, SQL explícito** (ADR-005) |
| Caché local | `github.com/karlseguin/ccache/v3` | v3.0.8 |
| Memcached | `github.com/bradfitz/gomemcache/memcache` · imagen `memcached` | — |
| RabbitMQ | `github.com/rabbitmq/amqp091-go` · imagen `rabbitmq:3-management` | v1.14.0 → **v1.15** |
| IDs | `github.com/google/uuid` | → **v1.6** (correlation id en el gateway) |
| Tokens / rate limit (gateway) | `github.com/golang-jwt/jwt/v5` · `golang.org/x/time/rate` | → previstas (pendiente de implementar) |
| Solr | HTTP plano (`net/http`) · imagen `solr:9` | — |
| LB | imagen `nginx:alpine` | — |
| Build | `golang:1.22-alpine` (actualizable a la versión de go.mod) | → **multi-stage `golang:1.26-alpine` → `alpine:3.20`**, contexto de build `servicios/` (para incluir `comun/`) |

## Estructura de un microservicio
Un módulo Go por servicio (carpeta propia, `go.mod` propio, `main.go` propio, **puerto propio**):
```
<servicio>/
├── main.go                 # cableado (DI manual) + rutas + router.Run
├── go.mod                  # la cátedra usa `module main`; en Rumbo: github.com/francoroblesss/Rumbo/servicios/<servicio>
├── config/                 # constantes / lectura de env (PORT, URIs)
├── controllers/<entidad>/  # Gin handlers: leen params/body, llaman al service, devuelven JSON
├── services/<entidad>/     # lógica de negocio; depende de INTERFACES
├── repositories/<entidad>/ # interfaz + impl real (mongo/solr/...) + mock + decoradores de caché
├── models/<entidad>/       # structs de dominio (tags bson/json). La cátedra también usa `dao/` y `dto/`
├── messaging/              # publisher/consumer RabbitMQ (ej. rabbitmq_publisher.go)
├── database/               # conexión (ConnectDB)
└── services/<entidad>/*_test.go  # tests del service con el mock
```
**En Rumbo:** esa estructura en capas la usan Identidad, Recursos, Notificaciones e Integración de carga. **Operaciones es hexagonal** (teórico 8): `dominio/` (sin tags json/db), `aplicacion/` (casos de uso), `puertos/` (interfaces de entrada y salida) y `adaptadores/` (`api`, `mysql`, `mensajeria`, `recursos`, `identidad`, `clima`). El gateway usa `net/http` + `httputil.ReverseProxy`. Todos los módulos se agrupan con `go.work` en la raíz.

Dos estilos de código coexisten en la cátedra; elegir uno por proyecto y mantenerlo:
- **Estilo Clase 2/3:** structs con campos exportados (`ItemsService{Repo: ...}`), receptores por valor en repos, métodos `GetByID`, `GetItemByID`, `CreateItem`.
- **Estilo Clase 5 (resuelta, más prolijo):** constructores `NewService(engine)`, `NewClient(baseURL, httpClient)`, `NewHandler(svc).Register(router)`, campos privados, `context.Context` como primer parámetro.
→ **Default para el proyecto: estilo Clase 5** (constructores + `ctx`), que además habilita timeouts/cancelación (teórico 7).

## Reglas
- **Usar `servicios/comun`** para observabilidad (`observabilidad.Iniciar`, `Instrumentar`, `ClienteHTTP`), mensajería (`mensajeria.NuevoSobre`, `Publicador`, `Consumir`), identidad (`identidad.Desde`, `RequiereRol`) y errores (`problema.Escribir`). Contrato en `docs/interfaces-internas.md`. Logs con `slog.InfoContext(ctx, …)`, no `log.Printf`.
- **No reinventar la rueda (indicación del profesor en clase):** caché, búsqueda, mensajería, validación de tokens, rate limiting, etc. se resuelven con **librerías o herramientas existentes**, preferentemente las vistas en clase. Nunca una caché "hecha a mano" en Go. Solo se escribe código propio para la lógica de negocio y el pegamento mínimo.
- **El service depende de interfaces**, nunca de Mongo/Solr/Memcached directamente. El controller no conoce la infraestructura.
- **DI manual en `main.go`**, en orden: infraestructura → repo → (decorador caché) → service → controller → rutas.
- **Caché como decorador del repositorio** (`NextRepo`), cache-aside, degradación elegante: fallo de caché → log + fuente; nunca 500 por la caché.
- **Mocks** implementan la misma interfaz con `map` en memoria; se usan en tests y como fallback de dependencias entre servicios.
- **Rutas REST**: plural, `/:id`, verbos correctos. Respuestas `gin.H{"error": err.Error()}` con status adecuado (400 validación, 404 no encontrado, 201 creado, 202 aceptado async, 429, 502/503/504 dependencias).
- **Errores**: envolver con `fmt.Errorf("contexto: %w", err)`; mensajes en español.
- **Llamadas HTTP salientes**: `http.NewRequestWithContext` + `http.Client{Timeout: ...}`. Nunca sin timeout.
- **RabbitMQ**: declarar exchange/cola idempotentemente, mensajes JSON persistentes, `PublishWithContext` con timeout, **ACK manual después del efecto**, QoS en consumidores, consumidores idempotentes. Evento con `id`, `tipo`, `version`, `ocurrido_en`, `correlation_id` (teórico 4).
- **Config por env**: en Rumbo la variable es `PUERTO` (la cátedra usa `PORT`, CLASE_3), con default; DSN de MySQL y URIs de Mongo/Rabbit por env (`MYSQL_DSN`, `MONGO_URI`, `RABBITMQ_URI`) para que funcionen en Docker Compose (nombre de servicio como host) y en local (`localhost`). Se leen en `config/config.go` con `config.Cargar()`.
- **Stateless**: nada de sesión/estado de usuario en memoria de la API (para poder poner réplicas detrás de NGINX).
- **Comentarios**: la cátedra comenta en español explicando el *porqué* (didáctico). Mantener ese tono, sin exagerar.
- **Logs**: `log.Println` / `log.Printf` con prefijos legibles; `log.Fatal` solo en arranque.
- **Docker**: un `compose.yaml` en la raíz que levante infra + servicios; healthchecks en la infraestructura. **Credenciales**: nunca en el repo (consigna §2.12); se leen de `.env` (copiado de `.env.example`). Cada servicio tiene **su propia base de datos y su propio usuario** (ADR-003): no usar `root/root` ni un usuario compartido.
- **Puertos**: gateway 8080 (único publicado), identidad 8081, operaciones 8082, recursos 8083, notificaciones 8084, integracion 8085.

## Cómo se prueba (como pide la cátedra)
- `docker compose up --build` levanta todo (es el procedimiento único que pide la consigna). Para desarrollar un servicio suelto: `go run .` desde su carpeta, con la infraestructura levantada.
- `go mod tidy && go run .` por servicio, **cada uno en su terminal / puerto** ("si la solución solo necesita un puerto, todavía no está resuelta").
- `go test ./...` con mocks; `go run -race` si hay concurrencia.
- `curl` / Postman contra cada endpoint; panel de RabbitMQ (15672) y Solr Admin (8983) como observatorio.
