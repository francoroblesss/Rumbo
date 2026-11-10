# Resumen de k6 de la caída provocada

Generado por `pruebas/fallas/resumir.mjs` a partir de `k6.json`. Minuto 0 = primer pedido (2026-11-10T13:50:10.885Z). Horas en UTC. Incluye las reservas de `setup()` (minuto 0).

## Totales
| operación | 200 | 201 | 502 | 503 | total |
| --- | --- | --- | --- | --- | --- |
| reservar | 0 | 175 | 31 | 161 | 367 |
| consultar | 335 | 0 | 26 | 0 | 361 |
| disponibilidad | 43 | 0 | 33 | 15 | 91 |

## Hitos de las reservas
| hito | hora (UTC) | duración |
| --- | --- | --- |
| primera reserva con 503 | 2026-11-10T13:53:15.124Z | 108 ms |
| primera reserva con 503 en menos de 100 ms (sin esperar el timeout) | 2026-11-10T13:53:17.535Z | 18 ms |
| última reserva con 503 | 2026-11-10T14:01:10.030Z | 11 ms |
| primera reserva con 201 después de la caída | 2026-11-10T14:01:15.047Z | 28 ms |

## Por minuto
| minuto | desde (UTC) | operación | 200 | 201 | 502 | 503 |
| --- | --- | --- | --- | --- | --- | --- |
| 0 | 13:50:10 | reservar | 0 | 27 | 0 | 0 |
| 0 | 13:50:10 | consultar | 21 | 0 | 0 | 0 |
| 0 | 13:50:10 | disponibilidad | 6 | 0 | 0 | 0 |
| 1 | 13:51:10 | reservar | 0 | 24 | 0 | 0 |
| 1 | 13:51:10 | consultar | 24 | 0 | 0 | 0 |
| 1 | 13:51:10 | disponibilidad | 6 | 0 | 0 | 0 |
| 2 | 13:52:10 | reservar | 0 | 24 | 0 | 0 |
| 2 | 13:52:10 | consultar | 24 | 0 | 0 | 0 |
| 2 | 13:52:10 | disponibilidad | 6 | 0 | 0 | 0 |
| 3 | 13:53:10 | reservar | 0 | 1 | 0 | 23 |
| 3 | 13:53:10 | consultar | 24 | 0 | 0 | 0 |
| 3 | 13:53:10 | disponibilidad | 0 | 0 | 1 | 5 |
| 4 | 13:54:10 | reservar | 0 | 0 | 9 | 15 |
| 4 | 13:54:10 | consultar | 16 | 0 | 8 | 0 |
| 4 | 13:54:10 | disponibilidad | 0 | 0 | 3 | 3 |
| 5 | 13:55:10 | reservar | 0 | 0 | 4 | 20 |
| 5 | 13:55:10 | consultar | 19 | 0 | 5 | 0 |
| 5 | 13:55:10 | disponibilidad | 0 | 0 | 5 | 1 |
| 6 | 13:56:10 | reservar | 0 | 0 | 9 | 15 |
| 6 | 13:56:10 | consultar | 16 | 0 | 8 | 0 |
| 6 | 13:56:10 | disponibilidad | 0 | 0 | 4 | 2 |
| 7 | 13:57:10 | reservar | 0 | 0 | 4 | 20 |
| 7 | 13:57:10 | consultar | 20 | 0 | 4 | 0 |
| 7 | 13:57:10 | disponibilidad | 0 | 0 | 3 | 3 |
| 8 | 13:58:10 | reservar | 0 | 0 | 0 | 24 |
| 8 | 13:58:10 | consultar | 23 | 0 | 1 | 0 |
| 8 | 13:58:10 | disponibilidad | 0 | 0 | 5 | 1 |
| 9 | 13:59:10 | reservar | 0 | 0 | 2 | 22 |
| 9 | 13:59:10 | consultar | 24 | 0 | 0 | 0 |
| 9 | 13:59:10 | disponibilidad | 0 | 0 | 6 | 0 |
| 10 | 14:00:10 | reservar | 0 | 0 | 2 | 22 |
| 10 | 14:00:10 | consultar | 24 | 0 | 0 | 0 |
| 10 | 14:00:10 | disponibilidad | 0 | 0 | 6 | 0 |
| 11 | 14:01:10 | reservar | 0 | 23 | 1 | 0 |
| 11 | 14:01:10 | consultar | 24 | 0 | 0 | 0 |
| 11 | 14:01:10 | disponibilidad | 6 | 0 | 0 | 0 |
| 12 | 14:02:10 | reservar | 0 | 24 | 0 | 0 |
| 12 | 14:02:10 | consultar | 24 | 0 | 0 | 0 |
| 12 | 14:02:10 | disponibilidad | 6 | 0 | 0 | 0 |
| 13 | 14:03:10 | reservar | 0 | 24 | 0 | 0 |
| 13 | 14:03:10 | consultar | 24 | 0 | 0 | 0 |
| 13 | 14:03:10 | disponibilidad | 6 | 0 | 0 | 0 |
| 14 | 14:04:10 | reservar | 0 | 24 | 0 | 0 |
| 14 | 14:04:10 | consultar | 24 | 0 | 0 | 0 |
| 14 | 14:04:10 | disponibilidad | 6 | 0 | 0 | 0 |
| 15 | 14:05:10 | reservar | 0 | 4 | 0 | 0 |
| 15 | 14:05:10 | consultar | 4 | 0 | 0 | 0 |
| 15 | 14:05:10 | disponibilidad | 1 | 0 | 0 | 0 |

## Duración de las reservas por minuto y estado (ms, medida por k6)
| minuto | estado | pedidos | p50 | p95 | máximo |
| --- | --- | --- | --- | --- | --- |
| 0 | 201 | 27 | 22 | 51 | 63 |
| 1 | 201 | 24 | 28 | 50 | 65 |
| 2 | 201 | 24 | 26 | 39 | 40 |
| 3 | 201 | 1 | 21 | 21 | 21 |
| 3 | 503 | 23 | 11 | 32 | 108 |
| 4 | 502 | 9 | 5 | 22 | 22 |
| 4 | 503 | 15 | 9 | 30 | 30 |
| 5 | 502 | 4 | 2 | 8 | 8 |
| 5 | 503 | 20 | 8 | 20 | 26 |
| 6 | 502 | 9 | 4 | 12 | 12 |
| 6 | 503 | 15 | 9 | 94 | 94 |
| 7 | 502 | 4 | 4 | 5 | 5 |
| 7 | 503 | 20 | 8 | 25 | 31 |
| 8 | 503 | 24 | 7 | 29 | 45 |
| 9 | 502 | 2 | 1 | 6 | 6 |
| 9 | 503 | 22 | 7 | 23 | 27 |
| 10 | 502 | 2 | 2 | 10 | 10 |
| 10 | 503 | 22 | 6 | 20 | 30 |
| 11 | 201 | 23 | 23 | 51 | 55 |
| 11 | 502 | 1 | 8 | 8 | 8 |
| 12 | 201 | 24 | 20 | 31 | 40 |
| 13 | 201 | 24 | 22 | 43 | 100 |
| 14 | 201 | 24 | 19 | 36 | 48 |
| 15 | 201 | 4 | 18 | 26 | 26 |
