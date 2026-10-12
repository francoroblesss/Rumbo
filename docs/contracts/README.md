# Capacidad publicada: Solicitar traslado

Un sistema externo le pide a Rumbo **trasladar una cosa desde un origen hasta un destino**. Rumbo registra la solicitud, un operador la revisa (la acepta, propone un horario o la rechaza), la planifica, la ejecuta e informa el resultado. El sistema externo **inicia una operación real** dentro de Rumbo y sigue su avance.

Solo hay **tres datos obligatorios: `cosa`, `origen` y `destino`**. La capacidad es genérica a propósito, para que cualquier sistema (por ejemplo, uno de gestión de almacenamiento) la pueda usar sin adaptarse a un modelo cerrado.

## Versión vigente
| Versión | Contrato | Base URL | Estado |
| --- | --- | --- | --- |
| **2.0.0** | [`traslados-v2.yaml`](traslados-v2.yaml) (OpenAPI 3.1) | `/v2` · mock local: `http://localhost:4010` · producción: _a publicar_ | **Vigente** |
| 1.0.0 | [`solicitudes-carga-v1.yaml`](solicitudes-carga-v1.yaml) | `/v1` | Reemplazada por 2.0.0; nunca tuvo consumidores ni se implementó |

Política de versiones: ver "Compatibilidad" más abajo, [ADR-002](../adr/ADR-002-contrato-propio.md) y [ADR-006](../adr/ADR-006-contrato-generico-v2.md).

## Flujo de uso
```
1. POST /v2/auth/token                      → token (Bearer, 1 h)
2. POST /v2/traslados  (Idempotency-Key)    → 202, estado RECIBIDA
3. Consultar cada ≥ 60 s:
     GET /v2/traslados?actualizado_desde=<último actualizado_en procesado>
     (o GET /v2/traslados/{id})
4. Si estado = CONTRAPROPUESTA → POST /{id}/respuesta-contrapropuesta {acepta} antes de contrapropuesta.vence_en
5. Si estado = ACEPTADA → seguir traslado.estado hasta un estado final
6. Para cancelar → POST /{id}/cancelacion {motivo}
```

## Operaciones
| Operación | Endpoint | Resultado |
| --- | --- | --- |
| Obtener token | `POST /v2/auth/token` | `200` con `access_token`, `token_type`, `expira_en_segundos` |
| Solicitar un traslado | `POST /v2/traslados` | `202` con la solicitud `RECIBIDA` y header `Location` |
| Consultar una solicitud | `GET /v2/traslados/{id}` | `200` con la solicitud y, si fue aceptada, su `traslado` |
| Listar / seguir cambios | `GET /v2/traslados?actualizado_desde=&estado=&pagina=&tamanio=` | `200` paginado, ordenado por `actualizado_en` ascendente |
| Responder contrapropuesta | `POST /v2/traslados/{id}/respuesta-contrapropuesta` | `200` con la solicitud `ACEPTADA` o `RECHAZADA_POR_CLIENTE` |
| Cancelar | `POST /v2/traslados/{id}/cancelacion` | `200` con la solicitud o el traslado cancelado, o la cancelación pendiente de aprobación |

## Datos a enviar
| Campo | Obligatorio | Descripción |
| --- | --- | --- |
| `cosa.descripcion` | **sí** | Qué se traslada (ej. "cajas de mercadería seca") |
| `cosa.cantidad` | **sí** | Cantidad de unidades o bultos (≥ 1) |
| `origen` | **sí** | `nombre` y `direccion` (obligatorios), `contacto` (opcional) |
| `destino` | **sí** | Igual que `origen` |
| `cosa.peso_kg`, `cosa.volumen_m3` | no | Peso y volumen totales. Si se informa el peso, Rumbo lo usa para elegir un vehículo con capacidad suficiente |
| `hora_llegada_solicitada` | no | Hora a la que la cosa debe **llegar** al destino (ISO 8601 con zona horaria, ej. `-03:00`). **Si no se envía, Rumbo propone un horario** (`CONTRAPROPUESTA`) |
| `tolerancia_minutos` | no (0) | Flexibilidad aceptada alrededor de la hora pedida (0 a 720). 0 = hora estricta |
| `referencia_externa` | no | Identificador de la operación en el sistema del consumidor; Rumbo lo devuelve tal cual |
| `observaciones` | no | Indicaciones para el retiro o la entrega |

