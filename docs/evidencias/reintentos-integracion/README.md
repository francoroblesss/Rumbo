# Reintento de Integración a Operaciones

Corrida del **23/11/2026, 19:27–19:32 UTC** (16:27–16:32 en Córdoba), con `pruebas/fallas/reintentos.sh` ([ADR-035](../../adr/ADR-035-reintento-integracion-operaciones.md)), volúmenes limpios, perfil de observabilidad y cupo alto (`FRANJA_CUPO_TRASLADOS=100000`), en Windows 11 con Docker Desktop.

**Qué se hizo:** k6 con `caida-operaciones.js` durante 4 min (0,4 reservas, 0,4 consultas por id y 0,1 consultas de disponibilidad por segundo, por el gateway y con el token del sistema externo) y, en los minutos 1, 2 y 3, `docker compose pause operaciones`, `sleep 1.5` y `docker compose unpause operaciones`. Es la "validación del endpoint" del reintento del lado del servidor: congelar Operaciones un instante con tráfico real y ver qué le llega al consumidor.

## Estados por minuto (de `resumen.md`; minuto 0 = primer pedido, 19:27:53)
| Minuto | Pausa dentro del minuto | Reservas | Consultas por id | Disponibilidad | Reserva más lenta |
| --- | --- | --- | --- | --- | --- |
| 0 | 1 (19:28:32–19:28:37) | 27 `201` | 21 `200` | 6 `200` | 2195 ms |
| 1 | 2 (19:29:31–19:29:34) | 24 `201` | 24 `200` | 6 `200` | 1581 ms |
| 2 | 3 (19:30:31–19:30:36) | 24 `201` | 24 `200` | 6 `200` | 999 ms |
| 3 | — | 24 `201` | 24 `200` | 6 `200` | 57 ms |
| 4 | — | 4 `201` | 4 `200` | — | 30 ms |

**Total: 225 pedidos, 0 errores** (`http_req_failed 0.00%`, los 242 checks bien, `k6-salida.txt`). **El consumidor no vio ningún `503`**, frente a los `503` de toda la caída del postmortem.

Las horas de cada pausa son las de `linea-temporal.txt`: se anotan antes del `pause` y después del `unpause`, así que incluyen lo que tarda `docker compose` en Windows (1–2 s por comando). Operaciones estuvo congelado menos de ese intervalo, pero más de los 1,5 s del `sleep`.

## Reintentos (`metricas.txt` y `logs.txt`)
| Hora (UTC) | Réplica | Operación | Primer intento | Espera | Resultado para el consumidor |
| --- | --- | --- | --- | --- | --- |
| 19:28:36 | integracion-1 | `reserva` | timeout de 1 s (`Client.Timeout exceeded`) | 262 ms | `201` en 2195 ms |
| 19:29:33 | integracion-2 | `disponibilidad` | timeout de 1 s | 288 ms | `200` en 1550 ms |
| 19:29:33 | integracion-1 | `reserva` | timeout de 1 s | 287 ms | `201` en 1581 ms |

- **3 reintentos** (2 de `reserva`, 1 de `disponibilidad`), los tres con un segundo intento que respondió bien. `rumbo_reintentos_total` da lo mismo que los logs: `integracion-1 · reserva = 2` e `integracion-2 · disponibilidad = 1`. La consulta `increase(rumbo_reintentos_total[10m])` del script da menos (1,14 y 0): una serie de contador que nace en 1 no tiene muestra anterior, y `increase()` no cuenta ese primer incremento. Por eso `metricas.txt` trae también el valor acumulado.
- Las esperas (262–288 ms) caen en el rango del primer backoff: 200 ms con ±50 % de jitter, entre 100 y 300 ms.
- En la tercera pausa no hubo reintento: la reserva que la cruzó tardó 999 ms y respondió justo antes de que venciera el timeout de 1 s.
- **Ningún `no se pudo llamar a Operaciones`** (no se agotaron los intentos) y el breaker no se abrió: `max(rumbo_circuito_estado{dependencia="operaciones"}) = 0`.

## Reservas (`reservas.txt`)
- 103 claves con `201`, cada una con **una sola** fila en `claves_idempotencia`, y 103 solicitudes con esa referencia externa, **0 repetidas**. Las dos reservas reintentadas no se duplicaron: el pedido congelado se procesó al volver (timeout = resultado desconocido, teórico 7), y el reintento con la misma `Idempotency-Key` recibió la reserva ya hecha.
- La línea "claves que recibieron solo `503`" sale vacía porque no hubo ninguna clave con `503`: la lista queda vacía y la consulta `IN ()` no devuelve nada.
- No hubo ningún `5xx` de Operaciones en un reintento de reserva (el caso de un pedido original todavía en curso que anota ADR-035).

## Cómo se corrió en Windows
- `python3` es el atajo de la Microsoft Store, así que las consultas a Prometheus del script no corrieron en el momento. Se repitieron con `python` y `time=2026-11-23T19:33:18Z`, la hora en que las hubiera hecho el script (el encabezado de `metricas.txt` lo dice).
- `resumir.mjs` y `verificar-reservas.sh` comparan rutas de estilo Linux (`file://` + `argv[1]`, `/tmp`). `resumen.md` se generó con `node:22-alpine` en Docker y `verificar-reservas.sh`, copiando las listas de claves de `C:\tmp` a `/tmp` de Git Bash. No se cambiaron los scripts.

El detalle por pedido (`k6.json`) se generó durante la corrida y no se versiona: los números salen de `resumen.md`, `k6-salida.txt` y `logs.txt`.
