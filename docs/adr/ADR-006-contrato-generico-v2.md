# ADR-006 — Contrato genérico "Solicitar traslado" v2 (reemplaza parcialmente a ADR-002)

- **Fecha:** 2026-10-11
- **Estado:** aceptada — **reemplazada parcialmente por [ADR-019](ADR-019-contrato-sin-version-en-la-ruta.md)**: las rutas `/v2/...` pasan a `/traslados` y `/auth/token` (contrato 3.0.0). La capacidad genérica sigue vigente.
- **Reemplaza a:** la **definición de la capacidad y del contrato** de [ADR-002](ADR-002-contrato-propio.md) (versión 1.0.0, específica de carga). El resto de ADR-002 sigue vigente: REST con OpenAPI, `202` con procesamiento asíncrono, seguimiento por consulta, token firmado, idempotencia, Problem Details, política de versionado y mock con Prism.

## Contexto
En la corrección de la Entrega 1, el profesor indicó:
- que un traslado es **"una función con tres parámetros: cosa, inicio y destino"**;
- que, para que el grupo de gestión de almacenamiento nos consuma, la capacidad tiene que ser **más abstracta que cerrada**: "cambiar persona por cosa en los parámetros";
- que hay que revisar los **endpoints `/v1` y actualizar las versiones**, sin que la documentación quede desfasada respecto del código.

El contrato 1.0.0 (`/v1/solicitudes-carga`) exigía cinco datos, todos con significado de carga de stock: referencia externa, hora de llegada, descripción, bultos y peso. Para un consumidor que solo quiere "mover algo de A a B", eso es un modelo cerrado.

El teórico 6 dice que, ante un cambio de contrato, primero se busca la **evolución compatible**. Este cambio no lo es: se renombran campos (`carga` → `cosa`, `bultos` → `cantidad`), cambian los obligatorios y cambia el recurso. Según nuestra propia política (ADR-002), un cambio incompatible es una **versión mayor nueva**.

## Decisión
1. **Nueva versión mayor 2.0.0** del contrato, en `docs/contracts/traslados-v2.yaml`, bajo **`/v2`**. El recurso pasa a ser **`/v2/traslados`** y el token se obtiene en `/v2/auth/token`.
2. **Tres datos obligatorios:** `cosa` (descripción y cantidad), `origen` y `destino`. Son opcionales:
   - `cosa.peso_kg` y `cosa.volumen_m3`; si se informa el peso, Rumbo lo usa para elegir un vehículo con capacidad suficiente;
   - `hora_llegada_solicitada` y `tolerancia_minutos`;
   - `referencia_externa` y `observaciones`.
3. **Sin hora pedida**, el operador elige un horario según la disponibilidad y lo envía como **contrapropuesta**: la solicitud pasa a `CONTRAPROPUESTA` y el consumidor la acepta o la rechaza. No se crea una regla nueva: se reutiliza el ciclo de la solicitud del dominio (Rumbo nunca fija un horario sin el acuerdo del cliente).
4. **La versión 1.0.0 queda en el repo marcada como reemplazada.** No se implementa ni se publica, porque nunca tuvo consumidores. Como no hay nadie que migrar, **no se mantienen `/v1` y `/v2` conviviendo**. La convivencia de versiones que pide ADR-002 aplica cuando existen consumidores.
5. **Coherencia entre documentación y código:**
   - el gateway rutea `/v2/auth/` y `/v2/traslados` y ya no expone rutas `/v1/solicitudes-carga`;
   - la **API interna** de la aplicación web también queda versionada: `/api/v1/<servicio>/…`;
   - los documentos vivos (README, `ARCHITECTURE.md`, `SPEC.md`, `dominio.md` y la documentación del contrato) usan el mismo nombre de la capacidad y las mismas rutas.
6. **Adentro de Rumbo el dominio no cambia:** personas, pacientes, alumnos y carga siguen siendo conceptos internos. Es el servicio **Integración de carga** el que traduce "cosa" al modelo interno (traslado de carga), como lo establece ADR-001 ("un contexto traduce lo que recibe", teórico 8).

## Alternativas consideradas
- **Corregir el contrato dentro de `/v1`**, ya que nadie lo consume. Descartada: el profesor pidió actualizar las versiones, y nuestra política (ADR-002) define que un cambio incompatible es una versión mayor. Hacerlo así deja la historia visible.
- **Mantener `/v1` y `/v2` conviviendo.** Descartada: `/v1` no tiene consumidores ni implementación, así que mantenerla solo sumaría código sin un problema que resolver.
- **Generalizar también el dominio interno** (todo traslado mueve "cosas"). Descartada: la corrección apunta a la capacidad publicada. El dominio interno necesita distinguir personas (padrón, ausencias, silla de ruedas) de carga.
- **Hacer obligatoria la hora.** Descartada: sería un modelo menos abstracto, y el ciclo de contrapropuesta ya resuelve el caso sin regla nueva.

## Consecuencias
- **Ganamos:**
  - una capacidad simple y genérica (tres datos), más fácil de consumir para cualquier grupo;
  - versiones coherentes entre contrato, gateway y documentación;
  - historia del contrato visible: v1 queda registrada como reemplazada.
- **Pagamos:**
  - sin peso informado, Rumbo no puede validar la capacidad del vehículo al recibir el pedido. Lo resuelve el operador al planificar (puede rechazar o contraproponer);
  - sin hora informada, siempre hay una vuelta más (la contrapropuesta) antes de que el traslado quede comprometido.

## Cómo se verifica
- El mock v2 (Prism) acepta el pedido mínimo (`cosa`, `origen`, `destino`) con `202`, rechaza el pedido sin `cosa` con `422 DATOS_INVALIDOS` y no expone `/v1`. **Verificado** el 11/10/2026.
- En la implementación, el test de contrato de Integración de carga valida las respuestas contra `traslados-v2.yaml`.