## Estados
**Solicitud (`estado`)**
| Estado | Significado | ¿Final? |
| --- | --- | --- |
| `RECIBIDA` | Registrada, pendiente de revisión | no |
| `CONTRAPROPUESTA` | Rumbo propone un horario (`contrapropuesta.hora_llegada_propuesta`): otro distinto del pedido, o uno nuevo si no se pidió hora. Responder antes de `vence_en` (24 h) | no |
| `ACEPTADA` | El horario es un compromiso; existe `traslado` | no (seguir `traslado.estado`) |
| `RECHAZADA` | Rumbo no puede atenderla (`motivo_rechazo`) | sí |
| `RECHAZADA_POR_CLIENTE` | El consumidor rechazó la contrapropuesta o no respondió en 24 h | sí |
| `CANCELADA` | El consumidor la canceló antes de ser aceptada | sí |

**Traslado (`traslado.estado`, solo si la solicitud está `ACEPTADA`)**
`PROGRAMADO` → `ASIGNADO` → `EN_CURSO` → `COMPLETADO`. Finales alternativos: `CANCELADO`, `NO_REALIZADO` (no se pudo retirar la cosa) e `INTERRUMPIDO` (empezó y no terminó). Un retraso se informa en `traslado.demora_minutos`; no es un estado.

**Rumbo nunca fija ni cambia el horario sin el acuerdo del consumidor**: si no puede cumplir el pedido, o si no se pidió hora, propone uno (`CONTRAPROPUESTA`) o rechaza.

## Cancelación
| Momento | Resultado |
| --- | --- |
| Antes de que la solicitud sea aceptada | Solicitud `CANCELADA` |
| Aceptada, con **24 h o más** antes de la hora comprometida | Traslado `CANCELADO` |
| Aceptada, con **menos de 24 h** | `cancelacion.estado = PENDIENTE_APROBACION`; un operador la aprueba (`APROBADA`, traslado `CANCELADO`) o no (`RECHAZADA`) |
| Traslado `EN_CURSO` o finalizado | `409 ESTADO_NO_PERMITE_OPERACION` |

La versión 2 no permite modificar una solicitud: para cambiar datos u horario, cancelar y crear una nueva.

## Errores
Formato **Problem Details** (RFC 9457, `application/problem+json`) con un `codigo` estable. El consumidor decide según `codigo`, no según el texto.

| HTTP | `codigo` | Qué significa | Qué hacer |
| --- | --- | --- | --- |
| 400 | `JSON_MAL_FORMADO` | El cuerpo no es JSON válido o un parámetro tiene formato inválido | Corregir; no reintentar igual |
| 401 | `NO_AUTENTICADO` | Falta el token, está vencido o es inválido | Pedir un token nuevo |
| 403 | `SIN_PERMISO` | La organización no está habilitada para esta capacidad | Contactar a Rumbo |
| 404 | `NO_ENCONTRADA` | La solicitud no existe o es de otra organización | Verificar el `id` |
| 409 | `CLAVE_IDEMPOTENCIA_REUTILIZADA` | La `Idempotency-Key` ya se usó con otro cuerpo | Usar una clave nueva para una intención nueva |
| 409 | `ESTADO_NO_PERMITE_OPERACION` / `CONTRAPROPUESTA_VENCIDA` | El estado actual no admite la operación | Consultar la solicitud; no reintentar |
| 422 | `DATOS_INVALIDOS` | Falta un dato obligatorio o un valor no cumple el contrato (detalle en `errores`) | Corregir el pedido |
| 422 | `HORARIO_EN_EL_PASADO` / `ANTICIPACION_INSUFICIENTE` / `EXCEDE_CAPACIDAD` | Regla de negocio | Cambiar el pedido |
| 429 | `DEMASIADOS_PEDIDOS` | Se superó el límite de la organización | Esperar `Retry-After` segundos |
| 503 | `SERVICIO_NO_DISPONIBLE` | Rumbo no puede procesar ahora | Reintentar con backoff y la **misma** `Idempotency-Key` |

## Autenticación, idempotencia y condiciones de uso
- **Autenticación:** `POST /v2/auth/token` con `client_id` y `client_secret` (los entrega el administrador de Rumbo). Devuelve un token firmado, válido 1 h, que se envía como `Authorization: Bearer <token>`. Cada organización ve solo sus solicitudes.
- **Idempotencia:** todo `POST` sobre traslados exige `Idempotency-Key` (se recomienda un UUID por intención). Rumbo conserva la clave 24 h con el cuerpo y el resultado: reintentar con la misma clave y el mismo cuerpo devuelve el mismo resultado sin duplicar; la misma clave con otro cuerpo devuelve `409`. Ante un timeout o un `503`, **reintentar con la misma clave**.
- **Límite de uso:** 60 pedidos por minuto por organización; si se excede, `429` con `Retry-After`.
- **Polling:** consultar como máximo una vez por minuto, usando `actualizado_desde` para traer solo lo que cambió.
- **Fechas:** ISO 8601 con zona horaria. Rumbo opera en hora de Argentina (`-03:00`).
- **Campos desconocidos:** el consumidor debe ignorar los campos que no conozca (ver Compatibilidad).

