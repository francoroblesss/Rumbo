# Práctico — Clase 4: Asincronismo con RabbitMQ

Fuente: repo del docente, `CLASE_4/` (`PRESENTACION/Presentacion.pdf` 15 págs., `README.md`, `compose.yaml`, `DEMO/1..3`, `ACTIVIDADES/`, `ACTIVIDADES_RESUELTAS/`).

La clase va de lo más simple a lo más completo. "El objetivo no es memorizar la API: es observar qué ocurre cuando un productor, una cola y uno o varios consumidores trabajan a ritmos distintos." Orden: Demo 1 → Actividad 1 (QoS) → Actividad 2 (Fanout) → Demo 2 (DLQ, cierre guiado) → Demo 3 (backoff).

## Diccionario
| Término | Significado |
| --- | --- |
| Productor | Envía un mensaje |
| Broker | Recibe, guarda y entrega mensajes |
| Cola | Espera los mensajes hasta que alguien los procese |
| Consumidor | Recibe y procesa mensajes |
| ACK | "Terminé de procesarlo" |
| DLQ | Cola donde apartamos mensajes rechazados |

## Conceptos
- **Síncrono = llamada telefónica** (espera en línea; el error se conoce al instante; HTTP request/response). **Asíncrono = bandeja de pedidos** (deja el pedido y sigue; un worker lo toma cuando puede).
- "En asincronismo, **'recibido' no significa 'terminado'**": significa "guardé el trabajo y lo procesaré después". La diferencia no es velocidad: **el emisor no queda esperando**.
- **Tres momentos de un trabajo asíncrono** (analogía restaurante): **Aceptación** (respondemos "recibido", podemos dar un ID) → **Registro durable** (cola o base) → **Finalización** (un worker lo procesa). "Primero guardamos el trabajo. Después confirmamos que fue recibido... Así no respondemos 'ok' por algo que todavía no quedó registrado." La Demo 1 muestra el tercer momento.
- **Tres desacoples del broker:** **espacial** (máquinas distintas) · **temporal** (no tienen que estar encendidos a la vez) · **de ritmo** (la cola absorbe picos; lo muestra la actividad de QoS).

## RabbitMQ con Docker
```bash
docker run -d --name clase-rabbitmq -p 5672:5672 -p 15672:15672 \
  -e RABBITMQ_DEFAULT_USER=user -e RABBITMQ_DEFAULT_PASS=pass rabbitmq:3-management
# o, desde CLASE_4 (compose.yaml: mismo contenedor clase-rabbitmq):
docker compose up -d
```
- Panel http://localhost:15672 (`user`/`pass`). URI de las demos `amqp://user:pass@localhost:5672`. Librería `github.com/rabbitmq/amqp091-go` **v1.14.0**, go 1.25.0.
- **Panel → Queues:** **Ready** = esperando consumidor; **Unacked** = entregados que no terminaron; **Total** = Ready + Unacked. "El panel es nuestro observatorio."
- Si se cambia la configuración de una cola durable, RabbitMQ no permite redeclararla con argumentos distintos → detener programas y reiniciar el contenedor.
- Cada programa en su terminal; `CTRL+C` para detener.

## Demo 1 — productor → `cola-pedidos` → consumidor (ACK manual)
- Intencionalmente mínima: sin JSON, exchanges ni manejo de errores.
- Productor: `amqp.Dial` → `conn.Channel()` ("es por donde enviamos las operaciones") → `QueueDeclare("cola-pedidos", true, false, false, false, nil)` (crea si no existe) → `ch.Publish("", q.Name, false, false, amqp.Publishing{Body: []byte("pedido-101")})` (`""` = exchange por defecto). Sin consumidor activo se ve **Ready=1**.
- Consumidor: también declara la cola ("si ya existe, RabbitMQ no la duplica"); `Consume(queue, "", false /*autoAck*/, ...)`; simula **10 s** de trabajo y recién entonces `d.Ack(false)`. Mientras procesa: Ready baja, Unacked sube.
- "**ACK no significa 'lo leí'. Significa 'terminé de procesarlo'.**" Ejecutar primero el consumidor, luego el productor.

