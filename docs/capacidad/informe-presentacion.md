# Informe de capacidad — Presentación: reservas simultáneas sobre la misma franja

Medición de la limitación declarada en `docs/ARCHITECTURE.md` §10: **las reservas sobre una misma franja se atienden de a una** (bloqueo de la fila con `SELECT … FOR UPDATE`, [ADR-022](../adr/ADR-022-consistencia-y-concurrencia.md), D4). Se sigue el método de [ADR-015](../adr/ADR-015-capacidad-y-costos.md) (escalones, SLO y regla del límite) y el estilo del [informe de la Entrega 2](informe-entrega-2.md).

- **Fecha:** 2026-11-08, de 17:17 a 17:25 UTC (una corrida completa de `bash pruebas/carga/ejecutar-misma-franja.sh`).
- **Scripts y datos crudos:** [`pruebas/carga/`](../../pruebas/carga/): `misma-franja.js`, `resultado-<misma|repartidas>.json` (k6), `bloqueos-<modo>.txt` (contadores de InnoDB cada 2 s), `stats-<modo>.txt` (`docker stats` cada 5 s) y `verificacion-misma.json`. Las tablas salen de `node pruebas/carga/resumir-misma-franja.mjs`.
- **Máquina:** notebook Apple M2 (8 núcleos, 8 GB) con macOS 26 y Docker Desktop (3,8 GB para Docker). Todo el sistema y k6 corren en la misma máquina: los números sirven para **comparar los dos modos** entre sí, no como capacidad absoluta de producción. Es otra máquina que la de la Entrega 2, así que las latencias no se comparan con ese informe.
- **Sistema levantado:** `gateway` y sus dependencias más `memcached` (2 réplicas de Integración detrás del balanceador), con `FRANJA_CUPO_TRASLADOS=100000` y `FRANJA_CUPO_KG=100000000` para que el cupo no corte la prueba. Sin observabilidad (Grafana) ni Solr.
- **Carga:** solo `POST /traslados` (reservas), cada una con una `Idempotency-Key` distinta. Escenario `ramping-arrival-rate` de k6 con escalones de **10, 25, 50 y 100 reservas/s**: 5 s de rampa y 30 s de meseta cada uno. Las tablas son de la meseta.
  - **Misma franja:** todas las reservas a la franja del 2026-11-11 de 10 a 12 h.
  - **Franjas distintas (control):** las reservas rotan entre las 173 franjas de los próximos 29 días (≈ 0,6 reservas/s por franja en el escalón de 100).
- **Destino:** `http://balanceador-integracion` dentro de la red de Docker, como en la Entrega 2 (el límite del gateway, 60 pedidos por minuto, cortaría la prueba). El token del sistema externo se pide una vez al gateway y de él salen los headers de identidad, más el secreto interno ([ADR-026](../adr/ADR-026-despliegue-en-render.md)).
- **Bloqueos:** contadores de InnoDB `Innodb_row_lock_waits` (cuántas veces una transacción esperó un bloqueo de fila), `Innodb_row_lock_time` (ms totales esperando) e `Innodb_row_lock_current_waits` (cuántas esperan en ese instante), leídos con `SHOW GLOBAL STATUS`. Se eligieron frente a `SHOW ENGINE INNODB STATUS` porque acumulan, y restando antes y después de cada meseta dan el tiempo de espera del escalón; ese comando solo muestra las esperas del momento. `performance_schema` está apagado en `compose.yaml`, así que **los contadores son de toda la instancia de MySQL, no de la tabla `franjas`**.
- **CPU:** % de **un** núcleo, mediana de las muestras de cada meseta sin sus primeros 5 s (como en la Entrega 2).
- **SLO (ADR-014):** p95 ≤ 300 ms y errores ≤ 1 %.

## Resultados por escalón

| Escalón (reservas/s) | Franjas | Logradas (req/s) | p50 (ms) | p95 (ms) | p99 (ms) | Errores (%) | SLO | Esperas por bloqueo | Tiempo esperando (ms) | Espera media (ms) | Esperando a la vez |
| ---: | --- | ---: | ---: | ---: | ---: | ---: | :---: | ---: | ---: | ---: | ---: |
| 10 | misma | 10,0 | 9,0 | 16,0 | 27,2 | 0,00 | sí | 1 | 13 | 13,0 | 0,0 |
| 10 | distintas | 10,0 | 8,8 | 16,4 | 23,8 | 0,00 | sí | 34 | 104 | 3,1 | 0,0 |
| 25 | misma | 25,0 | 5,6 | 21,4 | 34,5 | 0,00 | sí | 61 | 1113 | 18,2 | 0,1 |
| 25 | distintas | 25,0 | 5,5 | 13,0 | 20,1 | 0,00 | sí | 31 | 341 | 11,0 | 0,0 |
| 50 | misma | 50,0 | 4,3 | 31,2 | 93,2 | 0,00 | sí | 184 | 5801 | 31,5 | 0,0 |
| 50 | distintas | 50,0 | 4,4 | 25,5 | 50,0 | 0,00 | sí | 136 | 2792 | 20,5 | 0,1 |
| 100 | misma | 100,0 | 3,3 | 62,0 | 122,6 | 0,00 | sí | 675 | 27532 | 40,8 | 0,9 |
| 100 | distintas | 100,0 | 3,4 | 76,5 | 138,7 | 0,00 | sí | 542 | 27315 | 50,4 | 1,4 |

"Esperando a la vez" es el promedio de `Innodb_row_lock_current_waits` en las muestras de la meseta.

