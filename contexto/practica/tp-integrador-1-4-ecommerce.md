# Práctico — Trabajo práctico integrador 1-4: e-commerce y logística

Fuente: repo del docente, `. TRABAJO_PRACTICO_1_4/` (la carpeta empieza con ". "). Integra Clases 1 a 4 (microservicios, capas, caché, RabbitMQ).

## Idea
Partir de un monolito simple de e-commerce (clientes, productos y pedidos en el mismo proceso, `:8080`) y transformarlo gradualmente en un pequeño **monorepo de microservicios** que publique un evento cuando un pedido queda confirmado.
```
Antes:   cliente -> monolito :8080
Después: cliente -> clientes :8081
                 -> pedidos  :8082
         pedidos -- evento pedido.confirmado --> RabbitMQ --> logística (conceptual)
```
**Logística no se implementa**: solo representa al sistema que tomaría el evento para preparar un envío.

## Monolito base (`monolito/`, módulo `monolito-ecommerce`, go 1.25.0, Gin v1.10.0)
| Método | Endpoint | Respuesta fija del monolito |
| --- | --- | --- |
| `POST` | `/clientes` | 201 `{"mensaje": "cliente creado"}` |
| `GET` | `/clientes/:id` | 200 `{"id", "nombre": "Ana Pérez"}` |
| `GET` | `/productos` | 200 `{"productos": [{P-1 Auriculares stock 10}, {P-2 Teclado stock 8}]}` |
| `POST` | `/pedidos` | 201 `{"mensaje": "pedido confirmado", "pedido_id": "PED-1"}` |

## Qué hay que hacer
1. Ejecutar y entender el monolito (`go mod tidy && go run .`).
2. Separar en dos microservicios: **`clientes`** (alta y consulta) y **`pedidos`** (consulta de productos y confirmación de pedidos).
3. Cada uno en un **puerto diferente** y en su propia terminal.
4. Organización por capas: **controlador, servicio y repositorio**.
5. **Caché simple** (en memoria) para las consultas de productos.
6. Publicar **`pedido.confirmado`** en RabbitMQ al confirmar un pedido.

No hace falta: autenticación, frontend, pagos, stock real, base de datos real ni microservicio de logística (datos en memoria).

### Requisitos mínimos
- **`clientes`** — `:8081`; `POST /clientes`, `GET /clientes/:id`; controlador/servicio/repositorio; datos en memoria. "No copiar la lógica de productos, pedidos ni RabbitMQ: esas responsabilidades pertenecen al otro microservicio."
- **`pedidos`** — `:8082`; `GET /productos`, `POST /pedidos`; controlador/servicio/repositorio; caché en memoria para `GET /productos`; publicar el evento al confirmar.
- `microservicios/eventos/` queda disponible para modelos/utilidades de eventos.

### RabbitMQ dentro de `pedidos`
```
POST /pedidos -> servicio confirma el pedido -> publica pedido.confirmado -> cola pedidos-confirmados -> logística podría preparar el envío
```
- La publicación se hace **en el servicio, después de confirmar**.
- Evento sugerido: `{"tipo": "pedido.confirmado", "pedido_id": "PED-1", "cliente_id": "C-1", "producto_id": "P-1"}`.
- Primera versión: publicar en la cola **`pedidos-confirmados`**. **No** hace falta consumidor, DLQ, fanout, backoff ni reintentos.
- Organización sugerida: `pedidos/{controllers/, services/pedido_service.go, repositories/, messaging/rabbitmq_publisher.go, main.go}`. En `rabbitmq_publisher.go` van `amqp.Dial`, `QueueDeclare("pedidos-confirmados", ...)` y `Publish(...)`.
- RabbitMQ se levanta desde `CLASE_4` con `docker compose up -d`.

## Entrega y criterio
- Entregar el código dentro de `microservicios/`.
- **Terminado** cuando el monolito puede reemplazarse por los dos microservicios en puertos distintos y un pedido confirmado genera el evento en RabbitMQ.
- El README lista un `ARQUITECTURA.md` en la estructura inicial que **no existe en el repo**.
