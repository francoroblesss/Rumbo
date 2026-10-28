# ADR-019 — Rutas sin versión (reemplaza parcialmente a ADR-002 y ADR-006)

- **Fecha:** 2026-10-16
- **Estado:** aceptada — las operaciones del contrato 3.0.0 quedan **reemplazadas por [ADR-021](ADR-021-capacidad-de-autoservicio.md)** (4.0.0). Las rutas sin versión siguen vigentes.
- **Reemplaza a:**
  - el punto 8 "Versionado" de [ADR-002](ADR-002-contrato-propio.md), en lo que dice de la **versión mayor en la URL** y de la convivencia de rutas `/vN`;
  - las rutas `/v2/...` de [ADR-006](ADR-006-contrato-generico-v2.md);
  - el prefijo versionado de la API interna (`/api/v1/<servicio>` y `/internas/v1/...`) que fijaron `docs/interfaces-internas.md` y [ADR-011](ADR-011-busqueda.md) (`/api/v1/operaciones/traslados/busqueda` pasa a `/api/operaciones/traslados/busqueda`).
  - El resto sigue vigente: REST con OpenAPI, la capacidad genérica (cosa, origen, destino), `202` asíncrono, polling, token, idempotencia, Problem Details, mock con Prism, y semver del contrato en `info.version` con historial en `docs/contracts/README.md`.

## Contexto
- ADR-002 puso la versión mayor en la ruta (`/v1`, después `/v2`), y la API interna se armó igual (`/api/v1/...`, `/internas/v1/...`).
- **Indicación del profesor:** en la clase del viernes le pidió a otro grupo que **sacara la versión de sus endpoints** (tenían rutas tipo `api/v2`), porque obliga a ir cambiando y actualizando las rutas todo el tiempo. Nos lo transmitió un compañero de ese grupo el 2026-10-16. El argumento vale igual para nuestras rutas públicas y para las internas.
- La consigna (§4.1) pide un **contrato formal y versionado**. Eso se cumple versionando el **documento del contrato**; la consigna no pide que la versión esté en la URL.
- **Teórico 9:**
  - la versión puede ir en la **ruta**, en un **header o media type**, o asociarse a una **fecha**, y "ninguna estrategia es universal";
  - el número de versión **no vuelve compatible un cambio**: lo que protege al consumidor es la política de evolución (cambios aditivos que el consumidor tolera) y, para los incompatibles, una **migración coordinada** o versiones que conviven;
  - la versión en la ruta es visible, pero duplica recursos y lógica.
- La capacidad todavía **no tiene consumidores**: no está desplegada (ADR-018) y nadie integró `/v2`. Cambiar las rutas ahora no rompe a nadie.

## Decisión
1. **Rutas sin versión:**

   | Antes | Ahora |
   | --- | --- |
   | `POST /v2/auth/token` | `POST /auth/token` |
   | `/v2/traslados` | `/traslados` |
   | `/v2/traslados/{id}` | `/traslados/{id}` |
   | `/v2/traslados/{id}/respuesta-contrapropuesta` | `/traslados/{id}/respuesta-contrapropuesta` |
   | `/v2/traslados/{id}/cancelacion` | `/traslados/{id}/cancelacion` |

   Cuerpos, estados, errores, autenticación e idempotencia no cambian.
2. **El contrato sigue versionado** con semver en `info.version`.
   - Pasa a la **versión 3.0.0** en `docs/contracts/traslados.yaml`, porque cambiar las rutas es incompatible.
   - La 2.1.0 (`traslados-v2.yaml`) queda en el repositorio marcada como reemplazada, igual que la 1.0.0.
   - Como no hubo consumidores, no hay convivencia.
