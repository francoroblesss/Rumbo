# Interfaces internas entre servicios

> Contrato **interno** entre los servicios de Rumbo: mensajes, endpoints internos, identidad, errores, logs, métricas y trazas.
> El contrato **público** (para otros grupos) está en [`contracts/`](contracts/README.md).
> Implementa las decisiones de [ADR-004](adr/ADR-004-comunicacion-entre-servicios.md) (comunicación), [ADR-002](adr/ADR-002-contrato-propio.md)/[ADR-006](adr/ADR-006-contrato-generico-v2.md)/[ADR-019](adr/ADR-019-contrato-sin-version-en-la-ruta.md)/[ADR-021](adr/ADR-021-capacidad-de-autoservicio.md) (contrato público de autoservicio, rutas sin versión), [ADR-022](adr/ADR-022-consistencia-y-concurrencia.md) (consistencia y concurrencia), [ADR-024](adr/ADR-024-alcance-de-la-presentacion.md) a [ADR-027](adr/ADR-027-avisos-en-tiempo-real.md) (alcance de la Presentación, clima en las reservas, despliegue y avisos en tiempo real; §11 a §16) y el teórico 6 (identidad en el borde).
> Las piezas reutilizables (sobre, publicador, consumidor con reintentos y DLQ, observabilidad, identidad y errores) están en el módulo Go [`servicios/comun`](../servicios/comun). **Usarlo en lugar de reimplementarlas.**
> Si un servicio necesita algo que no está acá, **no se inventa**: se agrega primero a este documento.

## 1. Identificadores
| Entidad | Formato | Quién lo genera |
| --- | --- | --- |
| Solicitud | `sol-<uuid>` (ej. `sol-0b7e…`) | **Operaciones**, también para las reservas externas (Integración guarda su copia con el id que le devuelve Operaciones, ADR-021) |
| Traslado | `tra-<uuid>` | Operaciones, al aceptar la solicitud o al reservar una franja |
| Mensaje | `<uuid>` | Quien publica |
| Correlation id | `<uuid>` | El gateway (o el que llega en `X-Correlation-ID`) |

`<uuid>` = `github.com/google/uuid`, `uuid.NewString()`.

## 2. Identidad propagada por el gateway (teórico 6)
El gateway valida el token firmado y **reemplaza** estos headers. Si el cliente los manda, el gateway los **borra** antes de reenviar. Los servicios confían en ellos porque solo el gateway conoce el secreto interno (§11.2).

| Header | Contenido | Ejemplo |
| --- | --- | --- |
| `X-Rumbo-Usuario` | `sub` del token (id de usuario o `client_id` del sistema externo) | `usr-5f1c…` · `org-almacenamiento` |
| `X-Rumbo-Organizacion` | `org` del token; vacío para personal de Rumbo | `org-7a2e…` |
| `X-Rumbo-Roles` | `roles` del token, separados por coma | `operador` · `sistema_externo` |
| `X-Correlation-ID` | Correlation id (se conserva si viene) | `3f6c1e2a-…` |

Roles posibles: `cliente`, `operador`, `chofer`, `administrador`, `sistema_externo`.
**Autorización distribuida:** cada servicio decide qué puede hacer cada rol sobre **sus** datos (paquete `comun/identidad`: `identidad.Desde(c)` y `identidad.RequiereRol(...)`).

### Token (emitido por Identidad, validado por el gateway)
JWT firmado **HS256** con `TOKEN_SECRETO`, librería `github.com/golang-jwt/jwt/v5`.

| Claim | Valor |
| --- | --- |
| `iss` | `rumbo-identidad` |
| `aud` | `rumbo` |
| `sub` | id del usuario o `client_id` |
| `org` | id de organización (omitido para personal de Rumbo) |
| `roles` | lista de roles |
| `iat`, `exp` | emisión y vencimiento (**1 h**) |

### Rutas públicas (sin token) en el gateway
- `POST /auth/token` (client credentials del contrato público) → Identidad `POST /auth/token`.
- `POST /api/identidad/login` (usuarios de la web) → Identidad `POST /login`.

Todo lo demás exige `Authorization: Bearer <token>` (401 `NO_AUTENTICADO` si falta o es inválido).

### Rutas que el gateway **no** expone
Todo path que contenga **`/internas/`** responde **404** en el gateway: son endpoints entre servicios.

### Límite de tráfico (gateway)
Token bucket por organización (o por usuario si no tiene organización), con `golang.org/x/time/rate`: **60 pedidos por minuto, ráfaga de 10**. Al superarlo, `429 DEMASIADOS_PEDIDOS` con `Retry-After` en segundos.

