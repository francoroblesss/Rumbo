# Resumen de k6 de la caída provocada

Generado por `pruebas/fallas/resumir.mjs` a partir de `k6.json`. Minuto 0 = primer pedido (2026-11-10T14:27:02.200Z). Horas en UTC. Incluye las reservas de `setup()` (minuto 0).

## Totales
| operación | 200 | 201 | 503 | total |
| --- | --- | --- | --- | --- |
| reservar | 0 | 169 | 197 | 366 |
| consultar | 361 | 0 | 0 | 361 |
| disponibilidad | 42 | 0 | 49 | 91 |

## Hitos de las reservas
| hito | hora (UTC) | duración |
| --- | --- | --- |
| primera reserva con 503 | 2026-11-10T14:30:06.417Z | 36 ms |
| primera reserva con 503 en menos de 100 ms (sin esperar el timeout) | 2026-11-10T14:30:06.417Z | 36 ms |
| última reserva con 503 | 2026-11-10T14:38:18.895Z | 5 ms |
| primera reserva con 201 después de la caída | 2026-11-10T14:38:16.424Z | 38 ms |

## Por minuto
| minuto | desde (UTC) | operación | 200 | 201 | 503 |
| --- | --- | --- | --- | --- | --- |
| 0 | 14:27:02 | reservar | 0 | 27 | 0 |
| 0 | 14:27:02 | consultar | 21 | 0 | 0 |
| 0 | 14:27:02 | disponibilidad | 6 | 0 | 0 |
| 1 | 14:28:02 | reservar | 0 | 24 | 0 |
| 1 | 14:28:02 | consultar | 24 | 0 | 0 |
| 1 | 14:28:02 | disponibilidad | 6 | 0 | 0 |
| 2 | 14:29:02 | reservar | 0 | 24 | 0 |
| 2 | 14:29:02 | consultar | 24 | 0 | 0 |
| 2 | 14:29:02 | disponibilidad | 6 | 0 | 0 |
| 3 | 14:30:02 | reservar | 0 | 1 | 23 |
| 3 | 14:30:02 | consultar | 24 | 0 | 0 |
| 3 | 14:30:02 | disponibilidad | 0 | 0 | 6 |
| 4 | 14:31:02 | reservar | 0 | 0 | 24 |
| 4 | 14:31:02 | consultar | 24 | 0 | 0 |
| 4 | 14:31:02 | disponibilidad | 0 | 0 | 6 |
| 5 | 14:32:02 | reservar | 0 | 0 | 24 |
| 5 | 14:32:02 | consultar | 24 | 0 | 0 |
| 5 | 14:32:02 | disponibilidad | 0 | 0 | 6 |
| 6 | 14:33:02 | reservar | 0 | 0 | 24 |
| 6 | 14:33:02 | consultar | 24 | 0 | 0 |
| 6 | 14:33:02 | disponibilidad | 0 | 0 | 6 |
| 7 | 14:34:02 | reservar | 0 | 0 | 24 |
| 7 | 14:34:02 | consultar | 24 | 0 | 0 |
| 7 | 14:34:02 | disponibilidad | 0 | 0 | 6 |
| 8 | 14:35:02 | reservar | 0 | 0 | 24 |
| 8 | 14:35:02 | consultar | 24 | 0 | 0 |
| 8 | 14:35:02 | disponibilidad | 0 | 0 | 6 |
| 9 | 14:36:02 | reservar | 0 | 0 | 24 |
| 9 | 14:36:02 | consultar | 24 | 0 | 0 |
| 9 | 14:36:02 | disponibilidad | 0 | 0 | 6 |
| 10 | 14:37:02 | reservar | 0 | 0 | 24 |
| 10 | 14:37:02 | consultar | 24 | 0 | 0 |
| 10 | 14:37:02 | disponibilidad | 0 | 0 | 6 |
| 11 | 14:38:02 | reservar | 0 | 18 | 6 |
| 11 | 14:38:02 | consultar | 24 | 0 | 0 |
| 11 | 14:38:02 | disponibilidad | 5 | 0 | 1 |
| 12 | 14:39:02 | reservar | 0 | 24 | 0 |
| 12 | 14:39:02 | consultar | 24 | 0 | 0 |
| 12 | 14:39:02 | disponibilidad | 6 | 0 | 0 |
| 13 | 14:40:02 | reservar | 0 | 24 | 0 |
| 13 | 14:40:02 | consultar | 24 | 0 | 0 |
| 13 | 14:40:02 | disponibilidad | 6 | 0 | 0 |
| 14 | 14:41:02 | reservar | 0 | 24 | 0 |
| 14 | 14:41:02 | consultar | 24 | 0 | 0 |
| 14 | 14:41:02 | disponibilidad | 6 | 0 | 0 |
| 15 | 14:42:02 | reservar | 0 | 3 | 0 |
| 15 | 14:42:02 | consultar | 4 | 0 | 0 |
| 15 | 14:42:02 | disponibilidad | 1 | 0 | 0 |

## Duración de las reservas por minuto y estado (ms, medida por k6)
| minuto | estado | pedidos | p50 | p95 | máximo |
| --- | --- | --- | --- | --- | --- |
| 0 | 201 | 27 | 28 | 72 | 84 |
| 1 | 201 | 24 | 22 | 42 | 43 |
| 2 | 201 | 24 | 20 | 36 | 37 |
| 3 | 201 | 1 | 37 | 37 | 37 |
| 3 | 503 | 23 | 11 | 31 | 36 |
| 4 | 503 | 24 | 7 | 16 | 31 |
| 5 | 503 | 24 | 7 | 24 | 31 |
| 6 | 503 | 24 | 7 | 20 | 25 |
| 7 | 503 | 24 | 8 | 25 | 32 |
| 8 | 503 | 24 | 6 | 29 | 29 |
| 9 | 503 | 24 | 6 | 28 | 33 |
| 10 | 503 | 24 | 7 | 27 | 42 |
| 11 | 201 | 18 | 21 | 38 | 38 |
| 11 | 503 | 6 | 7 | 17 | 17 |
| 12 | 201 | 24 | 20 | 28 | 32 |
| 13 | 201 | 24 | 19 | 31 | 31 |
| 14 | 201 | 24 | 19 | 24 | 33 |
| 15 | 201 | 3 | 26 | 28 | 28 |
