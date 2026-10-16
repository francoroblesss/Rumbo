# Informe de capacidad — Entrega 2

> **Nota (2026-10-16):** esta medición se hizo con las rutas bajo `/v2`. Desde [ADR-019](../adr/ADR-019-contrato-sin-version-en-la-ruta.md) son las mismas operaciones sin versión (`/traslados`, `/auth/token`); los comandos y resultados se conservan como se registraron.

Mediciones de la capacidad publicada "Solicitar traslado" con el método de [ADR-015](../adr/ADR-015-capacidad-y-costos.md). Las conclusiones (límite, escalado y costos) están en ese ADR.

- **Fecha:** 2026-10-16, de 15:25 a 15:37 UTC (corrida completa de `bash pruebas/carga/ejecutar.sh`).
- **Scripts y datos crudos:** [`pruebas/carga/`](../../pruebas/carga/): `traslados.js`, `resultado-<A|B|C>.json` (k6) y `stats-<A|B|C>.txt` (`docker stats` cada 5 s). Las tablas salen de `node pruebas/carga/resumir.mjs`.
- **Máquina:** notebook con Windows 11 y Docker Desktop (WSL 2), 12 núcleos lógicos y 7,6 GB para Docker. Todo el sistema corre en la misma máquina, con k6 incluido, así que los números sirven para **comparar escenarios** entre sí, no como capacidad absoluta de producción.
- **Destino:** `http://balanceador-integracion` dentro de la red de Docker, con los headers de identidad que pone el gateway. Mezcla: 90 % `GET /v2/traslados/{id}` y 10 % `POST /v2/traslados`. Escalones de 30 s.
- **CPU:** % de **un** núcleo (100 % = 1 vCPU), **mediana** de las ~5 muestras de cada escalón (se descartan los primeros 5 s de transición). Se usa la mediana porque los healthchecks de `mongo` (`mongosh` cada 5 s) y `rabbitmq` producen picos de 60 % a 180 % sin relación con la carga: en reposo, `mongo` marca ~1 % y salta a ~66 % en la muestra que coincide con el healthcheck.
- **SLO (ADR-014):** p95 ≤ 300 ms y errores ≤ 1 %.

## Escenario A — 1 réplica de Integración, sin Memcached

| Escalón (req/s) | Logradas (req/s) | p95 (ms) | p99 (ms) | Errores (%) | SLO | CPU integracion-1 (%) | CPU integracion-2 (%) | CPU mongo (%) | CPU memcached (%) | CPU balanceador (%) |
| ---: | ---: | ---: | ---: | ---: | :---: | ---: | ---: | ---: | ---: | ---: |
| 1 | 1,0 | 4,5 | 6,9 | 0,00 | sí | 0,4 | — | 2,2 | — | 0,0 |
| 2 | 2,0 | 5,6 | 405,8 | 0,00 | sí | 0,8 | — | 2,1 | — | 0,1 |
| 5 | 5,0 | 5,1 | 20,7 | 0,00 | sí | 1,5 | — | 4,8 | — | 0,2 |
| 10 | 10,0 | 5,0 | 9,5 | 0,00 | sí | 3,3 | — | 1,9 | — | 0,4 |
| 20 | 20,0 | 4,1 | 5,9 | 0,00 | sí | 5,8 | — | 4,1 | — | 0,8 |
| 50 | 50,0 | 4,1 | 6,6 | 0,00 | sí | 12,4 | — | 7,0 | — | 2,0 |
| 100 | 100,0 | 3,8 | 6,3 | 0,00 | sí | 22,7 | — | 11,0 | — | 4,0 |

**Capacidad: más de 100 req/s** (cumplen todos los escalones). En el escalón de 100 req/s, el contenedor con más CPU es **`integracion-1` (22,7 %)**.

## Escenario B — 1 réplica de Integración, con Memcached