## Compatibilidad
- **Cambios compatibles** (versión menor, misma `/v2`): agregar campos opcionales a respuestas o pedidos, nuevos endpoints, nuevos `codigo` de error dentro de un HTTP ya documentado.
- **Cambios incompatibles** (nueva versión mayor, `/v3`, conviviendo con `/v2` hasta finalizar la evaluación): quitar o renombrar campos, cambiar el significado de un campo o de un estado, **agregar estados** a `estado` o `traslado.estado`, volver obligatorio un campo opcional.
- Cada cambio se registra en el historial de esta página y se avisa al grupo consumidor.

## Ejemplos
**Obtener token**
```bash
curl -X POST http://localhost:4010/v2/auth/token \
  -H 'Content-Type: application/json' \
  -d '{"client_id":"org-externa","client_secret":"s3cr3t0-de-ejemplo"}'
```

**Solicitar un traslado con solo lo obligatorio** (Rumbo propondrá el horario)
```bash
curl -i -X POST http://localhost:4010/v2/traslados \
  -H 'Authorization: Bearer <token>' \
  -H 'Idempotency-Key: 3f6c1e2a-8b4d-4c1e-9a7f-2d5b6e8c9a01' \
  -H 'Content-Type: application/json' \
  -d '{
    "cosa":    {"descripcion": "Cajas de mercadería seca", "cantidad": 20},
    "origen":  {"nombre": "Sucursal Centro", "direccion": "Av. Colón 1234, Córdoba"},
    "destino": {"nombre": "Sucursal Cerro",  "direccion": "Av. Rafael Núñez 4500, Córdoba"}
  }'
```

**Solicitar un traslado con hora y datos opcionales**
```bash
curl -i -X POST http://localhost:4010/v2/traslados \
  -H 'Authorization: Bearer <token>' \
  -H 'Idempotency-Key: 7b1d2c3e-1111-4c1e-9a7f-2d5b6e8c9a02' \
  -H 'Content-Type: application/json' \
  -d '{
    "cosa":    {"descripcion": "Cajas de mercadería seca", "cantidad": 20, "peso_kg": 350},
    "origen":  {"nombre": "Sucursal Centro", "direccion": "Av. Colón 1234, Córdoba"},
    "destino": {"nombre": "Sucursal Cerro",  "direccion": "Av. Rafael Núñez 4500, Córdoba"},
    "hora_llegada_solicitada": "2026-10-16T14:00:00-03:00",
    "tolerancia_minutos": 120,
    "referencia_externa": "TRF-2026-00123"
  }'
```

**Seguir los cambios**
```bash
curl 'http://localhost:4010/v2/traslados?actualizado_desde=2026-10-15T09:00:00-03:00' \
  -H 'Authorization: Bearer <token>'
```

**Aceptar una contrapropuesta**
```bash
curl -X POST http://localhost:4010/v2/traslados/sol-7f3a9c/respuesta-contrapropuesta \
  -H 'Authorization: Bearer <token>' -H 'Idempotency-Key: 9a1b...' \
  -H 'Content-Type: application/json' -d '{"acepta": true}'
```

El contrato incluye ejemplos de respuesta para cada estado: recibida, contrapropuesta, aceptada, completada con demora, rechazada y cancelación pendiente.

## Mock para probar la integración
El mock se genera directamente del contrato con [Prism](https://github.com/stoplightio/prism), así que siempre coincide con él. Requiere Node.js:
```bash
npx @stoplight/prism-cli mock docs/contracts/traslados-v2.yaml -p 4010
```
- **Valida cada pedido** contra el contrato: si falta un dato obligatorio responde `422 DATOS_INVALIDOS`, y sin `Authorization` responde `401`.
- Devuelve los ejemplos del contrato. Para elegir uno, usar el header `Prefer`:
  - `Prefer: example=contrapropuesta` (también `recibida`, `completada`, `rechazada`);
  - `Prefer: code=409` para forzar un error.
- **No guarda estado**: una solicitud creada no avanza sola. Para probar el ciclo completo, usar los ejemplos de cada estado.

## Historial de versiones
| Versión | Fecha | Cambios | Compatible con la anterior |
| --- | --- | --- | --- |
| 2.0.0 | 2026-10-11 | Capacidad genérica: `cosa` (descripción + cantidad; peso y volumen opcionales) en lugar de `carga`; solo `cosa`, `origen` y `destino` obligatorios; hora y referencia opcionales (sin hora, Rumbo propone una); recurso `/v2/traslados`; código `EXCEDE_CAPACIDAD` (antes `CARGA_EXCEDE_CAPACIDAD`). Corrección de la Entrega 1 (ADR-006) | **No** (por eso es versión mayor) |
| 1.0.0 | 2026-10-04 | Versión inicial, específica de carga (`/v1/solicitudes-carga`) | — |
