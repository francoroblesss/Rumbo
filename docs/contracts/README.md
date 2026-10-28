# Capacidad publicada: Reservar traslado

Un sistema externo **reserva el traslado de una cosa desde un origen hasta un destino** en una franja horaria. Rumbo responde en el momento si lo tomó o no, lo ejecuta e informa el resultado. **No hay negociación ni pasos manuales**: el sistema externo solo llama endpoints y cada respuesta es definitiva.

Para reservar hacen falta **`cosa`, `origen`, `destino` y `franja_inicio`**. La capacidad es genérica a propósito, para que cualquier sistema (por ejemplo, uno de gestión de almacenamiento) la pueda usar sin adaptarse a un modelo cerrado.

## Versión vigente
| Versión | Contrato | Base URL | Estado |
| --- | --- | --- | --- |
| **4.0.0** | [`traslados.yaml`](traslados.yaml) (OpenAPI 3.1) | Rutas sin versión (`/disponibilidad`, `/traslados`, `/auth/token`) · mock local: `http://localhost:4010` · producción: _a publicar_ | **Vigente** |
| 3.0.0 | [`traslados-v3.yaml`](traslados-v3.yaml) | Rutas sin versión | Reemplazada por 4.0.0 (ADR-021): tenía revisión del operador y contrapropuesta. Solo se usó contra el mock |
| 2.1.0 | [`traslados-v2.yaml`](traslados-v2.yaml) | `/v2` | Reemplazada por 3.0.0; nunca tuvo consumidores |
| 1.0.0 | [`solicitudes-carga-v1.yaml`](solicitudes-carga-v1.yaml) | `/v1` | Reemplazada por 2.0.0; nunca tuvo consumidores ni se implementó |