| Escalón (req/s) | Logradas (req/s) | p95 (ms) | p99 (ms) | Errores (%) | SLO | CPU integracion-1 (%) | CPU integracion-2 (%) | CPU mongo (%) | CPU memcached (%) | CPU balanceador (%) |
| ---: | ---: | ---: | ---: | ---: | :---: | ---: | ---: | ---: | ---: | ---: |
| 1 | 1,0 | 4,6 | 5,0 | 0,00 | sí | 0,3 | — | 18,0 | 0,0 | 0,0 |
| 2 | 2,0 | 3,0 | 4,9 | 0,00 | sí | 0,7 | — | 1,8 | 0,1 | 0,1 |
| 5 | 5,0 | 2,5 | 4,0 | 0,00 | sí | 1,4 | — | 0,9 | 0,1 | 0,2 |
| 10 | 10,0 | 2,7 | 5,0 | 0,00 | sí | 1,9 | — | 9,4 | 0,3 | 0,5 |
| 20 | 20,0 | 2,7 | 5,2 | 0,00 | sí | 3,4 | — | 1,6 | 0,5 | 0,8 |
| 50 | 50,0 | 2,7 | 5,7 | 0,00 | sí | 7,3 | — | 3,6 | 1,2 | 2,0 |
| 100 | 100,0 | 2,4 | 4,1 | 0,00 | sí | 13,5 | — | 7,8 | 2,1 | 3,9 |

**Capacidad: más de 100 req/s.** En el escalón de 100 req/s, el contenedor con más CPU es **`integracion-1` (13,5 %)**.

## Escenario C — 2 réplicas de Integración, con Memcached

| Escalón (req/s) | Logradas (req/s) | p95 (ms) | p99 (ms) | Errores (%) | SLO | CPU integracion-1 (%) | CPU integracion-2 (%) | CPU mongo (%) | CPU memcached (%) | CPU balanceador (%) |
| ---: | ---: | ---: | ---: | ---: | :---: | ---: | ---: | ---: | ---: | ---: |
| 1 | 1,0 | 4,0 | 4,2 | 0,00 | sí | 0,2 | 0,4 | 41,2 | 0,1 | 0,0 |
| 2 | 2,0 | 4,0 | 7,8 | 0,00 | sí | 0,4 | 0,3 | 2,1 | 0,1 | 0,1 |
| 5 | 5,0 | 3,3 | 7,8 | 0,00 | sí | 0,8 | 0,9 | 1,6 | 0,1 | 0,2 |
| 10 | 10,0 | 3,2 | 6,1 | 0,00 | sí | 1,4 | 2,1 | 2,4 | 0,4 | 0,5 |
| 20 | 20,0 | 2,8 | 4,5 | 0,00 | sí | 2,0 | 1,7 | 1,7 | 0,5 | 0,8 |
| 50 | 50,0 | 2,4 | 5,0 | 0,00 | sí | 4,4 | 4,2 | 2,6 | 1,1 | 1,9 |
| 100 | 100,0 | 2,3 | 3,3 | 0,00 | sí | 7,5 | 6,2 | 4,5 | 2,0 | 3,7 |

**Capacidad: más de 100 req/s.** En el escalón de 100 req/s, el contenedor con más CPU es **`integracion-1` (7,5 %)**.

> Los valores aislados de `mongo` a 1 req/s (18 % en B y 41 % en C) son el healthcheck: con 5 muestras, en ese escalón coincidió en más de la mitad. No acompañan a la carga (en el escalón siguiente vuelven a ~2 %).

## Capacidad y principal límite (regla de ADR-015)
- **Ningún escenario llegó al límite:** los tres cumplen el SLO en todos los escalones, con p95 ≤ 5,6 ms y 0 % de errores a 100 req/s. La capacidad **supera las 100 req/s medidas** en A, B y C.
- **Principal límite** (contenedor con más CPU en el escalón de 100 req/s): **la réplica de Integración** en los tres escenarios. En A, `integracion-1` usa 22,7 % de un núcleo a 100 req/s y crece de forma aproximadamente lineal (≈ 0,23 % por req/s). Si se extrapola, una réplica sin caché llegaría a un núcleo cerca de las 400 req/s. Es una estimación que hay que confirmar midiendo escalones mayores.
- **p99 de 406 ms en A, escalón de 2 req/s:** no es carga. En A y B, `integracion-2` está detenida, pero NGINX la sigue teniendo en el upstream: cada ~32 s (`fail_timeout=30s`) vuelve a probarla, espera el `proxy_connect_timeout` de 1 s y reenvía el pedido a `integracion-1`. Con 61 requests en ese escalón, ese único pedido lento queda en el p99. Se ve en el log de NGINX como `replica=172.23.0.13:8085, 172.23.0.12:8085 tiempo=1.000, 0.002`. Es el costo del health check pasivo de [ADR-009](../adr/ADR-009-balanceo.md) y no afecta al p95.

