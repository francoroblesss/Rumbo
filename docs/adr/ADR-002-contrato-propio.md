# ADR-002 — Contrato propio: Solicitar traslado de carga (D8)

- **Fecha:** 2026-10-04
- **Estado:** aceptada (versión 1.0.0 para la Entrega 1; se actualiza en la Entrega 2 si el contrato cambia) — **reemplazada parcialmente por [ADR-006](ADR-006-contrato-generico-v2.md)**: la capacidad pasa a ser genérica (cosa, origen, destino) en la versión 2.0.0 (`/v2`). El resto sigue vigente.
- **Revisión 2026-10-04:** la capacidad la atiende el nuevo servicio **Integración de carga** en lugar de Operaciones (ADR-001). El contrato no cambia.

## Contexto
La consigna (§4.1) exige publicar una capacidad propia con un **contrato formal y versionado**, en un formato estándar y procesable, que otro grupo pueda consumir sin explicaciones privadas y que incorpore a un **flujo importante** de su negocio.

Rumbo publica **"Solicitar traslado de carga"**: una empresa de stock pide trasladar carga entre sucursales (`dominio.md` §15). La capacidad la expone el servicio **Integración de carga** a través del API Gateway; las decisiones de negocio las toma Operaciones (ADR-001). Para la Entrega 1 se pide el contrato con un mock.

Restricciones del dominio que el contrato tiene que respetar:
- **Recibir no es confirmar:** un operador revisa cada solicitud y la acepta, contrapropone o rechaza. Puede pasar tiempo entre una cosa y otra.
- La contrapropuesta vence a las 24 h.
- Cancelar con menos de 24 h de anticipación es una excepción que aprueba un operador.
- La carga se describe con descripción, bultos y peso obligatorios.

## Decisión
1. **API REST sincrónica descrita con OpenAPI 3.1** (`docs/contracts/solicitudes-carga-v1.yaml`). Es el formato estándar y procesable para HTTP: sirve para generar el mock, validar pedidos y escribir el test de contrato del consumidor.
2. **Procesamiento asíncrono expuesto con `202 Accepted`:** crear devuelve la solicitud registrada en estado `RECIBIDA`, con id durable y `Location`. El resultado llega después (teórico 4: aceptación → registro durable → finalización; un 202 sin persistir sería trasladar la incertidumbre al consumidor).
3. **Seguimiento por consulta (polling):** `GET /v1/solicitudes-carga/{id}` y `GET /v1/solicitudes-carga?actualizado_desde=` para traer solo lo que cambió. Rumbo no llama al sistema del consumidor.
4. **Estados del dominio en el contrato:** se exponen el estado de la solicitud y, si fue aceptada, el del traslado. La demora es un dato (`demora_minutos`), no un estado. La contrapropuesta y la cancelación son operaciones explícitas, con las reglas de 24 h del dominio.
5. **Autenticación con token firmado (client credentials):** `POST /v1/auth/token` → `Authorization: Bearer`. El Gateway valida firma y vigencia, y responde 401 si falla. Cada organización solo ve sus solicitudes (teórico 6: autenticación centralizada en el borde, autorización en el servicio dueño).
6. **Idempotencia obligatoria en los `POST`** con `Idempotency-Key` (teórico 7). Integración de carga guarda 24 h la clave con el cuerpo y el resultado, en el mismo documento que la solicitud: el mismo par devuelve el mismo resultado; la misma clave con otro cuerpo devuelve `409`. Así el consumidor puede reintentar ante timeouts o `503` sin duplicar traslados.
7. **Errores con Problem Details (RFC 9457)** y un `codigo` estable, documentado con su interpretación y acción esperada. Se separa 400 (JSON mal formado) de 422 (datos inválidos o regla de negocio), 409 (conflicto de estado o de idempotencia), 429 con `Retry-After` (límite por organización, teórico 6) y 503 (reintentable).
8. **Versionado:** versión mayor en la URL (`/v1`) y semver en el contrato (`info.version`). Primero se busca la **evolución compatible** (campos opcionales nuevos; el consumidor ignora lo que no conoce). Un cambio incompatible, incluido **agregar estados**, implica `/v2` conviviendo con `/v1` hasta el fin de la evaluación. Los cambios se registran en `docs/contracts/README.md`.
9. **Mock generado del contrato con Prism:** `npx @stoplight/prism-cli mock docs/contracts/solicitudes-carga-v1.yaml`. Valida los pedidos y responde con los ejemplos de cada estado.

## Alternativas consideradas
- **Webhook además de la consulta.** Descartada para la v1: Rumbo pasaría a depender de la disponibilidad del sistema del consumidor y habría que firmar y reintentar cada aviso. La consulta con `actualizado_desde` resuelve el seguimiento con menos acoplamiento. Se puede agregar en una versión menor sin romper la v1.
- **Eventos en un broker compartido entre grupos.** Descartada: exige infraestructura común entre equipos y acopla su operación.
- **Respuesta `201 Created` con el traslado confirmado.** Descartada: contradice el dominio, porque la aceptación la decide un operador después. Prometer un traslado que todavía no está comprometido sería un resultado falso.
- **API key fija.** Descartada: el teórico 6 propone token firmado y validado en el borde; la API key aparece solo como una dimensión del rate limiting. Además, el token unifica el mecanismo con el resto de los usuarios.
- **Permitir modificar la solicitud en la v1.** Descartada: suma reglas (revalidar disponibilidad, plazo de 24 h por cambio) sin que el flujo del consumidor lo necesite. Cancelar y crear otra cubre el caso. Agregarlo después es un cambio compatible.
- **Mock propio en Go con estados simulados.** Descartada para esta entrega: es código que puede desalinearse del contrato. Prism garantiza que el mock coincide con él.

## Consecuencias
- **Ganamos:** el consumidor tiene un contrato procesable para generar clientes y tests de contrato; los reintentos no duplican traslados; Rumbo no depende de la disponibilidad del consumidor; las reglas del dominio quedan explícitas en el contrato.
- **Pagamos:**
  - El consumidor conoce los cambios con hasta un minuto de demora (por el polling).
  - Rumbo tiene que almacenar las claves de idempotencia (24 h) y validar tokens en el Gateway.
  - El mock no simula transiciones de estado.
  - Agregar un estado obliga a sacar una versión mayor.
- **Pendiente para la implementación:** la URL pública de producción (se publica al desplegar, requerida para la Presentación Grupal) y las credenciales reales del grupo consumidor.

## Cómo se verifica
- El mock levanta desde el contrato y valida los pedidos: se probó con un pedido válido (`202`), uno sin `peso_kg` (`422 DATOS_INVALIDOS`), uno sin token (`401`), la selección de ejemplos por estado y un error forzado (`409`).
- En la implementación, un test de contrato valida las respuestas reales de Integración de carga contra `solicitudes-carga-v1.yaml`.
- Un `POST` repetido con la misma `Idempotency-Key` crea una sola solicitud.