**Las rutas no llevan versión.** El contrato sí está versionado: la versión vigente es la de `info.version` de `traslados.yaml`, y cada cambio queda en el [historial](#historial-de-versiones). Política de versiones: "Compatibilidad" más abajo y [ADR-019](../adr/ADR-019-contrato-sin-version-en-la-ruta.md).

## Flujo de uso
```
1. POST /auth/token                                  → token (Bearer, 1 h)
2. GET  /disponibilidad?fecha=2026-11-20&peso_kg=350 → franjas con lugar
3. POST /traslados  (Idempotency-Key)                → 201 PROGRAMADO        (reservado: es un compromiso)
                                                     → 409 SIN_DISPONIBILIDAD (elegir otra franja, clave nueva)
4. Seguir el avance, cada ≥ 60 s:
     GET /traslados?actualizado_desde=<último actualizado_en procesado>   (o GET /traslados/{id})
5. Para cancelar (si todavía no empezó) → POST /traslados/{id}/cancelacion {motivo} → 200 CANCELADO
```

## Operaciones
| Operación | Endpoint | Resultado |
| --- | --- | --- |
| Obtener token | `POST /auth/token` | `200` con `access_token`, `token_type`, `expira_en_segundos` |
| Consultar disponibilidad | `GET /disponibilidad?fecha=&peso_kg=` | `200` con las franjas de la fecha que todavía se pueden reservar |
| Reservar un traslado | `POST /traslados` | `201` con el traslado `PROGRAMADO` y header `Location`, o `409 SIN_DISPONIBILIDAD` |
| Consultar un traslado | `GET /traslados/{id}` | `200` con el traslado |
| Listar / seguir cambios | `GET /traslados?actualizado_desde=&estado=&pagina=&tamanio=` | `200` paginado, ordenado por `actualizado_en` ascendente |
| Cancelar | `POST /traslados/{id}/cancelacion` | `200` con el traslado `CANCELADO` |

## Franjas
- Una **franja** es una ventana de **llegada** al destino. Rumbo se compromete a que la cosa llegue antes de `franja.fin`.
- Las franjas son de **2 h entre las 08:00 y las 20:00** (hora de Argentina): 08–10, 10–12, …, 18–20.
- Se reserva con **al menos 2 h de anticipación** y **hasta 30 días** adelante. `GET /disponibilidad` solo muestra las franjas que todavía cumplen eso.
- Cada franja tiene un **cupo** de traslados y de kg para todos los sistemas externos.
  - Una reserva ocupa un lugar y, si informa `peso_kg`, descuenta ese peso.
  - Una carga más pesada que el cupo total de una franja recibe `422 EXCEDE_CAPACIDAD`: hay que dividirla en varios traslados.
- **La disponibilidad orienta, la reserva decide:** entre la consulta y el `POST`, otro sistema puede tomar el último lugar. En ese caso la reserva responde `409 SIN_DISPONIBILIDAD` sin reservar nada.

## Datos a enviar (`POST /traslados`)
| Campo | Obligatorio | Descripción |
| --- | --- | --- |
| `cosa.descripcion` | **sí** | Qué se traslada (ej. "cajas de mercadería seca") |
| `cosa.cantidad` | **sí** | Cantidad de unidades o bultos (≥ 1) |
| `origen` | **sí** | `nombre` y `direccion` (obligatorios), `contacto` (opcional) |
| `destino` | **sí** | Igual que `origen` |
| `franja_inicio` | **sí** | Inicio de la franja a reservar, tal como lo devuelve `GET /disponibilidad` (ISO 8601 con zona, ej. `2026-11-20T10:00:00-03:00`) |
| `cosa.peso_kg`, `cosa.volumen_m3` | no | Peso y volumen totales. El peso descuenta del cupo de kg de la franja |
| `referencia_externa` | no | Identificador de la operación en el sistema del consumidor; Rumbo lo devuelve tal cual |
| `observaciones` | no | Indicaciones para el retiro o la entrega |

## Estados del traslado (`estado`)
| Estado | Significado | ¿Final? |
| --- | --- | --- |
| `PROGRAMADO` | Reservado: Rumbo se comprometió a la franja | no |
| `ASIGNADO` | Ya tiene chofer y vehículo | no |
| `EN_CURSO` | El chofer lo inició | no |
| `COMPLETADO` | La cosa llegó al destino | sí |
| `CANCELADO` | Cancelado antes de iniciar | sí |
| `NO_REALIZADO` | No se pudo retirar la cosa (`detalle_resultado`) | sí |
| `INTERRUMPIDO` | Empezó y no terminó, por ejemplo por una avería (`detalle_resultado`) | sí |

Un retraso se informa en `demora_minutos`; no es un estado.

## Cancelación
| Momento | Resultado |
| --- | --- |
| `PROGRAMADO` o `ASIGNADO`, con **24 h o más** antes del fin de la franja | `200`, traslado `CANCELADO`, `cancelacion.tardia = false` |
| `PROGRAMADO` o `ASIGNADO`, con **menos de 24 h** | `200`, traslado `CANCELADO`, `cancelacion.tardia = true` (se aplica igual; Rumbo lo registra para su operación) |
| `EN_CURSO` o terminado | `409 ESTADO_NO_PERMITE_OPERACION` |

Cancelar libera el lugar de la franja. El contrato no permite modificar una reserva: para cambiar datos o franja, cancelar y reservar de nuevo.

## Errores
Formato **Problem Details** (RFC 9457, `application/problem+json`) con un `codigo` estable. El consumidor decide según `codigo`, no según el texto.

| HTTP | `codigo` | Qué significa | Qué hacer |
| --- | --- | --- | --- |
| 400 | `JSON_MAL_FORMADO` | El cuerpo no es JSON válido o un parámetro tiene formato inválido (por ejemplo, `fecha`) | Corregir; no reintentar igual |
| 401 | `NO_AUTENTICADO` | Falta el token, está vencido o es inválido | Pedir un token nuevo |
| 403 | `SIN_PERMISO` | La organización no está habilitada para esta capacidad | Contactar a Rumbo |
| 404 | `NO_ENCONTRADA` | El traslado no existe o es de otra organización | Verificar el `id` |
| 409 | `SIN_DISPONIBILIDAD` | La franja ya no tiene lugar para esta carga | Consultar la disponibilidad y reservar otra franja con una clave nueva |
| 409 | `CLAVE_IDEMPOTENCIA_REUTILIZADA` | La `Idempotency-Key` ya se usó con otro cuerpo | Usar una clave nueva para una intención nueva |
| 409 | `ESTADO_NO_PERMITE_OPERACION` | El traslado ya empezó o terminó (no se puede cancelar) | Consultar el traslado; no reintentar |
| 422 | `DATOS_INVALIDOS` | Falta un dato obligatorio, un valor no cumple el contrato o `franja_inicio` no es una franja reservable (detalle en `errores`) | Corregir el pedido |
| 422 | `ANTICIPACION_INSUFICIENTE` / `HORARIO_EN_EL_PASADO` | La franja empieza en menos de 2 h o ya empezó | Elegir una franja posterior |
| 422 | `EXCEDE_CAPACIDAD` | El peso supera el cupo total de una franja | Dividir la carga en varios traslados |
| 429 | `DEMASIADOS_PEDIDOS` | Se superó el límite de la organización | Esperar `Retry-After` segundos |
| 503 | `SERVICIO_NO_DISPONIBLE` | Rumbo no puede procesar ahora | Reintentar con backoff y la **misma** `Idempotency-Key` |
| 502 | `SERVICIO_NO_DISPONIBLE` | El punto de entrada no pudo comunicarse con el servicio | Reintentar con backoff y la **misma** `Idempotency-Key` |
| 504 | `TIEMPO_AGOTADO` | Rumbo no respondió en 5 s; **la reserva pudo haberse hecho** | Reintentar con la **misma** `Idempotency-Key`: si ya se hizo, devuelve la misma reserva sin tomar otro lugar |

## Autenticación, idempotencia y condiciones de uso
- **Autenticación:**
  - `POST /auth/token` con `client_id` y `client_secret` (los entrega el administrador de Rumbo);
  - devuelve un token firmado, válido 1 h, que se envía como `Authorization: Bearer <token>`;
  - cada organización ve solo sus traslados.
- **Idempotencia:** todo `POST` sobre traslados exige `Idempotency-Key` (se recomienda un UUID por intención).
  - Rumbo guarda la clave junto con el cuerpo y el resultado, en la misma transacción que la reserva.
  - Reintentar con la misma clave y el mismo cuerpo devuelve el mismo resultado, sin tomar otro lugar, aunque mientras tanto la franja se haya llenado.
  - La misma clave con otro cuerpo devuelve `409`.
  - Ante un timeout o un `503`, **reintentar con la misma clave**.
  - Ante un `409 SIN_DISPONIBILIDAD`, la nueva reserva es otra intención y lleva **otra clave**.
- **Límite de uso:** 60 pedidos por minuto por organización; si se excede, `429` con `Retry-After`.
- **Polling:** consultar como máximo una vez por minuto, usando `actualizado_desde` para traer solo lo que cambió.
- **Fechas:** ISO 8601 con zona horaria. Rumbo opera en hora de Argentina (`-03:00`).
- **Campos desconocidos:** el consumidor debe ignorar los campos que no conozca (ver Compatibilidad).

## Compatibilidad
- **Cambios compatibles** (versión menor o de parche; las rutas no cambian):
  - agregar campos opcionales a respuestas o pedidos;
  - nuevos endpoints;
  - nuevos `codigo` de error. Para que no rompan a nadie, **el consumidor que reciba un `codigo` que no conoce decide por el HTTP status** (4xx: corregir el pedido; 5xx: reintentar con backoff y la misma `Idempotency-Key`).
- **Cambios incompatibles** (nueva versión mayor):
  - quitar o renombrar campos;
  - cambiar el significado de un campo o de un estado;
  - **agregar estados** a `estado`;
  - volver obligatorio un campo opcional.

  Como las rutas no llevan versión, se hacen con **migración coordinada** con el grupo consumidor: se avisa con anticipación, se acuerda la fecha del cambio y se registra en el historial (ADR-019). Rumbo prefiere siempre la evolución compatible.
- Cada cambio se registra en el historial de esta página y se avisa al grupo consumidor.

## Ejemplos
**Obtener token**
```bash
curl -X POST http://localhost:4010/auth/token \
  -H 'Content-Type: application/json' \
  -d '{"client_id":"org-externa","client_secret":"s3cr3t0-de-ejemplo"}'
```

**Consultar la disponibilidad de un día**
```bash
curl 'http://localhost:4010/disponibilidad?fecha=2026-11-20&peso_kg=350' \
  -H 'Authorization: Bearer <token>'
```

**Reservar con solo lo obligatorio**
```bash
curl -i -X POST http://localhost:4010/traslados \
  -H 'Authorization: Bearer <token>' \
  -H 'Idempotency-Key: 3f6c1e2a-8b4d-4c1e-9a7f-2d5b6e8c9a01' \
  -H 'Content-Type: application/json' \
  -d '{
    "cosa":          {"descripcion": "Cajas de mercadería seca", "cantidad": 20},
    "origen":        {"nombre": "Sucursal Centro", "direccion": "Av. Colón 1234, Córdoba"},
    "destino":       {"nombre": "Sucursal Cerro",  "direccion": "Av. Rafael Núñez 4500, Córdoba"},
    "franja_inicio": "2026-11-20T10:00:00-03:00"
  }'
```

**Reservar con peso y referencia**
```bash
curl -i -X POST http://localhost:4010/traslados \
  -H 'Authorization: Bearer <token>' \
  -H 'Idempotency-Key: 7b1d2c3e-1111-4c1e-9a7f-2d5b6e8c9a02' \
  -H 'Content-Type: application/json' \
  -d '{
    "cosa":               {"descripcion": "Cajas de mercadería seca", "cantidad": 20, "peso_kg": 350},
    "origen":             {"nombre": "Sucursal Centro", "direccion": "Av. Colón 1234, Córdoba"},
    "destino":            {"nombre": "Sucursal Cerro",  "direccion": "Av. Rafael Núñez 4500, Córdoba"},
    "franja_inicio":      "2026-11-20T10:00:00-03:00",
    "referencia_externa": "TRF-2026-00123"
  }'
```

**Seguir los cambios**
```bash
curl 'http://localhost:4010/traslados?actualizado_desde=2026-11-19T09:00:00-03:00' \
  -H 'Authorization: Bearer <token>'
```

**Cancelar**
```bash
curl -X POST http://localhost:4010/traslados/sol-7f3a9c/cancelacion \
  -H 'Authorization: Bearer <token>' -H 'Idempotency-Key: 9a1b...' \
  -H 'Content-Type: application/json' -d '{"motivo": "La mercadería se despachó por otro medio."}'
```

El contrato incluye ejemplos de respuesta para cada caso: programado, completado, no realizado, cancelado (tardía), franja sin lugar y los errores de regla de negocio.

## Mock para probar la integración
El mock se genera directamente del contrato con [Prism](https://github.com/stoplightio/prism), así que siempre coincide con él. Requiere Node.js:
```bash
npx @stoplight/prism-cli mock docs/contracts/traslados.yaml -p 4010
```
- **Valida cada pedido** contra el contrato: si falta un dato obligatorio responde `422 DATOS_INVALIDOS`, y sin `Authorization` responde `401`.
- Devuelve los ejemplos del contrato. Para elegir uno, usar el header `Prefer`:
  - `Prefer: example=completado` (también `programado`, `noRealizado`, `cancelado`);
  - `Prefer: code=409` (franja sin lugar) o `Prefer: code=422` para forzar un error.
- **No guarda estado**: una reserva creada no avanza sola ni descuenta cupo. Para probar el ciclo completo, usar los ejemplos de cada estado.

## Historial de versiones
| Versión | Fecha | Cambios | Compatible con la anterior |
| --- | --- | --- | --- |
| 4.0.0 | 2026-10-28 | **Capacidad de autoservicio: reservar una franja** (ADR-021). Nuevo `GET /disponibilidad`. `POST /traslados` exige `franja_inicio` (ya no `hora_llegada_solicitada` ni `tolerancia_minutos`) y responde `201` con el traslado `PROGRAMADO` o `409 SIN_DISPONIBILIDAD` (código nuevo). El recurso que se informa es el traslado (`TrasladoReservado`: `estado` del traslado, `franja`, `cancelacion.tardia`). Se eliminan `RECIBIDA`, `CONTRAPROPUESTA`, `RECHAZADA`, `RECHAZADA_POR_CLIENTE`, `CANCELADA`, `POST /traslados/{id}/respuesta-contrapropuesta`, `CONTRAPROPUESTA_VENCIDA` y la cancelación `PENDIENTE_APROBACION`: la cancelación se aplica en el momento | **No** (versión mayor). El consumidor solo usaba el mock: se le avisa y se actualiza el mock |
| 3.0.0 | 2026-10-16 | **Las rutas dejan de llevar versión:** `/v2/traslados` → `/traslados` y `/v2/auth/token` → `/auth/token`. Cuerpos, estados, errores, autenticación e idempotencia no cambian (ADR-019) | **No** (cambian las rutas). Sin consumidores todavía: no hubo migración |
| 2.1.0 | 2026-10-16 | Se documentan `502 SERVICIO_NO_DISPONIBLE` y `504 TIEMPO_AGOTADO`, que el punto de entrada ya podía devolver (deadline de 5 s, ADR-004), ahora con Problem Details. Nuevo `codigo` `TIEMPO_AGOTADO` | **Sí** (versión menor) |
| 2.0.0 | 2026-10-11 | Capacidad genérica: `cosa` (descripción + cantidad; peso y volumen opcionales) en lugar de `carga`; solo `cosa`, `origen` y `destino` obligatorios; hora y referencia opcionales (sin hora, Rumbo propone una); recurso `/v2/traslados`; código `EXCEDE_CAPACIDAD` (antes `CARGA_EXCEDE_CAPACIDAD`). Corrección de la Entrega 1 (ADR-006) | **No** (por eso es versión mayor) |
| 1.0.0 | 2026-10-04 | Versión inicial, específica de carga (`/v1/solicitudes-carga`) | — |