**CPU** (% de un núcleo, mediana de la meseta):

| Escalón (reservas/s) | Franjas | operaciones | mysql | integracion-1 | integracion-2 |
| ---: | --- | ---: | ---: | ---: | ---: |
| 10 | misma | 4,7 | 6,7 | 2,5 | 2,6 |
| 10 | distintas | 4,8 | 6,7 | 3,0 | 2,9 |
| 25 | misma | 7,4 | 10,0 | 4,0 | 3,8 |
| 25 | distintas | 7,0 | 11,1 | 3,3 | 3,6 |
| 50 | misma | 10,5 | 13,4 | 2,8 | 2,7 |
| 50 | distintas | 11,7 | 15,0 | 3,6 | 2,9 |
| 100 | misma | 18,8 | 21,5 | 5,2 | 5,4 |
| 100 | distintas | 20,2 | 22,3 | 6,0 | 6,2 |

## Verificación: no hay sobrerreserva (D4)
- En el modo "misma", `traslados_reservados` de la franja pasó de **0 a 6249**, y k6 recibió **6249 respuestas `201`** (de 6250 requests: la que falta es el pedido del token). **Coinciden exactamente: ni un lugar de más ni de menos.**
- En los dos modos, **0 % de errores** en todos los escalones: ningún `503` por timeout de bloqueo, deadlock ni breaker abierto.

## Principal límite observado
- **Hasta 100 reservas/s sobre una sola franja no se llegó al límite:** los cuatro escalones cumplen el SLO en los dos modos, con 0 % de errores y el throughput pedido.
- **La serialización por el `FOR UPDATE` se nota, pero poco, y recién desde 50 reservas/s:**
  - a 10 y 25 reservas/s los dos modos tienen p95 casi iguales (16,0 contra 16,4 ms y 21,4 contra 13,0 ms);
  - a 50 reservas/s el p99 de la misma franja casi duplica al de franjas distintas (93,2 contra 50,0 ms), y el tiempo total esperando bloqueos es el doble (5801 contra 2792 ms);
  - a 100 reservas/s hay en promedio 0,9 transacciones esperando un bloqueo en cada instante, y la espera media por bloqueo es de 41 ms.
- **Pero a 100 reservas/s el control tiene la misma latencia o peor** (p95 76,5 ms contra 62,0 ms, y el mismo tiempo total de espera, ~27,4 s). O sea que en ese escalón el aumento del p95 **no lo explica la franja compartida**, sino algo común a los dos modos. Lo que se midió no alcanza para decir qué es:
  - los contadores de InnoDB son de toda la instancia, no de la tabla `franjas` (performance_schema apagado). En el control hay esperas aunque cada franja reciba < 1 reserva/s, así que vienen de otras filas o índices que comparten todas las reservas; **no se midió cuáles**;
  - la CPU no es el límite: `mysql` usa 21–22 % y `operaciones` 19–20 % de un núcleo a 100 reservas/s, en los dos modos.
- Por la regla de ADR-015 (todos los escalones cumplen → el contenedor con más CPU en el último escalón), el principal límite es **`mysql` (21,5 % de un núcleo en la misma franja)**, con mucho margen.
- **Lo que no se midió:** escalones de más de 100 reservas/s (no se encontró el escalón que no cumple), el tiempo que cada transacción retiene el bloqueo de la franja, las esperas por tabla, ni la variación entre corridas (es **una sola corrida**: diferencias de pocos ms entre modos pueden ser ruido).

## Qué haría falta para escalar
- **Primero, medir más arriba** (200, 400 y 800 reservas/s, el mismo paso que deja pendiente ADR-015) para encontrar el escalón en que la misma franja deja de cumplir y comparar con el control. Mientras el control no se degrade, cualquier diferencia de ahí en adelante es la serialización por franja.
- **Si la franja compartida resultara el límite**, las salidas que mantienen la garantía de D4 son achicar lo que se bloquea, no sacar el bloqueo:
  - **franjas de menor granularidad** (por ejemplo, de 1 h en vez de 2 h): reparten las reservas de un pico entre más filas;
  - **cupo partido**: dividir el cupo de una franja en N filas (sub-cupos) y que cada reserva bloquee una sola al azar; si esa no tiene lugar, prueba otra. La suma de los sub-cupos sigue siendo el cupo, así que nunca se reserva de más, pero N reservas a la vez avanzan sin esperarse.
- Cualquiera de las dos cambia el calendario o el modelo de datos de la franja: sería un ADR nuevo que reemplace parte de ADR-021/ADR-022, y **con lo medido hoy no hace falta**.

## Conclusión para D4 y D13
- **D4 (ADR-022):** la garantía se cumple bajo carga: con 6249 reservas simultáneas sobre una franja, a hasta 100 por segundo, se ocuparon exactamente 6249 lugares para 6249 `201`, sin errores ni deadlocks. El costo de serializar por fila existe (la espera por bloqueo crece con la tasa y se ve en el p99 a 50 reservas/s), pero hasta 100 reservas/s queda dentro del SLO y no es mayor que el de repartir las reservas entre franjas. El bloqueo pesimista por fila es suficiente para el volumen esperado.
- **D13 (ADR-015):** la reserva sobre una misma franja **soporta más de 100 reservas/s** en esta máquina, con p95 ≤ 62 ms. El principal límite por la regla es `mysql` (21,5 % de un núcleo), sin saturar. El siguiente paso es el mismo que dejó la Entrega 2: medir escalones mayores.
