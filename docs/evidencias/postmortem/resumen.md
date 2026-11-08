# Resumen de k6 de la caída provocada

Generado por `pruebas/fallas/resumir.mjs` a partir de `k6.json`. Minuto 0 = primer pedido (2026-11-08T18:30:10.910Z). Horas en UTC. Incluye las reservas de `setup()` (minuto 0).

## Totales
| operación | 200 | 201 | 409 | 503 | total |
| --- | --- | --- | --- | --- | --- |
| reservar | 0 | 166 | 2 | 199 | 367 |
| consultar | 361 | 0 | 0 | 0 | 361 |

## Hitos de las reservas
| hito | hora (UTC) | duración |
| --- | --- | --- |
| primera reserva con 503 | 2026-11-08T18:33:14.656Z | 2005 ms |
| primera reserva con 503 en menos de 100 ms (sin esperar el timeout) | 2026-11-08T18:33:42.653Z | 3 ms |
| última reserva con 503 | 2026-11-08T18:41:27.628Z | 3 ms |
| primera reserva con 201 después de la caída | 2026-11-08T18:41:30.177Z | 52 ms |

## Por minuto
| minuto | desde (UTC) | operación | 200 | 201 | 409 | 503 |
| --- | --- | --- | --- | --- | --- | --- |
| 0 | 18:30:10 | reservar | 0 | 27 | 0 | 0 |
| 0 | 18:30:10 | consultar | 21 | 0 | 0 | 0 |
| 1 | 18:31:10 | reservar | 0 | 24 | 0 | 0 |
| 1 | 18:31:10 | consultar | 24 | 0 | 0 | 0 |
| 2 | 18:32:10 | reservar | 0 | 23 | 1 | 0 |
| 2 | 18:32:10 | consultar | 24 | 0 | 0 | 0 |
| 3 | 18:33:10 | reservar | 0 | 0 | 0 | 24 |
| 3 | 18:33:10 | consultar | 24 | 0 | 0 | 0 |
| 4 | 18:34:10 | reservar | 0 | 0 | 0 | 24 |
| 4 | 18:34:10 | consultar | 24 | 0 | 0 | 0 |
| 5 | 18:35:10 | reservar | 0 | 0 | 0 | 24 |
| 5 | 18:35:10 | consultar | 24 | 0 | 0 | 0 |
| 6 | 18:36:10 | reservar | 0 | 0 | 0 | 24 |
| 6 | 18:36:10 | consultar | 24 | 0 | 0 | 0 |
| 7 | 18:37:10 | reservar | 0 | 0 | 0 | 24 |
| 7 | 18:37:10 | consultar | 24 | 0 | 0 | 0 |
| 8 | 18:38:10 | reservar | 0 | 0 | 0 | 24 |
| 8 | 18:38:10 | consultar | 24 | 0 | 0 | 0 |
| 9 | 18:39:10 | reservar | 0 | 0 | 0 | 24 |
| 9 | 18:39:10 | consultar | 24 | 0 | 0 | 0 |
| 10 | 18:40:10 | reservar | 0 | 0 | 0 | 24 |
| 10 | 18:40:10 | consultar | 24 | 0 | 0 | 0 |
| 11 | 18:41:10 | reservar | 0 | 16 | 1 | 7 |
| 11 | 18:41:10 | consultar | 24 | 0 | 0 | 0 |
| 12 | 18:42:10 | reservar | 0 | 24 | 0 | 0 |
| 12 | 18:42:10 | consultar | 24 | 0 | 0 | 0 |
| 13 | 18:43:10 | reservar | 0 | 24 | 0 | 0 |
| 13 | 18:43:10 | consultar | 24 | 0 | 0 | 0 |
| 14 | 18:44:10 | reservar | 0 | 24 | 0 | 0 |
| 14 | 18:44:10 | consultar | 24 | 0 | 0 | 0 |
| 15 | 18:45:10 | reservar | 0 | 4 | 0 | 0 |
| 15 | 18:45:10 | consultar | 4 | 0 | 0 | 0 |

## Duración de las reservas por minuto y estado (ms, medida por k6)
| minuto | estado | pedidos | p50 | p95 | máximo |
| --- | --- | --- | --- | --- | --- |
| 0 | 201 | 27 | 34 | 281 | 784 |
| 1 | 201 | 24 | 29 | 49 | 312 |
| 2 | 201 | 23 | 31 | 237 | 451 |
| 2 | 409 | 1 | 12 | 12 | 12 |
| 3 | 503 | 24 | 425 | 2005 | 2005 |
| 4 | 503 | 24 | 3 | 2005 | 2006 |
| 5 | 503 | 24 | 3 | 2005 | 2005 |
| 6 | 503 | 24 | 4 | 2005 | 2006 |
| 7 | 503 | 24 | 3 | 2005 | 2007 |
| 8 | 503 | 24 | 4 | 2005 | 2005 |
| 9 | 503 | 24 | 4 | 2003 | 2005 |
| 10 | 503 | 24 | 4 | 2006 | 2007 |
| 11 | 201 | 16 | 32 | 52 | 52 |
| 11 | 409 | 1 | 6 | 6 | 6 |
| 11 | 503 | 7 | 3 | 5 | 5 |
| 12 | 201 | 24 | 28 | 42 | 42 |
| 13 | 201 | 24 | 25 | 48 | 51 |
| 14 | 201 | 24 | 26 | 45 | 68 |
| 15 | 201 | 4 | 36 | 41 | 41 |
