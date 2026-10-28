# La materia

- **Materia:** Arquitectura de Software (ARQ 2026 · UCC), Ingeniería en Informática.
- **Práctico:** Tomás Cassanelli (también dicta Desarrollo de Software). Estilo: clases prácticas, demos en vivo, actividades con variante personalizada por estudiante vía formulario.
- **Repo de práctica:** `github.com/TomiCassanelli/arquitectura-de-software` (rama `main`, una carpeta por clase: `PRESENTACION/`, `DEMO/`, `ACTIVIDADES/`, `ACTIVIDADES_RESUELTAS/`). Ya no se guarda clonado: resumido en `contexto/practica/`.
- **Formato del teórico:** apuntes PDF con estructura fija: introducción → conceptos → patrones → caso **EventOS** → casos reales (Netflix, Uber, Amazon, Stripe…) → tabla de trade-offs → puntos clave → referencias.

## Cronograma visto (a oct. 2026)

| # | Teórico (apunte) | Práctico (repo) |
| --- | --- | --- |
| 1 | Monolito → Microservicios + estrategias de migración | REST en Gin, monolito del gimnasio, interfaces y mocks |
| 2 | Datos en microservicios (DB per service, Saga, Outbox, MongoDB) | MongoDB + Docker + Repository Pattern + tests con mock |
| 3 | Caché | Caché: ccache (local), Memcached (distribuida), degradación elegante |
| 4 | Comunicación asíncrona | RabbitMQ: ACK manual, QoS/fair dispatch, fanout, DLQ, backoff |
| 5 | Motores de búsqueda, relevancia y RAG (9/9/2026) | Apache Solr: q/fq/qf/edismax + API Go detrás de interfaz |
| 6 | API Gateway, Load Balancing, Service Discovery (16/9/2026) | Concurrencia en Go: goroutines, channels, select, mutex, worker pool, context |
| 7 | Resiliencia ante fallos parciales (23/9/2026) | Balanceo con NGINX + 3 réplicas en Docker Compose |
| 8 | Estilos de arquitectura de backend (DDD, Hexagonal, Clean, CQRS, Event Sourcing, reactivo) | Observabilidad: Prometheus, logs en MySQL, TrackID, Grafana (`practica/clase-8-observabilidad.md`) |
| 9 | Comunicación entre servicios: contratos y compatibilidad, REST, gRPC, GraphQL, SSE, WebSockets, webhooks, IA externa (14/10/2026) | _(sin práctico en el repo todavía)_ |
| próx. | (apuntes mencionan "Producción y despliegue": SLOs, observabilidad) | Monitoreo: logs, métricas, trazas |

Resúmenes: `teoria/01..09-*.md` y `practica/*.md`.

## Stack de la cursada ("NECESITAMOS TENERLAS A TODAS")

Git, VS Code, **Go** (+ **Gin**), Node.js (frontend: npm/TypeScript), Postman/Swagger, SQL Workbench (MySQL), **Docker**,
**MongoDB**, **Memcached/Redis**, **Apache Solr**, **RabbitMQ**, **NGINX**.

## Caso de referencia del teórico: EventOS

Plataforma de venta de tickets online. Dominios: **Usuarios, Eventos, Tickets, Pagos** (luego Catálogo, Búsqueda, Carrito, Órdenes, Notificaciones, Analítica).
Nació como monolito Go + MySQL compartido; se extrae **Tickets** primero (pico 50x en festivales) con Strangler Fig.
Todos los apuntes usan EventOS para ilustrar: el proyecto final probablemente se evalúe con el mismo tipo de razonamiento.

## Proyecto final

Consigna confirmada: ver `proyecto/consigna.md` (transcripción de `Enunciado TP Final.pdf`).