3. **La API interna tampoco lleva versión** (mismo argumento):

   | Antes | Ahora |
   | --- | --- |
   | `/api/v1/<servicio>/...` (web → gateway) | `/api/<servicio>/...` |
   | `/internas/v1/solicitudes/...` (Integración → Operaciones) | `/internas/solicitudes/...` |

   El gateway sigue bloqueando todo path con `/internas/` y sigue quitando el prefijo `/api/<servicio>` antes de reenviar.
4. **Política de evolución** (`docs/contracts/README.md` § Compatibilidad):
   - **Preferir cambios compatibles** (versión menor o de parche): campos opcionales nuevos, endpoints nuevos y `codigo` de error nuevos. El consumidor ignora los campos desconocidos y, ante un `codigo` desconocido, decide por el HTTP status.
   - **Un cambio incompatible** (quitar o renombrar campos, cambiar significados, agregar estados) sube la versión mayor y se hace con **migración coordinada** con el grupo consumidor: se avisa con anticipación, se acuerda la fecha y se registra en el historial. Si alguna vez dos versiones tuvieran que convivir, se decide en un ADR nuevo cómo distinguirlas (por ejemplo, por media type), sin volver a meter la versión en la ruta.

## Alternativas consideradas
- **Mantener `/v2`.** Descartada: contradice la indicación del profesor, y la ruta con versión no protege al consumidor por sí sola (teórico 9).
- **Sacar la versión solo del contrato público y dejar `/api/v1` adentro.** Descartada: el motivo del profesor (cambiar rutas en cada versión) aplica igual, y la API interna tiene un único consumidor por ruta (la web o Integración), que se actualiza en el mismo pull request.
- **Versión en un header** (`Accept: application/vnd.rumbo.v3+json` o `Rumbo-Version`). Descartada por ahora: con un solo consumidor y sin versiones que tengan que convivir, suma una regla que nadie necesita. Queda como opción si alguna vez hace falta convivencia.
- **Versión por fecha** (como Stripe). Descartada: obliga a mantener varios comportamientos fechados.
- **Mantener `/v2` como alias de las rutas nuevas.** Descartada: nadie la usa, y mantenerla duplicaría rutas sin un problema que resolver (lo mismo que se decidió con `/v1` en ADR-006).

## Consecuencias
- **Ganamos:**
  - rutas estables para el grupo consumidor: `/traslados` no cambia aunque cambie la versión del contrato;
  - el gateway rutea por recurso;
  - la evolución depende de la política de compatibilidad, que es lo que protege al consumidor.
- **Pagamos:**
  - un cambio incompatible ya no puede publicarse en paralelo cambiando la ruta: hay que coordinarlo con el consumidor;
  - la versión no se ve en los logs ni en las métricas; se ve en el contrato.
- **Cambios en el sistema:**
  - el gateway rutea `/auth/`, `/traslados` y `/api/<servicio>/`;
  - Identidad atiende `POST /auth/token` e Integración atiende `/traslados`, con `Location: /traslados/{id}`;
  - Operaciones atiende `/internas/solicitudes/...` y el cliente de Integración la llama ahí; la web llama a `/api/identidad/login` y `/api/operaciones/solicitudes`;
  - la métrica agrupa por recurso (`ruta="/traslados"`, `ruta="/api/operaciones"`) y las alertas del SLO filtran por `/traslados`;
  - se actualizan el test de contrato, los scripts de k6 y la documentación viva.
  - Los documentos de evidencia y los ADR anteriores conservan `/v2`, porque registran lo que se midió y decidió en su momento.

## Cómo se verifica
- El test de contrato de Integración (`kin-openapi`) valida cada respuesta contra `traslados.yaml` 3.0.0.
- Con el sistema levantado:
  - `POST /auth/token` y `POST /traslados` a través del gateway devuelven `200` y `202`;
  - `/v2/traslados` responde `404`;
  - la métrica del gateway muestra `ruta="/traslados"`;
  - la consola del operador ingresa y opera por `/api/...`, y las acciones del sistema externo que llaman a Operaciones (`/internas/...`) responden.