## 3. Errores (todos los servicios)
**Problem Details** (RFC 9457), `Content-Type: application/problem+json`, con el paquete `comun/problema`:
```json
{ "type": "https://rumbo.example/errores/estado-no-permite-operacion",
  "title": "El estado actual no permite la operación", "status": 409,
  "codigo": "ESTADO_NO_PERMITE_OPERACION", "detail": "La contrapropuesta venció.",
  "errores": [ { "campo": "cosa.cantidad", "mensaje": "es obligatorio" } ] }
```
Los `codigo` del contrato público son los de [`contracts/README.md`](contracts/README.md#errores). Los endpoints internos usan los mismos.

## 4. Endpoints internos (no pasan por el gateway público)

### 4.1 Operaciones — usados por Integración de carga (capacidad publicada, ADR-021)
Base: `OPERACIONES_URL` (en Docker, `http://operaciones:8082`). Headers: `X-Correlation-ID` y `traceparent` (los agrega `comun/observabilidad`); en los `POST`, además, `X-Rumbo-Organizacion` e `Idempotency-Key` (**la misma que mandó el consumidor externo**). Timeout del cliente: **2 s**, circuit breaker y sin reintento propio (ADR-004, ADR-008).

| Método y ruta | Cuerpo | Respuesta |
| --- | --- | --- |
| `GET /internas/disponibilidad?fecha=AAAA-MM-DD&peso_kg=` | — | `200 {"fecha", "franjas": [{"inicio", "fin", "disponible", "traslados_disponibles", "kg_disponibles"}]}` · `422 DATOS_INVALIDOS` (fecha mal formada o fuera del período reservable) |
| `POST /internas/reservas` | El cuerpo de `POST /traslados` del contrato público (`cosa`, `origen`, `destino`, `franja_inicio`, opcionales) | `201` con la **vista de solicitud** (§6), ya `ACEPTADA` con su traslado `PROGRAMADO` y `traslado.franja` · `409 SIN_DISPONIBILIDAD` / `CLAVE_IDEMPOTENCIA_REUTILIZADA` · `422 DATOS_INVALIDOS` / `ANTICIPACION_INSUFICIENTE` / `HORARIO_EN_EL_PASADO` / `EXCEDE_CAPACIDAD` |
| `POST /internas/solicitudes/{id}/cancelacion` | `{"motivo": "texto"}` | `200` con la vista (traslado `CANCELADO`, `cancelacion.tardia`) · `404` · `409 ESTADO_NO_PERMITE_OPERACION` · `422` |

- La reserva corre en **una transacción** (ADR-022): clave de idempotencia → validar la franja → bloquear la franja (`FOR UPDATE`) → descontar el lugar → crear la solicitud y el traslado → outbox (`operaciones.solicitud.aceptada`, versión 1) → guardar la respuesta de la clave.
- Operaciones guarda la `Idempotency-Key` por organización y endpoint (`reserva`, `cancelacion`) **en la misma transacción**: un reintento devuelve la respuesta original (también el `201`) aunque la franja se haya llenado.
- Cancelar una reserva externa nunca queda pendiente de aprobación: devuelve el lugar a la franja en la misma transacción.
- Operaciones verifica que la solicitud sea de `X-Rumbo-Organizacion` (si no, `404`).
- Los errores 4xx traen el `codigo` del contrato público: Integración los reenvía tal cual. Un 5xx, un timeout o el breaker abierto son `503 SERVICIO_NO_DISPONIBLE` con `Retry-After: 30`.

### Calendario de franjas (variables de entorno de Operaciones)
| Variable | Por defecto | Qué es |
| --- | --- | --- |
| `FRANJA_APERTURA_HORA` / `FRANJA_CIERRE_HORA` | `8` / `20` | Horario de las franjas (hora de Argentina) |
| `FRANJA_DURACION_HORAS` | `2` | Duración de cada franja |
| `FRANJA_CUPO_TRASLADOS` / `FRANJA_CUPO_KG` | `4` / `2000` | Cupo de cada franja |
| `FRANJA_ANTICIPACION_HORAS` | `2` | Anticipación mínima para reservar |
| `FRANJA_HORIZONTE_DIAS` | `30` | Hasta cuántos días adelante se reserva |

### 4.2 Operaciones — API del operador
Eliminada por [ADR-024](adr/ADR-024-alcance-de-la-presentacion.md). La API de la web está en §12.

### 4.3 Identidad
| Método y ruta (en Identidad) | Cuerpo | Respuesta |
| --- | --- | --- |
| `POST /auth/token` | `{"client_id","client_secret"}` | `200 {"access_token","token_type":"Bearer","expira_en_segundos":3600}` · `401 NO_AUTENTICADO` |
| `POST /login` | `{"email","password"}` | Igual que el anterior |

`GET /yo` y la siembra de usuarios de demostración: §15.

## 5. Mensajería (RabbitMQ)

### 5.1 Sobre (todo mensaje)
```json
{
  "id": "uuid",
  "tipo": "operaciones.solicitud.aceptada",
  "version": 1,
  "ocurrido_en": "2026-10-15T09:40:00-03:00",
  "correlation_id": "uuid",
  "agregado_id": "sol-…",
  "agregado_version": 4,
  "datos": { }
}
```
- Propiedades AMQP: `content_type=application/json`, `delivery_mode=2` (persistente), `message_id=id`, `type=tipo`.
- Headers AMQP: `traceparent` / `tracestate` (contexto de traza W3C) y `x-correlation-id`.
- **Siempre** con `comun/mensajeria` (`mensajeria.NuevoSobre`, `Publicador.Publicar`, `mensajeria.Consumir`).
- `agregado_version` es la versión de la **solicitud**: Operaciones la incrementa en **cada** cambio de la solicitud o de su traslado.

### 5.2 Topología
| Elemento | Nombre | Tipo / notas |
| --- | --- | --- |
| Exchange de eventos de Operaciones | `rumbo.operaciones` | `topic`, durable. Routing key = `tipo` |
| Exchange de eventos de Recursos | `rumbo.recursos` | `topic`, durable |
| Cola de estados para Integración | `integracion.estados` | Bindings: `operaciones.solicitud.*`, `operaciones.traslado.*` y `operaciones.cancelacion.*`. Consumen todas las réplicas de Integración (compiten) |
| Reintento | `<cola>.reintento` | La crea `comun/mensajeria`: TTL por mensaje y vuelta a `<cola>` |
| Mensajes fallidos | `<cola>.dlq` | La crea `comun/mensajeria`. Dueño: el equipo del servicio consumidor |

Política de fallos (ADR-004, en `comun/mensajeria`): error transitorio → hasta **3 reintentos** con backoff exponencial y jitter (~1 s, ~4 s, ~16 s); error permanente (`mensajeria.Permanente(err)`) o reintentos agotados → `<cola>.dlq`. **ACK solo después del efecto.** Prefetch: **10**.

### 5.3 (Eliminado) Comando `integracion.solicitud_carga.recibida`
Desde [ADR-021](adr/ADR-021-capacidad-de-autoservicio.md) la reserva va por HTTP (`POST /internas/reservas`, §4.1) y no existe el comando ni la cola `operaciones.solicitudes-externas`. Integración ya no publica mensajes.

### 5.4 Eventos de solicitudes y traslados (`rumbo.operaciones`)
Todos llevan en `datos` la **vista completa de la solicitud** (§6) más `organizacion_id` y `origen` (`"externo"` o `"interno"`). Así, un consumidor solo guarda la vista si `agregado_version` es mayor que la que tiene.

| `tipo` | Cuándo |
| --- | --- |
| `operaciones.solicitud.recibida` (no se publica desde ADR-024) | Se registró una solicitud (externa o interna) |
| `operaciones.solicitud.aceptada` | El operador la aceptó, el cliente aceptó la contrapropuesta o **un sistema externo reservó una franja** (versión 1, ya con el traslado). Se crea el traslado |
| `operaciones.solicitud.contrapropuesta` (no se publica desde ADR-024) | El operador propuso un horario |
| `operaciones.solicitud.rechazada` (no se publica desde ADR-024) | El operador la rechazó |
| `operaciones.solicitud.rechazada_por_cliente` (no se publica desde ADR-024) | El cliente rechazó la contrapropuesta |
| `operaciones.solicitud.vencida` (no se publica desde ADR-024) | La contrapropuesta venció sin respuesta (estado `RECHAZADA_POR_CLIENTE`) |
| `operaciones.solicitud.cancelada` (no se publica desde ADR-024) | Se canceló antes de ser aceptada |
| `operaciones.cancelacion.registrada` | Se pidió cancelar un traslado aceptado (aprobada directamente o `PENDIENTE_APROBACION`) |
| `operaciones.traslado.estado_cambiado` | Cambió el estado del traslado (incluida una cancelación aprobada) o su demora |
| `operaciones.traslado.alerta_climatica` | La regla de clima (D9) marcó o actualizó una alerta |

```json
"datos": {
  "organizacion_id": "org-…",
  "origen": "externo",
  "solicitud": { "...vista de §6..." }
}
```

## 6. Vista de solicitud (respuestas internas y eventos)
Es la misma forma que `SolicitudTraslado` del contrato público (`docs/contracts/traslados.yaml`), más campos internos que el contrato público **ignora**:
```json
{
  "id": "sol-…",
  "referencia_externa": "TRF-2026-00123",
  "estado": "ACEPTADA",
  "cosa": { "descripcion": "Cajas", "cantidad": 20, "peso_kg": 350 },
  "origen": { "nombre": "…", "direccion": "…" },
  "destino": { "nombre": "…", "direccion": "…" },
  "hora_llegada_solicitada": "2026-10-16T14:00:00-03:00",
  "tolerancia_minutos": 120,
  "observaciones": null,
  "contrapropuesta": { "hora_llegada_propuesta": "…", "vence_en": "…" },
  "motivo_rechazo": null,
  "traslado": {
    "id": "tra-…", "estado": "PROGRAMADO", "hora_llegada_comprometida": "…",
    "demora_minutos": null, "finalizado_en": null, "detalle_resultado": null,
    "franja": { "inicio": "2026-11-20T10:00:00-03:00", "fin": "2026-11-20T12:00:00-03:00" },
    "alerta_climatica": { "estado": "NO_OPERABLE", "motivos": ["HIGH_WIND"], "observado_en": "…" }
  },
  "cancelacion": { "estado": "PENDIENTE_APROBACION", "motivo": "…", "solicitada_en": "…", "tardia": true },
  "version": 4,
  "creado_en": "…",
  "actualizado_en": "…"
}
```
- Los campos ausentes van como `null` u omitidos.
- `traslado.alerta_climatica.estado` puede ser `NO_OPERABLE` (con `motivos`) o `NO_VERIFICADO` (el servicio de clima no respondió).
- Al aceptar, el traslado se crea en estado **`PROGRAMADO`** con la hora comprometida (la asignación a un recorrido es posterior).
- `traslado.franja` existe solo en las reservas externas; ahí `hora_llegada_comprometida` es el fin de la franja.
- `cancelacion.tardia` es `true` si se pidió con menos de 24 h. En una reserva externa la cancelación siempre queda `APROBADA`; `PENDIENTE_APROBACION` solo existe para las solicitudes de la web.

## 7. Logs (todos los servicios)
`log/slog` en **JSON** a stdout, configurado por `observabilidad.Iniciar`. Si está `LOGS_MYSQL_DSN`, los registros INFO, WARN y ERROR se guardan además en la tabla `logs` del MySQL de observabilidad, en lotes y en segundo plano. Grafana los lee de ahí ([ADR-023](adr/ADR-023-logs-en-mysql.md)). En la tabla, `correlation_id` y `trace_id` son columnas y el resto de los campos va en `atributos` (JSON).

| Campo | Siempre | Contenido |
| --- | --- | --- |
| `time`, `level`, `msg` | sí | Estándar de slog. Mensajes en español |
| `servicio` | sí | `gateway`, `identidad`, `operaciones`, `recursos`, `notificaciones`, `integracion` |
| `instancia` | sí | Variable `INSTANCIA` o hostname del contenedor |
| `correlation_id` | si hay request o mensaje | Tomado del contexto |
| `trace_id`, `span_id` | si hay traza | Tomados del contexto |
| Específicos | según el caso | `solicitud_id`, `tipo_mensaje`, `cola`, `ruta`, `estado_http`, `duracion_ms`, `error` |

Usar siempre `slog.InfoContext(ctx, …)` / `ErrorContext` (con el contexto, para que salgan `trace_id` y `correlation_id`). **Nunca** loguear tokens, contraseñas, secretos ni el cuerpo completo de un pedido.

## 8. Métricas (nombres fijos)
Todos los servicios Gin registran automáticamente, con `observabilidad.Instrumentar`, **`rumbo_http_duracion_segundos`** (histograma con `metodo`, `ruta`, `estado` e `instancia`): de ahí salen la tasa de requests, los errores y la latencia de cada servicio (RED). Las métricas propias:

| Métrica | Tipo | Atributos | Quién |
| --- | --- | --- | --- |
| `rumbo_mensajes_procesados_total` | contador | `cola`, `resultado` (`ok`, `reintento`, `dlq`) | `comun/mensajeria` |
| `rumbo_mensajes_publicados_total` | contador | `exchange`, `tipo`, `resultado` | `comun/mensajeria` |
| `rumbo_outbox_pendientes` | gauge | `servicio` | Operaciones (Integración ya no tiene pendientes de envío, ADR-021) |
| `rumbo_cache_operaciones_total` | contador | `cache`, `resultado` (`hit`, `miss`, `error`) | Integración (Memcached), Operaciones (clima) |
| `rumbo_circuito_estado` | gauge (0 cerrado, 1 semiabierto, 2 abierto) | `dependencia` | Integración, Operaciones |
| `rumbo_logs_descartados_total` | contador | `motivo` (`cola_llena`, `mysql`) | Todos (`comun/observabilidad`, ADR-023) |
| `rumbo_clima_consultas_total` | contador | `resultado` (`operable`, `no_operable`, `no_verificado`), `momento` (`reserva`, `inicio`; §13) | Operaciones |
| `rumbo_solicitudes_total` | contador | `origen`, `estado` | Operaciones |
| `rumbo_traslados_estado_total` | contador | `estado` | Operaciones (una vez por transición aplicada, §12) |
| `rumbo_indexacion_demora_segundos` | histograma | — | Operaciones (indexador de Solr, §16) |

## 9. Trazas
OpenTelemetry (`go.opentelemetry.io/otel`), exportadas por OTLP a `OTEL_EXPORTER_OTLP_ENDPOINT` (en Docker, `http://lgtm:4318`). Propagación **W3C Trace Context**:
- **HTTP:** automática con `observabilidad.MiddlewareGin` (entrada) y `observabilidad.ClienteHTTP` (salida).
- **RabbitMQ:** automática con `comun/mensajeria`: inyecta en los headers del mensaje al publicar y extrae al consumir.
- **Outbox:** como el relay publica después, en otro contexto, la traza se cortaría. Por eso, al guardar una fila de outbox, **se guarda también el contexto de la traza** (`propagation.MapCarrier` de OpenTelemetry: `traceparent` y `tracestate`). El relay lo extrae antes de `Publicar`, y así la publicación queda dentro de la misma traza.

Nombres de spans: los HTTP los pone la instrumentación (`POST /traslados`). Los de mensajería: `publicar <tipo>` y `consumir <tipo>`. El **flujo principal** que tiene que verse en una sola traza es:
**gateway → Integración (`POST /traslados`) → Operaciones (`POST /internas/reservas`) → `publicar operaciones.solicitud.aceptada` (relay del outbox) → `consumir …` en Integración** (ADR-021).

## 10. Variables de entorno comunes
| Variable | Uso |
| --- | --- |
| `PUERTO` | Puerto HTTP del servicio |
| `INSTANCIA` | Nombre de la réplica (si no está, el hostname) |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | Destino de trazas y métricas (vacío = no exporta, por ejemplo en tests) |
| `LOGS_MYSQL_DSN` | MySQL de logs (`rumbo_logs:<clave>@tcp(mysql-logs:3306)/observabilidad`). Vacío = los logs solo salen por stdout |
| `RABBITMQ_URI` | Broker |
| `TOKEN_SECRETO` | Firma de tokens (Identidad y gateway) |
| `SECRETO_INTERNO` | Secreto entre el gateway y los servicios (§11.2) |

## 11. Convenciones para la Presentación
Implementa [ADR-024](adr/ADR-024-alcance-de-la-presentacion.md) (alcance) y [ADR-026](adr/ADR-026-despliegue-en-render.md) (secreto interno y salud).

### 11.1 API de la aplicación web
- La web habla **solo con el gateway**, en `/api/<servicio>/...`. El gateway quita el prefijo `/api/<servicio>` antes de reenviar (ej. `GET /api/operaciones/franjas` llega a Operaciones como `GET /franjas`).
- Identidad del que llama: headers `X-Rumbo-Usuario`, `X-Rumbo-Organizacion` y `X-Rumbo-Roles` (§2). Cada servicio **autoriza** con `identidad.RequiereRol(...)`.
- **Rol `cliente`:** es un usuario de una organización que reserva por la API pública (por ejemplo, el grupo de almacenamiento). Ve y cancela **solo** lo de su organización: el servicio filtra **siempre** por `X-Rumbo-Organizacion`, nunca por un parámetro. Si pide algo de otra organización, `404`.
- **Roles `operador` y `administrador`:** ven todas las organizaciones. El administrador puede todo lo del operador, más reindexar la búsqueda.
- No hay rol `chofer` en esta etapa (ADR-024).
- Fechas y horas: entrada RFC 3339 con zona; salida RFC 3339 con `-03:00`. Fechas sueltas: `AAAA-MM-DD`, en hora de Argentina.
- Listados: `pagina` (desde 1) y `tamanio` (por defecto 20, máximo 100). Respuesta `{"resultados": [...], "pagina", "tamanio", "total"}`.

### 11.2 Secreto interno (ADR-026)
En Render los servicios tienen URL pública, así que no alcanza con "solo se llega a ellos a través del gateway" (§2).
- Header **`X-Rumbo-Interno: <SECRETO_INTERNO>`**. El gateway lo **agrega** a todo lo que reenvía y **borra** el que mande el cliente.
- Cada servicio lo exige con **`identidad.ExigirSecretoInterno()`** (middleware global de `comun/identidad`), en todas sus rutas salvo `GET /salud`. Si falta o no coincide: `401 NO_AUTENTICADO`, sin decir por qué. Compara en tiempo constante (`crypto/subtle`).
- Si `SECRETO_INTERNO` está vacío (tests unitarios), el middleware no exige nada y el servicio loguea `WARN "secreto interno deshabilitado"` al arrancar.
- Las llamadas **entre servicios de Rumbo** usan **`observabilidad.ClienteInterno(timeout)`**, que es `ClienteHTTP` más el header. Las llamadas a **otro grupo** (clima) siguen con `observabilidad.ClienteHTTP`, que **nunca** manda el secreto.

### 11.3 Salud
- Todo servicio Go expone **`GET /salud`** → `200 {"estado": "ok", "servicio": "<nombre>"}`, registrado con `observabilidad.Salud(router, servicio)`.
- Responde "el proceso está vivo", **no** verifica dependencias (teórico 6: vivo ≠ listo; una sonda que mira MySQL o RabbitMQ sacaría de servicio al proceso por una falla ajena).
- No pasa por el middleware de métricas ni por el secreto interno (como el `/health` de la clase 8, para no contar las sondas).
- Lo usan los healthchecks de `compose.yaml` y el gateway para despertar la cadena en Render (§11.4).

### 11.4 Gateway
- **CORS** para la web, con `github.com/rs/cors`: origen `WEB_ORIGEN` (por defecto `http://localhost:5173`); métodos `GET`, `POST`, `PUT`, `PATCH` y `DELETE`; headers `Authorization`, `Content-Type`, `Idempotency-Key` y `Last-Event-ID`; expone `X-Correlation-ID`. Va **antes** de la autenticación (el preflight no lleva token).
- **Stream de avisos:** `GET /api/notificaciones/avisos/stream` queda **fuera** del deadline de 5 s y sale sin buffer. Autenticación y límite de tráfico se aplican igual al abrirlo. El resto de las rutas conserva el deadline.
- **`GET /salud`** del gateway, público.
- **Despertar la cadena (ADR-026):** si `DESPERTAR_AL_ARRANCAR=true` (en Render; en local no hace falta porque Compose espera los healthchecks), al arrancar y en segundo plano el gateway hace `GET /salud` a Identidad, Operaciones e Integración (el camino de la capacidad), con reintentos cada 5 s durante hasta 120 s. Loguea `INFO "servicio despierto"` por cada uno (o `WARN "servicio no respondió al despertar"`). No bloquea el arranque.

## 12. Operaciones — API de la aplicación web
Reemplaza a la API del operador de §4.2 (ADR-024). En Operaciones; por el gateway, con el prefijo `/api/operaciones`.

| Método y ruta (en Operaciones) | Roles | Cuerpo / parámetros | Respuesta |
| --- | --- | --- | --- |
| `GET /franjas?fecha=AAAA-MM-DD` | `operador`, `administrador` | `fecha` obligatoria, entre hoy y hoy + `FRANJA_HORIZONTE_DIAS` (si no, `422 DATOS_INVALIDOS`). No aplica la anticipación: el operador ve también las franjas de hoy que ya empezaron | `200 {"fecha", "franjas": [{"inicio", "fin", "cupo_traslados", "traslados_reservados", "cupo_kg", "kg_reservados"}]}`, todas las franjas del calendario de ese día, en orden. Una franja sin reservas sale con `0` reservados |
| `GET /solicitudes?fecha=&estado_traslado=&organizacion_id=&pagina=&tamanio=` | `operador`, `administrador`, `cliente` | Todos opcionales. `fecha`: día de inicio de la franja. `estado_traslado`: `PROGRAMADO`, `EN_CURSO`, `COMPLETADO` o `CANCELADO` (otro valor, `422`). `organizacion_id`: solo para operador y administrador; **al cliente se le fuerza la suya** y el parámetro se ignora | Página de **vistas de solicitud** (§6), ordenadas por inicio de la franja y después por `creado_en` |
| `GET /solicitudes/{id}` | `operador`, `administrador`, `cliente` | — | Vista (§6). `404` si no existe o, para el cliente, si es de otra organización |
| `POST /solicitudes/{id}/estado` | `operador`, `administrador` | `{"estado": "EN_CURSO" \| "COMPLETADO"}` | `200` vista. Transiciones válidas: `PROGRAMADO` → `EN_CURSO` y `EN_CURSO` → `COMPLETADO` (este setea `traslado.finalizado_en`). Si el traslado **ya está** en el estado pedido: `200` con la vista y sin evento (reintento seguro). Cualquier otra: `409 ESTADO_NO_PERMITE_OPERACION`. Otro valor: `422`. Al pasar a `EN_CURSO` aplica la regla de clima (§13.3). Evento `operaciones.traslado.estado_cambiado` (y `operaciones.traslado.alerta_climatica` si la regla dejó una alerta) |
| `POST /solicitudes/{id}/cancelacion` | `cliente` (de su organización), `operador`, `administrador` | `Idempotency-Key` obligatorio (si falta, `422`); `{"motivo": "texto"}` | `200` vista con el traslado `CANCELADO` y `cancelacion` (`APROBADA`, `tardia` si faltan menos de 24 h). Es **el mismo caso de uso** que `POST /internas/solicitudes/{id}/cancelacion` (§4.1): se aplica en el momento y devuelve el lugar a la franja en la misma transacción (R-45). La clave se guarda con la organización **de la solicitud** y el endpoint `cancelacion`. `404` · `409 ESTADO_NO_PERMITE_OPERACION` (traslado `EN_CURSO`, `COMPLETADO` o ya `CANCELADO`) · `409 CLAVE_IDEMPOTENCIA_REUTILIZADA` · `422`. Evento `operaciones.cancelacion.registrada` |

- **Se eliminan** (ADR-024): `POST /solicitudes/{id}/aceptacion`, `/contrapropuesta`, `/rechazo` y `/cancelacion-tardia`, y la tarea periódica de vencimiento de contrapropuestas. Los eventos `operaciones.solicitud.recibida`, `.contrapropuesta`, `.rechazada`, `.rechazada_por_cliente`, `.vencida` y `.cancelada` ya no se publican.
- La **vista de solicitud** (§6) no cambia de forma. Los campos de la revisión (`hora_llegada_solicitada`, `contrapropuesta`, `motivo_rechazo`) quedan siempre en `null`.
- **Estados del traslado que se producen** (ADR-024): `PROGRAMADO`, `EN_CURSO`, `COMPLETADO` y `CANCELADO`. `ASIGNADO`, `NO_REALIZADO` e `INTERRUMPIDO` siguen en el contrato público, pero no se producen en esta versión.
- **Métrica nueva:** `rumbo_traslados_estado_total{estado}` (contador, una vez por transición aplicada).

## 13. Operaciones — clima en las reservas (ADR-025)
Reemplaza a la consulta "al aceptar" de [ADR-012](adr/ADR-012-consumo-clima.md): las reservas no pasan por un operador que acepte.

### 13.1 Qué extremo es el aeropuerto
- Al reservar (en la misma transacción, sin llamadas externas), Operaciones marca `traslado.aeropuerto_extremo`:
  - `ORIGEN` si el `nombre` o la `direccion` del origen, normalizados (minúsculas y sin tildes), contienen alguna de las palabras de **`AEROPUERTO_PALABRAS`**;
  - si no, `DESTINO` con la misma regla sobre el destino;
  - si no, `null`: ese traslado no consulta el clima.
- `AEROPUERTO_PALABRAS`: lista separada por comas, en la configuración de Operaciones. Por defecto, `aeropuerto,saco,pajas blancas`.
- Es un campo interno: no se agrega al contrato público.

### 13.2 Después de reservar (asíncrono)
- Consumidor con `mensajeria.Consumir`: cola **`operaciones.clima`**, exchange `rumbo.operaciones`, clave `operaciones.solicitud.aceptada`. Reintentos y DLQ `operaciones.clima.dlq` de `comun`.
- Para cada evento:
  1. Releer la solicitud (sin bloqueo). Si no tiene `aeropuerto_extremo`, si el traslado no está `PROGRAMADO` o si la hora comprometida no cae dentro de las próximas 6 h (`DentroDeVentanaClima`): ACK sin hacer nada.
  2. **Fuera de la transacción**, consultar el clima con el adaptador de siempre (timeout 1 s, un reintento ante error transitorio, breaker y caché de 5 min; ADR-012).
  3. En una transacción: bloquear la solicitud y, si sigue `PROGRAMADO`, registrar la alerta (`NO_OPERABLE` con sus motivos, o `NO_VERIFICADO` si el clima falló). Si el aeropuerto es operable, no se registra nada. Si ya tenía **la misma** alerta (mismo estado y mismos motivos), no se escribe ni se publica (idempotente ante un reenvío). Evento `operaciones.traslado.alerta_climatica`.
- **Fail-open:** un clima caído no es un error del mensaje (no se reintenta ni va a la DLQ): deja `NO_VERIFICADO`. Un error de MySQL sí es transitorio. Datos ilegibles → `mensajeria.Permanente` → DLQ.
- La reserva **no espera** al clima: su latencia no cambia (ADR-021).

### 13.3 Al pasar a `EN_CURSO` (síncrono)
- En `POST /solicitudes/{id}/estado` con `EN_CURSO`, si el traslado tiene `aeropuerto_extremo` y la hora comprometida cae dentro de las próximas 6 h: consultar el clima **antes** de la transacción y, dentro de ella, registrar la alerta junto con el cambio de estado (misma regla de idempotencia). Fail-open: el cambio de estado se hace igual.
- Es el "al iniciar" de R-41, que en esta etapa lo dispara el operador.

### 13.4 Métrica
`rumbo_clima_consultas_total` agrega el atributo **`momento`** (`reserva` o `inicio`).

## 14. Notificaciones (ADR-027)
Base `notificaciones` (MongoDB). Estilo en capas (ADR-010).

**Aviso:**
```json
{ "id": "<id del mensaje que lo originó>",
  "destinatario": { "tipo": "ORGANIZACION" | "ROL", "valor": "org-…" | "operador" },
  "titulo": "Reserva nueva", "mensaje": "…",
  "entidad": { "tipo": "solicitud", "id": "sol-…" },
  "creado_en": "…", "leido": false }
```
- `leido` es **compartido**: lo ve igual cualquier usuario del destinatario (el operador que lo marca lo marca para todos los operadores).

**Consumidor:** cola `notificaciones.avisos`, exchange `rumbo.operaciones`, claves `operaciones.solicitud.aceptada`, `operaciones.cancelacion.registrada`, `operaciones.traslado.estado_cambiado` y `operaciones.traslado.alerta_climatica`. Reintentos y DLQ `notificaciones.avisos.dlq` de `comun`. Idempotente: el `id` del aviso deriva del `id` del mensaje (es el `_id` en MongoDB).

| Evento | Condición | Destinatario | Título | Mensaje |
| --- | --- | --- | --- | --- |
| `solicitud.aceptada` | — (en esta etapa todas son reservas externas) | ROL `operador` | Reserva nueva | "Se reservó «{cosa.descripcion}» para la franja del {dd/mm} de {HH:MM} a {HH:MM}." |
| `cancelacion.registrada` | `cancelacion.tardia = true` | ROL `operador` | Cancelación tardía | "Se canceló «{cosa.descripcion}» con menos de 24 h: era para la franja del {dd/mm} de {HH:MM} a {HH:MM}." |
| `cancelacion.registrada` | — | ORGANIZACION (`organizacion_id`) | Traslado cancelado | "Se canceló el traslado de «{cosa.descripcion}» del {dd/mm}." |
| `traslado.alerta_climatica` | — | ROL `operador` y ORGANIZACION | Alerta climática | `NO_OPERABLE`: "El aeropuerto no está operable para «{cosa.descripcion}» ({motivos separados por coma})." · `NO_VERIFICADO`: "No se pudo verificar el clima del aeropuerto para «{cosa.descripcion}»." |
| `traslado.estado_cambiado` | `traslado.estado = EN_CURSO` | ORGANIZACION | Traslado en curso | "Salió el traslado de «{cosa.descripcion}»." |
| `traslado.estado_cambiado` | `traslado.estado = COMPLETADO` | ORGANIZACION | Traslado completado | "Llegó «{cosa.descripcion}» a {destino.nombre}." |
| `traslado.estado_cambiado` | otro estado | — | (sin aviso) | |

- Fechas y horas del mensaje: `dd/mm` y `HH:MM` en hora de Argentina, de `traslado.franja`.
- Si un evento genera varios avisos, el `id` de cada uno es `<id del mensaje>-<n>`, con `n` desde 1 en el orden de la tabla. Si genera uno solo, el `id` es el del mensaje.
- Un evento sin regla → ACK sin aviso. Datos ilegibles → `mensajeria.Permanente` → DLQ.

**API** (cualquier usuario autenticado con rol `operador`, `administrador` o `cliente`). Ve los avisos con destinatario `ROL` igual a alguno de sus roles (el `administrador` ve además los de `operador`) o `ORGANIZACION` igual a su organización:

| Método y ruta | Respuesta |
| --- | --- |
| `GET /avisos?no_leidos=true&pagina=&tamanio=` | Página de avisos, más nuevos primero |
| `POST /avisos/{id}/lectura` | `204`. `404` si no existe o no es para el que llama |
| `GET /avisos/stream` | **SSE** (`text/event-stream`): `event: aviso`, `id: <id>`, `data: <aviso JSON en una línea>`. Un comentario `: ping` cada 15 s. Con `Last-Event-ID`, primero reenvía los avisos posteriores a ese id; si el id no existe, los del último minuto; sin el header, ninguno viejo |

**Cómo lo consume la web:** con `fetch` y el header `Authorization`, leyendo el cuerpo como stream. `EventSource` no permite mandar `Authorization` (teórico 9), y no se pone el token en la URL. Si se corta, reconecta con `Last-Event-ID`.

## 15. Identidad (perfil y usuarios de demostración)
**Modelo:** `Usuario` gana `nombre`. `Organizacion` tiene `nombre`.

| Método y ruta (en Identidad) | Roles | Respuesta |
| --- | --- | --- |
| `GET /yo` | `operador`, `administrador`, `cliente` | `200 {"id", "email", "nombre", "roles", "organizacion": {"id", "nombre"} \| null}` (`null` para el personal de Rumbo). Con un token del sistema externo (no es un usuario): `404` |

**Siembra al arrancar** (idempotente, como la del administrador que ya existe; nunca pisa la contraseña de un usuario que ya existe):

| Qué | De dónde |
| --- | --- |
| Organización `org-almacenamiento` (la del sistema externo) con su `nombre` | `EXTERNO_ORGANIZACION_NOMBRE` (por defecto `Gestión de almacenamiento`) |
| Administrador, nombre `Administrador` | `ADMIN_EMAIL`, `ADMIN_PASSWORD` (ya existe; se le completa el nombre si falta) |
| Operador, nombre `Operador de Rumbo`, rol `operador` | `OPERADOR_EMAIL`, `OPERADOR_PASSWORD` |
| Cliente, nombre `Usuario de almacenamiento`, rol `cliente`, organización `org-almacenamiento` | `CLIENTE_EMAIL`, `CLIENTE_PASSWORD` |

Si `OPERADOR_EMAIL` o `CLIENTE_EMAIL` están vacíos, ese usuario no se siembra (en la nube no hacen falta, porque la web corre en local).

## 16. Búsqueda (ADR-011, con el alcance de ADR-024)
En Operaciones; por el gateway, con el prefijo `/api/operaciones`.

- **`GET /traslados/busqueda?q=&organizacion_id=&estado=&desde=&hasta=&alerta=&orden=fecha|relevancia&pagina=&tamanio=`**
  - roles `operador`, `administrador` y `cliente` (**al cliente se le fuerza su organización**, con un `fq` tomado de la identidad del gateway);
  - `q`: texto libre sobre la descripción de la cosa, los nombres y direcciones de origen y destino y la referencia externa;
  - `estado`: estado del traslado; `desde` y `hasta`: `AAAA-MM-DD` sobre el inicio de la franja; `alerta=true`: solo los que tienen alerta climática;
  - `orden=fecha` (por defecto): por inicio de la franja, ascendente. `orden=relevancia`: por puntaje; sin `q`, se ordena por fecha;
  - responde una página de `{"solicitud_id", "traslado_id", "organizacion_id", "referencia_externa", "descripcion", "origen_nombre", "destino_nombre", "estado_traslado", "franja_inicio", "franja_fin", "alerta_climatica": "NO_OPERABLE" | "NO_VERIFICADO" | null, "cancelacion_tardia": bool}`;
  - `503 SERVICIO_NO_DISPONIBLE` si Solr no responde.
- **`POST /admin/reindexacion`** (`administrador`) → `202 {"estado": "en_curso"}`. Recorre MySQL y vuelve a indexar todo en segundo plano; al terminar loguea `INFO "reindexación terminada"` con la cantidad.
- **Indexador:** cola `operaciones.indexador`, claves `operaciones.solicitud.*`, `operaciones.traslado.*` y `operaciones.cancelacion.*`. Upsert solo si la `agregado_version` del evento es mayor que la indexada. Métrica `rumbo_indexacion_demora_segundos` (histograma).
- Los filtros de ADR-011 sin datos en esta etapa (`tipo_servicio`, `chofer_id` y `vehiculo_id`) no se implementan (ADR-024).
