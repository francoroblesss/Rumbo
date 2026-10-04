# Clases prácticas — índice

Resúmenes del repo del docente de práctico (Tomás Cassanelli): `github.com/TomiCassanelli/arquitectura-de-software`, rama `main`, último commit leído `30a835b` "feat: upload demo and solved activities" (3/10/2026). **El clon local se borró**: estos resúmenes son la fuente del práctico. Si el docente sube clases nuevas, clonar el repo temporalmente, resumir lo nuevo acá y volver a borrarlo.

Cada `CLASE_N/` tiene `PRESENTACION/*.pdf` y, según la clase, `DEMO/`, `ACTIVIDADES/` (con `TODO`) y `ACTIVIDADES_RESUELTAS/`. **Las resueltas son la mejor referencia del estilo esperado** (ver `../convenciones-codigo.md`). Las actividades usan variantes por estudiante asignadas por formulario (`FORMULARIOS`, `GUIA_DOCENTE.md` y `EXTRA_melisearch` están en el `.gitignore` del repo y no se publican).

| Archivo | Clase | Tecnología / tema |
| --- | --- | --- |
| `clase-1-monolito-rest-interfaces-mocks.md` | 1 | Stack, monolito vs microservicios, REST en Gin, interfaces y mocks, TP del gimnasio |
| `clase-2-mongodb-repository-pattern.md` | 2 | MongoDB en Docker, driver Go, Repository Pattern, test con mock |
| `clase-3-cache.md` | 3 | Patrones de caché, ccache (local), Memcached, degradación elegante |
| `clase-4-rabbitmq.md` | 4 | RabbitMQ: ACK manual, QoS, fanout, DLQ, backoff |
| `clase-5-solr.md` | 5 | Solr: análisis, ranking, q/fq/qf/rows/fl, API Go detrás de interfaz |
| `clase-6-concurrencia-go.md` | 6 | Goroutines, channels, select, mutex, WaitGroup, fan-out/in, worker pool, context |
| `clase-7-balanceo-nginx.md` | 7 | HA vs resiliencia, escalado, algoritmos de LB, stateless, NGINX con 3 réplicas |
| `tp-integrador-1-4-ecommerce.md` | 1–4 | TP: monolito e-commerce → `clientes` + `pedidos` + caché + evento en RabbitMQ |

**TPs previos:** el TP del gimnasio (Clase 1) y el TP integrador 1-4 figuraban como **ya entregados** por el grupo (fuera de este repo), según el contexto armado el 25/9/2026. Sirven como antecedente: el proyecto final escala esas mismas ideas.

Próxima clase anunciada (Clase 7): **Monitoreo** — logs, métricas y trazas.

## Comandos Docker de cada tecnología (tal como los usa la cátedra)
| Tecnología | Comando / archivo | Acceso |
| --- | --- | --- |
| MongoDB | `docker run -p 27017:27017 -e MONGO_INITDB_ROOT_USERNAME=root -e MONGO_INITDB_ROOT_PASSWORD=root --name my-mongo mongo:4.4.6` | `mongodb://root:root@localhost:27017` |
| Memcached | `docker run --name memcached -p 11211:11211 -d memcached` | `localhost:11211` |
| RabbitMQ | `CLASE_4/compose.yaml` (`rabbitmq:3-management`, `user`/`pass`) | `amqp://user:pass@localhost:5672`, panel http://localhost:15672 |
| Solr | `CLASE_5/docker-compose.yml` (`solr:9`, `solr-precreate products`, healthcheck) + `schema.json` + `products.json` | http://localhost:8983/solr/ |
| NGINX | `CLASE_7/DEMO/compose.yaml` (`nginx:alpine`, `8080:80`, `nginx.conf` montado `:ro`) | http://localhost:8080 |

## Versiones vistas en el repo
Go `1.25.0` (Clases 1–4, TP) y `1.22` (Clases 5 y 7) · Gin `v1.12.0` (Clases 1–3) y `v1.10.0` (TP, Clases 5 y 7) · mongo-driver `v1.17.9` · ccache `v3.0.8` · amqp091-go `v1.14.0` · imagen de build `golang:1.22-alpine`.
