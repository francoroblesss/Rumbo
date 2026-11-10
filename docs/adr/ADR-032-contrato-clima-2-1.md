# ADR-032 — Copia fijada del contrato de clima 2.1.0 (reemplaza parcialmente a ADR-020)

- **Fecha:** 2026-11-10
- **Estado:** aceptada
- **Reemplaza parcialmente a:** [ADR-020](ADR-020-contrato-clima-2.md), en la copia fijada del contrato: `docs/contracts/consumidos/clima.yaml` pasa de la 2.0.1 (commit `70ad2d6` del proveedor) a la **2.1.0** (commit `9488714`). El resto de ADR-020 sigue vigente (ruta, timeouts y servicio dormido), con lo que cambió ADR-025.

## Contexto
- El 07/11 el grupo del sistema de aeropuerto publicó la versión **2.1.0** de su contrato de clima (`MaxiBarella/Proyecto-arquitectura`, commit `9488714`, "Suma INTERNAL_ERROR y el 500 al contrato de Clima"). Nos enteramos el 10/11, cuando nos pasaron la clave.
- Por su política de versiones (mayor.menor.parche), un cambio **menor** es compatible. El único cambio es que `GET /airports/{icao}/conditions` puede responder **`500 INTERNAL_ERROR`** ante un error inesperado, en formato Problem Details y sin el detalle, con la indicación "reintentar con espera creciente". La ruta, el header `X-API-Key`, el ICAO, el `200` y el `503 WEATHER_SOURCE_UNAVAILABLE` no cambian.
- `docs/contracts/consumidos/README.md` dice que la copia fija la versión contra la que se programó y se testeó, y que ante un cambio del proveedor se actualiza en un PR y el **test de contrato** muestra si es compatible (como pasó con la 2.0.1 en ADR-020).

## Decisión
- **Actualizar la copia fijada a la 2.1.0**, tal cual la publicó el proveedor, y anotarla en el historial de `docs/contracts/consumidos/README.md`.
- **El adaptador no cambia.** Ya trata cualquier `5xx` como falla del proveedor: un reintento con jitter, cuenta para el circuit breaker y, si sigue fallando, la operación sigue con "clima no verificado" (fail-open, ADR-012 y ADR-025). Es lo mismo que pide el contrato nuevo para el `500`.
- El mock local (`clima-mock`, Prism) se arma desde esta copia, así que también conoce el `500`.

## Alternativas consideradas
- **Quedarse con la copia 2.0.1.** Descartada: la copia tiene que ser la que publica el proveedor, para que el test de contrato valide contra lo vigente y el mock responda lo mismo que el servicio real.

## Consecuencias
- **Ganamos:** el test de contrato y el mock validan contra la versión vigente del proveedor.
- **Pagamos:** nada en código; un ADR y una fila en el historial de la copia.

## Cómo se verifica
- `go test ./adaptadores/clima/...` en `servicios/operaciones` con la copia 2.1.0: pasa (10/11/2026). El caso "500: un reintento" del test ya cubría este comportamiento.
