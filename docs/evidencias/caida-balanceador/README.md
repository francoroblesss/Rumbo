# Caída de Operaciones y réplica caída con el balanceador corregido (ADR-031)

Corrida del **10/11/2026, 14:27–14:45 UTC** (11:27–11:45 en Córdoba), con volúmenes limpios y cupo alto.

**Qué cambió respecto de la corrida del PR #33** (`docs/evidencias/caida-disponibilidad/`): solo la línea `proxy_next_upstream` de `infra/nginx/integracion.conf`, que pasó de `error timeout http_502 http_503` a `error timeout` ([ADR-031](../../adr/ADR-031-balanceador-solo-fallas-de-replica.md)). Se verificó en el contenedor antes de correr (`docker compose exec balanceador-integracion grep proxy_next_upstream /etc/nginx/nginx.conf` → `error timeout`). El procedimiento (`pruebas/fallas/`), el tráfico de k6 y los minutos del `stop` (14:30:01) y del `start` (14:38:01) de Operaciones son los mismos.

## A. Caída de Operaciones (de `resumen.md` y `k6-salida.txt`)
Pedidos **durante la caída** (minutos 3–10 de cada `resumen.md`):

| Operación | Antes (PR #33) | Después (esta corrida) |
| --- | --- | --- |
| `reservar` (`POST /traslados`) | 1 `201` · 161 `503` · **30 `502`** | 1 `201` · 191 `503` · **0 `502`** |
| `consultar` (`GET /traslados/{id}`) | 166 `200` · **26 `502`** | **192 `200`** · 0 `502` |
| `disponibilidad` (`GET /disponibilidad`) | 15 `503` · **33 `502`** | 48 `503` · **0 `502`** |

- **Cero `502` en toda la corrida** (la tabla de totales de `resumen.md` ni siquiera tiene la columna). `consultar` dio `200` en los 361 pedidos, todos los minutos.
- **`503` con `Retry-After`:** los 49 `503` de disponibilidad (48 durante la caída y 1 en el minuto 11) traían el encabezado: de los checks del escenario `disponibilidad` fallaron 49 de 182, exactamente los 49 pedidos que no dieron `200`, así que `503 con Retry-After` no falló nunca. Las 246 fallas de checks de `k6-salida.txt` son los 197 `503` de reservas y los 49 de disponibilidad.
- **Al volver:** primera reserva con `201` a las 14:38:16 y primer `200` de disponibilidad a las 14:38:21 (Operaciones sano a las 14:38:09). Desde el minuto 12, todo `200`/`201`.
- **`no live upstreams`:** no aparece ni en `logs.txt` ni en el log del balanceador (en la corrida del PR #33, sí).
- **Sin reintento en la otra réplica:** Integración registró 49 `503` de `GET /disponibilidad` (`logs.txt`), los mismos 49 que recibió k6. En la corrida del PR #33 registró 57 contra 15 que llegaron al consumidor: NGINX reintentaba cada `GET` con `503` en la otra réplica.

## Reservas (`reservas.txt` y `reintentos.txt`)
172 claves con `201`, cada una con una sola fila en `claves_idempotencia` y una sola solicitud con esa referencia externa (sin duplicadas); **0** claves que recibieron solo `503` tienen reserva (ninguna reserva oculta). Las 3 reservas reintentadas con la misma `Idempotency-Key` dieron `201`, con una fila cada una.

## B. Una réplica caída se sigue detectando (`replica-caida.txt` y `balanceador.txt`)
40 consultas de `GET /disponibilidad`, una cada 2 s, de 14:43:10 a 14:44:32 UTC. `docker compose stop integracion-2` a las 14:43:30 (detenido 14:43:31) y `docker compose start integracion-2` a las 14:43:56 (iniciado 14:43:57). En `balanceador.txt`, `172.20.0.15` es `integracion-1` y `172.20.0.14` es `integracion-2`.

- **Los 40 pedidos dieron `200`.**
- Antes del `stop`, las respuestas alternan entre las dos réplicas.
- Con `integracion-2` detenido, NGINX dio dos `upstream timed out … while connecting to upstream` hacia `172.20.0.14` (14:43:32 y 14:43:37): esos dos pedidos tienen dos direcciones en `replica=` (el reintento en `integracion-1`) y dieron `200`. De 14:43:39 a 14:43:59, todo lo atendió `integracion-1`.
- Desde 14:44:01, las respuestas vuelven a alternar entre las dos réplicas.

(Las 4 primeras líneas de `balanceador.txt`, de 14:41–14:42, son del final de la corrida de k6: el `--since 3m` las incluye.)

El detalle por pedido (`k6.json`) se generó durante la corrida y no se versiona; los números salen de `resumen.md` y `k6-salida.txt`.
