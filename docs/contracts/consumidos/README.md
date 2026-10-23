# Contratos que Rumbo consume

Copias **fijadas** de los contratos publicados por otros grupos. Rumbo trabaja a partir del contrato y su documentación, sin depender de explicaciones privadas (consigna §4.1). La copia fija la versión exacta contra la que se programó y se testeó: si el proveedor publica un cambio, se actualiza la copia en un PR y el **test de contrato** muestra si el cambio es compatible.

| Capacidad | Proveedor | Archivo | Versión | Origen (commit) | Dirección real |
| --- | --- | --- | --- | --- | --- |
| Clima del aeropuerto (`GET /airports/{icao}/conditions`) | Grupo del sistema de aeropuerto | [`clima.yaml`](clima.yaml) | 2.0.1 | [`MaxiBarella/Proyecto-arquitectura@70ad2d6`](https://github.com/MaxiBarella/Proyecto-arquitectura/tree/70ad2d6896da850a3a5789e97c139c3dca7f960a/docs/contracts) (`docs/contracts/clima.yaml`) | `https://clima-aeropuerto.onrender.com` |

Documentación completa del proveedor: [README de su contrato](https://github.com/MaxiBarella/Proyecto-arquitectura/blob/70ad2d6896da850a3a5789e97c139c3dca7f960a/docs/contracts/README.md). Cómo lo usa Rumbo: ADR de D9.

En desarrollo local, `compose.yaml` levanta su mock (Prism, a partir de este archivo) como servicio `clima-mock`.

## Historial de la copia fijada
| Fecha | Versión | Commit del proveedor | Qué cambió | Qué hizo Rumbo |
| --- | --- | --- | --- | --- |
| 2026-10-14 | 1.0.0 | `d9113be` | Primera versión, con mock | Adaptador, test de contrato y mock ([ADR-012](../../adr/ADR-012-consumo-clima.md)) |
| 2026-10-23 | 2.0.1 | `70ad2d6` | Rompe: ruta sin `/v1`; `503` = dato de más de 30 min; precipitación de 15 min; servicio publicado que se duerme | El test de contrato falló con la copia nueva; se corrigió la ruta del adaptador ([ADR-020](../../adr/ADR-020-contrato-clima-2.md)) |

## Antes de una demo con el proveedor real
El servicio del proveedor se duerme sin uso. Unos minutos antes de la demo, despertarlo hasta recibir `200`:
```bash
curl --max-time 120 -H "X-API-Key: <clave>" https://clima-aeropuerto.onrender.com/airports/SACO/conditions
```
Mientras está dormido, Rumbo sigue funcionando y los traslados al aeropuerto quedan "clima no verificado" ([ADR-020](../../adr/ADR-020-contrato-clima-2.md)).
