# Resumen de k6 de la caída provocada

Generado por `pruebas/fallas/resumir.mjs` a partir de `k6.json`. Minuto 0 = primer pedido (2026-11-23T19:27:53.526Z). Horas en UTC. Incluye las reservas de `setup()` (minuto 0).

## Totales
| operación | 200 | 201 | total |
| --- | --- | --- | --- |
| reservar | 0 | 103 | 103 |
| consultar | 97 | 0 | 97 |
| disponibilidad | 24 | 0 | 24 |

## Hitos de las reservas
| hito | hora (UTC) | duración |
| --- | --- | --- |

## Por minuto
| minuto | desde (UTC) | operación | 200 | 201 |
| --- | --- | --- | --- | --- |
| 0 | 19:27:53 | reservar | 0 | 27 |
| 0 | 19:27:53 | consultar | 21 | 0 |
| 0 | 19:27:53 | disponibilidad | 6 | 0 |
| 1 | 19:28:53 | reservar | 0 | 24 |
| 1 | 19:28:53 | consultar | 24 | 0 |
| 1 | 19:28:53 | disponibilidad | 6 | 0 |
| 2 | 19:29:53 | reservar | 0 | 24 |
| 2 | 19:29:53 | consultar | 24 | 0 |
| 2 | 19:29:53 | disponibilidad | 6 | 0 |
| 3 | 19:30:53 | reservar | 0 | 24 |
| 3 | 19:30:53 | consultar | 24 | 0 |
| 3 | 19:30:53 | disponibilidad | 6 | 0 |
| 4 | 19:31:53 | reservar | 0 | 4 |
| 4 | 19:31:53 | consultar | 4 | 0 |
| 4 | 19:31:53 | disponibilidad | 0 | 0 |

## Duración de las reservas por minuto y estado (ms, medida por k6)
| minuto | estado | pedidos | p50 | p95 | máximo |
| --- | --- | --- | --- | --- | --- |
| 0 | 201 | 27 | 30 | 103 | 2195 |
| 1 | 201 | 24 | 24 | 50 | 1581 |
| 2 | 201 | 24 | 27 | 78 | 999 |
| 3 | 201 | 24 | 28 | 56 | 57 |
| 4 | 201 | 4 | 23 | 30 | 30 |
