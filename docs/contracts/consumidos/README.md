# Contratos que Rumbo consume

Copias **fijadas** de los contratos publicados por otros grupos. Rumbo trabaja a partir del contrato y su documentación, sin depender de explicaciones privadas (consigna §4.1). La copia fija la versión exacta contra la que se programó y se testeó: si el proveedor publica un cambio, se actualiza la copia en un PR y el **test de contrato** muestra si el cambio es compatible.

| Capacidad | Proveedor | Archivo | Versión | Origen (commit) |
| --- | --- | --- | --- | --- |
| Clima del aeropuerto (`GET /v1/airports/{icao}/conditions`) | Grupo del sistema de aeropuerto | [`clima-v1.yaml`](clima-v1.yaml) | 1.0.0 | [`MaxiBarella/Proyecto-arquitectura@d9113be`](https://github.com/MaxiBarella/Proyecto-arquitectura/tree/d9113be/docs/contracts) (`docs/contracts/clima-v1.yaml`) |

Documentación completa del proveedor: [README de su contrato](https://github.com/MaxiBarella/Proyecto-arquitectura/blob/d9113be/docs/contracts/README.md). Cómo lo usa Rumbo: ADR de D9.

En desarrollo local, `compose.yaml` levanta su mock (Prism, a partir de este archivo) como servicio `clima-mock`.