## Actividad 1 — Fair Dispatch y QoS
- Objetivo: repartir entre dos consumidores sin sobrecargar al más lento (el rápido recibe más trabajo).
- Código: cola durable `actividad-1-pedidos`; productor publica `pedido-1..10` (`PublishWithContext`, `DeliveryMode: amqp.Persistent`, 1 s entre cada uno). Consumidor con env `CONSUMIDOR` (nombre) y `DELAY_SEGUNDOS` (default 2), ACK manual tras el delay.
- **TODO → solución:** antes de `Consume`, `ch.Qos(1, 0, false)` (prefetch = 1 mensaje sin confirmar por consumidor). Sin QoS el lento puede acumular varios Unacked; con QoS=1 el rápido toma los que el lento no puede.
```bash
CONSUMIDOR=lento DELAY_SEGUNDOS=5 go run ./consumidor
CONSUMIDOR=rapido DELAY_SEGUNDOS=1 go run ./consumidor
go run ./productor
```

## Actividad 2 — Fanout Exchange (publish/subscribe)
- Una tienda confirma un pedido; **Facturación** (genera factura) y **Logística** (prepara envío) deben recibir **una copia cada una, sin compartir cola**.
```
tienda -> exchange fanout ex.pedido.confirmado -> q.facturas  -> facturación
                                               -> q.logistica -> logística
```
- Todos declaran `ExchangeDeclare("ex.pedido.confirmado", "fanout", true, ...)` (el productor también, para poder correr por separado). Cada consumidor declara su cola durable y consume con autoAck=true.
- Productor: body `{"pedido_id":"PED-1","estado":"confirmado"}`, `context.WithTimeout(5s)`, `PublishWithContext(ctx, exchange, "", ..., amqp.Publishing{ContentType: "application/json", DeliveryMode: amqp.Persistent, Body})`.
- **TODO → solución:** `ch.QueueBind(queueName, "", exchangeName, false, nil)` en facturación y luego en logística (en fanout la routing key puede ser `""`; no reutilizar `q.facturas` en logística).
- Pasos: bind facturación → publicar (solo facturación recibe) → bind logística → publicar (ambos reciben). Reiniciar el consumidor tras cada cambio. Sin binds, el productor no genera mensajes en ninguna cola.
- "En QoS miramos reparto de trabajo; en Fanout, una copia del mismo evento para dos destinos."

## Demo 2 — DLQ mínima (ACK + NACK + DLQ)
- Sin DLQ: un mensaje con error bloquea a los siguientes. Con DLQ: NACK → se aparta y la cola principal sigue. "**La DLQ no arregla los mensajes, solo los aparta**"; después un operador revisa y decide.
- Consumidor (declara ambas colas, por eso arranca primero): `QueueDeclare("demo.pedidos.dlq", ...)` y la principal `demo.pedidos` con `amqp.Table{"x-dead-letter-exchange": "", "x-dead-letter-routing-key": "demo.pedidos.dlq"}`. Si el body es `"ERROR"` → `delivery.Nack(false, false)` (solo este mensaje, **sin reencolar**) → RabbitMQ lo mueve a la DLQ; si no → `Ack(false)`.
- Productor publica `pedido-1`, `ERROR`, `pedido-2`. `go test ./...` en `DEMO/2` solo compila productor y consumidor.

## Demo 3 — Backoff exponencial de conexión
- `connectWithBackoff`: `maxAttempts = 4`; espera `time.Second * time.Duration(1<<(attempt-1))` → **1 s, 2 s, 4 s**; tras el 4.º intento, error final.
- Probar: `docker compose down` → `go run .` → durante los reintentos `docker compose up -d`.
- "Esperar cada vez más **protege al broker**: evita golpearlo miles de veces mientras se recupera."
- **Qué no demuestra:** no es reconexión completa de una app ya en marcha, solo la conexión inicial. **Jitter, reintentos de mensajes e idempotencia** quedan para una clase posterior.
