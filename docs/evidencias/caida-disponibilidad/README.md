# Caída de Operaciones con `GET /disponibilidad`

Corrida del **10/11/2026, 13:50–14:05 UTC** (10:50–11:05 en Córdoba), con el mismo procedimiento que el postmortem (`pruebas/fallas/`, `docker compose stop operaciones` en el minuto 3 y `start` en el 11) y volúmenes limpios.

**Qué se agregó respecto del postmortem:** un tercer escenario de k6, `disponibilidad`, con 0,1 consultas por segundo de `GET /disponibilidad?fecha=<mañana>&peso_kg=50` (54 pedidos por minuto en total). Acción de mejora 6 de `docs/POSTMORTEM.md`. Las evidencias del postmortem (`docs/evidencias/postmortem/`) no se tocaron: esta corrida se guardó con `EVIDENCIAS=docs/evidencias/caida-disponibilidad`.

## `GET /disponibilidad` (de `resumen.md` y `k6-salida.txt`)
| | Resultado |
| --- | --- |
| Antes de la caída (minutos 0–2) | 18 `200` |
| Durante la caída (minutos 3–10) | 48 pedidos: **15 `503`** y **33 `502`** |
| `503` con `Retry-After` | Los 15. El check `503 con Retry-After` no falló nunca: las 266 fallas de checks de la corrida son exactamente las 266 respuestas que no eran `200`/`201` |
| Primer `503` | 13:53:20, 10 s después del `stop` (primera consulta de disponibilidad que salió con Operaciones abajo), en 21 ms: el `stop` da error de conexión inmediato, no hay timeout |
| Primer `502` | 13:54:10, 60 s después del `stop`; desde ahí los `502` son mayoría (33 de los 48) |
| Primer `200` al volver | 14:01:20, 9 s después del `start` (Operaciones sano a las 14:01:18) |
| Después de la caída (minutos 11–15) | 25 `200`, ningún error |

**Comparación con las reservas de la misma corrida:** primer `503` a las 13:53:15 (5 s después del `stop`), primer `201` al volver a las 14:01:15 (4 s después del `start`). Disponibilidad degrada y se recupera igual que las reservas; la diferencia de segundos se debe solo a la tasa (una consulta cada 10 s contra una reserva cada 2,5 s).

## Hallazgo: `502` del balanceador de Integración
Durante la caída, además de los `503` esperados, hubo **`502` en las tres operaciones**: 33 de disponibilidad, 31 reservas y **26 `GET /traslados/{id}`**, que no dependen de Operaciones y en el postmortem dieron 361 `200` de 361. El `502` lo genera el NGINX que balancea Integración (`no live upstreams while connecting to upstream` en sus logs; el gateway registra el `502` en `logs.txt`): `infra/nginx/integracion.conf` reintenta con `proxy_next_upstream … http_503` y marca una réplica como caída con `max_fails=3 fail_timeout=30s`, así que los `503` correctos de Integración cuentan como fallas y, con las dos réplicas marcadas, NGINX corta todo por 30 s. Con la misma configuración, el postmortem no tuvo `502`: no se encontró por qué esta vez sí. **No se corrigió** (no era parte de esta tarea); queda para revisar.

## Reservas (`reservas.txt`)
178 claves con `201`, cada una con una sola fila en `claves_idempotencia` y una sola solicitud con esa referencia externa (sin duplicadas); **0** claves que recibieron solo `503` tienen reserva (ninguna reserva hecha sin que el consumidor lo sepa). Las 3 reservas reintentadas con la misma `Idempotency-Key` dieron `201` (`reintentos.txt`).

El detalle por pedido (`k6.json`) se generó durante la corrida y no se versiona; los números salen de `resumen.md` y `k6-salida.txt`.