## B contra A: efecto de la caché (ADR-007)
| Métrica (escalón de 100 req/s) | A (sin caché) | B (con caché) | Cambio |
| --- | ---: | ---: | ---: |
| p95 | 3,8 ms | 2,4 ms | −37 % |
| p99 | 6,3 ms | 4,1 ms | −35 % |
| CPU de `integracion-1` | 22,7 % | 13,5 % | −41 % |
| CPU de `mongo` | 11,0 % | 7,8 % | −29 % |
| CPU de `mongo` (escalón de 50 req/s) | 7,0 % | 3,6 % | −49 % |

- Con Memcached, el 90 % de lecturas por id se resuelve casi siempre en la caché: **hit ratio de 99,0 % en B** (7477 hits y 77 misses) y **97,8 % en C** (6925 hits y 154 misses), según `rumbo_cache_operaciones_total` durante cada escenario.
- **En A, Memcached está detenido** y la caché responde `error` en 6308 operaciones: el decorador cache-aside sigue y lee de MongoDB (fail-open). Por eso no hay errores para el consumidor: la caché mejora la latencia y la carga de la base, pero no es necesaria para funcionar.
- La caché baja el p95 y, sobre todo, la CPU de Integración y de MongoDB, porque cada lectura resuelta en Memcached es una consulta menos a la base. Con la base actual (20 solicitudes de prueba) MongoDB no es el cuello de botella. La caché protege a la base cuando el polling crezca (ADR-007).

## C contra B: efecto del balanceo (ADR-009)
| Métrica (escalón de 100 req/s) | B (1 réplica) | C (2 réplicas) |
| --- | ---: | ---: |
| p95 | 2,4 ms | 2,3 ms |
| CPU por réplica de Integración | 13,5 % | 7,5 % y 6,2 % |
| CPU total de Integración | 13,5 % | 13,7 % |

- **El trabajo se reparte entre las dos réplicas:** cada una usa la mitad de CPU que la réplica única de B, y el total es el mismo. El balanceo no hace más barato cada pedido: suma capacidad.
- **La latencia casi no cambia**, porque a 100 req/s una sola réplica todavía tiene margen (13,5 % de un núcleo). El balanceo mejora la latencia recién cerca de la saturación. Lo que sí aporta ya es **disponibilidad**: si cae una réplica, la otra sigue atendiendo.

## Capturas en texto de los paneles de Grafana
Tablero "Rumbo — estado general" (`infra/grafana/dashboards/rumbo-estado-general.json`). Los servicios exportan métricas cada 60 s, así que los paneles tienen resolución de 1 minuto. Las capturas son las mismas consultas evaluadas sobre la ventana de cada escenario.

**Panel 4 — Requests por instancia de Integración (balanceo).** `sum by (instancia) (rate(rumbo_http_duracion_segundos_seconds_count{job="integracion"}[2m]))`, durante C:
```
hora (UTC)   integracion-1   integracion-2
15:35:17         4,53 req/s      2,38 req/s
15:35:47         4,53 req/s      3,21 req/s
15:36:17        15,25 req/s      9,73 req/s
15:36:47        15,25 req/s     12,35 req/s
15:37:17        27,95 req/s     27,61 req/s   ← incluye el escalón de 100 req/s
```
Total del escenario C (`increase` sobre la ventana): `integracion-1` 4296 requests y `integracion-2` 3608 (54 % y 46 %). La diferencia viene de que `integracion-2` arranca al comienzo de C y exporta su primera muestra más tarde. En los escenarios A y B solo aparece `integracion-1` (6392 y 8387 requests).

**Panel 5 — Hit ratio de las cachés.** `sum(increase(rumbo_cache_operaciones_total{job="integracion",resultado="hit"}[…])) / sum(increase(…{resultado=~"hit|miss"}[…]))`:
```
escenario   hit      miss    error    hit ratio
A              33       22     6308    — (Memcached detenido: casi todo es error y se lee de MongoDB)
B            7477       77        0    99,0 %
C            6925      154        0    97,8 %
```
