# ADR-017 — Reconexión al broker y resolución dinámica de réplicas (reemplaza parcialmente a ADR-008)

- **Fecha:** 2026-10-16
- **Estado:** aceptada
- **Reemplaza a:** la fila "Todos — RabbitMQ al arrancar" de [ADR-008](ADR-008-resiliencia.md) (`mensajeria.ConectarConReintentos`: 5 intentos y, si fallan, el proceso termina) y la frase "el consumidor de estados reconecta al reiniciar" de la fila "Integración de carga — RabbitMQ". El resto de ADR-008 sigue vigente.
- **Resuelve:** los dos ajustes pendientes que dejó la validación de D5 en [ADR-016](ADR-016-validacion-d5.md).

## Contexto
ADR-004 y ADR-008 prometen que, **con RabbitMQ caído, Integración sigue aceptando solicitudes** (`202`): quedan pendientes de envío y el relay las publica cuando el broker vuelve. La validación de ADR-016 mostró que la implementación no lo cumplía:
- cuando se cerraba el canal, el consumidor devolvía error y `main` terminaba el proceso (`os.Exit(1)`) para que Docker lo reiniciara;
- al reiniciar, el servicio esperaba a RabbitMQ **antes** de abrir el puerto HTTP, así que `POST /v2/traslados` respondía `502` durante toda la caída;
- al volver, NGINX seguía respondiendo `502`: había resuelto las IP de las réplicas al arrancar y los contenedores reiniciados tenían otras.

Lo reprodujimos el 2026-10-16 (tres `POST` con el broker detenido: `502 502 502`).

Lo que dice el teórico:
- **Teórico 7:** el aislamiento de fallas se diseña. Una dependencia caída no puede llevarse puesta una capacidad que no la necesita: aceptar una solicitud solo necesita MongoDB.
- **Teórico 6:** *vivo ≠ listo ≠ dependencia degradada*. Que el broker no esté es una dependencia degradada, no un motivo para que el proceso deje de estar vivo.
- **Teórico 4:** el outbox existe justamente para desacoplar la escritura del negocio de la publicación.

## Decisión
1. **El proceso no termina cuando se cae RabbitMQ.** `comun/mensajeria.Conexion` mantiene la conexión en segundo plano:
   - escucha `NotifyClose` y vuelve a conectar con backoff exponencial y jitter (1 s, 2 s, 4 s… hasta 30 s);
   - reintenta mientras el servicio esté vivo, sin límite de intentos.
2. **El HTTP arranca sin esperar al broker.** `mensajeria.Conectar` devuelve enseguida.
3. **Consumidores:** `mensajeria.Consumir` espera a que haya conexión, consume, y si el canal se cierra vuelve a empezar. Los mensajes sin ACK vuelven a la cola y se entregan de nuevo (at-least-once). Los consumidores ya eran idempotentes.
4. **Publicador:** abre el canal cuando lo necesita y lo descarta ante cualquier falla. Vuelve a declarar los exchanges y las colas en cada canal nuevo. Sin broker, `Publicar` falla enseguida con `ErrSinConexion`, y el relay deja el mensaje pendiente (en el outbox de Operaciones, o con `pendiente_envio` en Integración).
5. **Balanceador (ADR-009):**
   - NGINX vuelve a resolver los nombres de las réplicas cada 10 s con el DNS de Docker (`resolver 127.0.0.11 valid=10s`, más `zone` y `resolve` en los `server` del upstream);
   - la imagen se fija en `nginx:1.28-alpine`, porque `resolve` existe desde NGINX 1.27.3.

**Librerías:** se sigue usando `github.com/rabbitmq/amqp091-go`, la librería de la clase práctica. Esta librería no reconecta sola: su documentación deja la reconexión a cargo de la aplicación, escuchando `NotifyClose`. Por eso `Conexion` es pegamento mínimo (unas 100 líneas) sobre la librería, no un cliente propio. En NGINX se usa una directiva propia.

## Alternativas consideradas
- **Dejar que Docker reinicie el proceso** (lo que había). Descartada: mientras el broker no está, el servicio no atiende HTTP, y la caída del broker se propaga al consumidor externo. Es lo contrario de lo que promete el outbox.
- **Cambiar a una librería con reconexión automática** (por ejemplo `github.com/wagslane/go-rabbitmq`). Descartada por ahora:
  - obliga a reescribir `comun/mensajeria`: reintentos con TTL, DLQ, confirmaciones y propagación de trazas;
  - la reconexión que hace falta es poca y la librería oficial documenta cómo hacerla;
  - cambiaría la librería vista en clase.
- **Health check activo en NGINX.** Descartado: es una función de NGINX Plus. El health check pasivo de ADR-009 más `resolve` alcanzan.
- **Reiniciar NGINX al recrear las réplicas.** Descartado: es un paso manual, justo lo que la consigna (§2.12) pide evitar.

## Consecuencias
- **Ganamos:**
  - con RabbitMQ caído, `POST /v2/traslados` responde `202` y nada se reinicia;
  - al volver el broker, los pendientes se publican solos;
  - las réplicas se pueden recrear sin tocar el balanceador.
- **Pagamos:**
  - un servicio puede estar vivo y atendiendo mientras sus consumidores están detenidos. Hay que mirarlo en los logs (`se cerró la conexión con RabbitMQ`, `consumidor listo`) y en `rumbo_outbox_pendientes`, que crece mientras el broker no está;
  - NGINX hace una consulta DNS cada 10 s.

## Cómo se verifica
- **Test de integración** con RabbitMQ real (`comun/mensajeria/conexion_test.go`, testcontainers): el broker corta todas las conexiones (`rabbitmqctl close_all_connections`), y el publicador y el consumidor vuelven a funcionar sin reiniciar nada. Otros tests del mismo archivo:
  - un error transitorio se reintenta;
  - uno permanente va a la DLQ sin reintentos;
  - sin broker, `Publicar` falla enseguida.
- **Test del relay** (`integracion/messaging/relay_test.go`): con el broker caído la solicitud sigue pendiente y no se publica dos veces mientras el reclamo está vigente.
- **Sobre el sistema levantado** (2026-10-16, ver [evidencias](../evidencias/entrega-2.md) §6):
  - con `rabbitmq` detenido, cinco `POST` dieron `202 202 202 202 202` y ningún contenedor se reinició;
  - al volver, los servicios loguearon `conectado a RabbitMQ`, y quedaron 0 pendientes en MongoDB y en el outbox de MySQL;
  - al recrear las dos réplicas con IP nuevas (.8 y .10 pasaron a .15 y .16), NGINX siguió respondiendo `200` y alternando entre las IP nuevas, sin reiniciarse.
