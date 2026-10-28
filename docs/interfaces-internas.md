# Interfaces internas entre servicios

> Contrato **interno** entre los servicios de Rumbo: mensajes, endpoints internos, identidad, errores, logs, métricas y trazas.
> El contrato **público** (para otros grupos) está en [`contracts/`](contracts/README.md).
> Implementa las decisiones de [ADR-004](adr/ADR-004-comunicacion-entre-servicios.md) (comunicación), [ADR-002](adr/ADR-002-contrato-propio.md)/[ADR-006](adr/ADR-006-contrato-generico-v2.md)/[ADR-019](adr/ADR-019-contrato-sin-version-en-la-ruta.md)/[ADR-021](adr/ADR-021-capacidad-de-autoservicio.md) (contrato público de autoservicio, rutas sin versión), [ADR-022](adr/ADR-022-consistencia-y-concurrencia.md) (consistencia y concurrencia) y el teórico 6 (identidad en el borde).
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
El gateway valida el token firmado y **reemplaza** estos headers. Si el cliente los manda, el gateway los **borra** antes de reenviar. Los servicios confían en ellos porque solo se llega a ellos a través del gateway.

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

### 4.2 Operaciones — API del operador (a través del gateway, `/api/operaciones/...`)
Requiere rol `operador` o `administrador`.

| Método y ruta (en Operaciones) | Cuerpo | Efecto |
| --- | --- | --- |
| `GET /solicitudes?estado=&pagina=&tamanio=` | — | Lista paginada (`resultados`, `pagina`, `tamanio`, `total`), más recientes primero |
| `GET /solicitudes/{id}` | — | Vista de la solicitud |
| `POST /solicitudes/{id}/aceptacion` | `{"aeropuerto_extremo": "ORIGEN" \| "DESTINO" \| null}` | Acepta con la hora pedida (`409` si no hay hora pedida: hay que contraproponer). Si se indica un extremo en el aeropuerto, aplica la regla de clima (D9) |
| `POST /solicitudes/{id}/contrapropuesta` | `{"hora_llegada_propuesta": "RFC3339"}` | `CONTRAPROPUESTA`, vence a las 24 h |
| `POST /solicitudes/{id}/rechazo` | `{"motivo": "texto"}` | `RECHAZADA` |
| `POST /solicitudes/{id}/cancelacion-tardia` | `{"aprobar": true}` | Resuelve una cancelación `PENDIENTE_APROBACION` |

### 4.3 Identidad
| Método y ruta (en Identidad) | Cuerpo | Respuesta |
| --- | --- | --- |
| `POST /auth/token` | `{"client_id","client_secret"}` | `200 {"access_token","token_type":"Bearer","expira_en_segundos":3600}` · `401 NO_AUTENTICADO` |
| `POST /login` | `{"email","password"}` | Igual que el anterior |

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
| `operaciones.solicitud.recibida` | Se registró una solicitud (externa o interna) |
| `operaciones.solicitud.aceptada` | El operador la aceptó, el cliente aceptó la contrapropuesta o **un sistema externo reservó una franja** (versión 1, ya con el traslado). Se crea el traslado |
| `operaciones.solicitud.contrapropuesta` | El operador propuso un horario |
| `operaciones.solicitud.rechazada` | El operador la rechazó |
| `operaciones.solicitud.rechazada_por_cliente` | El cliente rechazó la contrapropuesta |
| `operaciones.solicitud.vencida` | La contrapropuesta venció sin respuesta (estado `RECHAZADA_POR_CLIENTE`) |
| `operaciones.solicitud.cancelada` | Se canceló antes de ser aceptada |
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
| `rumbo_clima_consultas_total` | contador | `resultado` (`operable`, `no_operable`, `no_verificado`) | Operaciones |
| `rumbo_solicitudes_total` | contador | `origen`, `estado` | Operaciones |

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
