# Caída provocada de Operaciones (postmortem)

Scripts de la prueba de falla que pide la consigna (§2.9): se tira abajo Operaciones mientras hay tráfico real por el gateway y se registra qué vio el consumidor, qué detectó la observabilidad y cómo se recuperó. El análisis está en [`docs/POSTMORTEM.md`](../../docs/POSTMORTEM.md) y las salidas, en [`docs/evidencias/postmortem/`](../../docs/evidencias/postmortem/).

| Archivo | Qué hace |
| --- | --- |
| `caida-operaciones.js` | k6, 15 min por el gateway con el token del sistema externo: 0,4 reservas por segundo (cada una con su `Idempotency-Key`, sobre franjas de los próximos días), 0,4 consultas por segundo de `GET /traslados/{id}` de los traslados reservados al empezar y 0,1 consultas por segundo de `GET /disponibilidad` de mañana. 54 pedidos por minuto: debajo del límite del gateway (60). Guarda cada pedido en `k6.json` (`--out json`) con hora, operación, estado HTTP, clave y franja |
| `caida.sh` | Orquesta el procedimiento con los minutos fijos: k6 en el minuto 0, `docker compose stop operaciones` en el 3, consultas a Grafana cada 2 min mientras está caído, `docker compose start operaciones` en el 11, reintento de 3 reservas con `503` y, al terminar, logs y resumen. Anota la hora UTC de cada paso en `linea-temporal.txt` |
| `reintentar.mjs` | Reintenta por el gateway 3 reservas que dieron `503` con la **misma** `Idempotency-Key` y el mismo cuerpo, como indica el contrato |
| `resumir.mjs` | Cuenta los estados HTTP por minuto y por operación de `k6.json`, con la duración de las reservas y los hitos (primer y último `503`, primer `201` al volver), y escribe `resumen.md` |
| `logs.sh` | Saca del MySQL de logs, con el usuario de solo lectura de Grafana, los `WARN` y `ERROR` de Integración y del gateway, los cambios del breaker y los `INFO` de Operaciones al volver, entre el inicio y el fin de k6. Lo llama `caida.sh` |
| `verificar-reservas.sh` | Cruza las claves de k6 (y los reintentos) con MySQL de Operaciones: cada `201` tiene una sola fila de clave y una sola solicitud, y ninguna clave que recibió solo `503` dejó una reserva |

## Cómo correrlo
Requisitos: Docker y Node.js 22 (para `reintentar.mjs` y `resumir.mjs`). Se corre **en local**, nunca contra la nube.

1. En `.env`, activar el perfil de observabilidad (descomentar `COMPOSE_PROFILES=observabilidad`, `OTEL_EXPORTER_OTLP_ENDPOINT=http://lgtm:4318` y `LOGS_MYSQL_DSN=...`, como dice el `README.md`).
2. Levantar con cupo alto, sin modificar `compose.yaml` (como `pruebas/carga/README.md`), así el cupo de las franjas no corta las reservas con `409`. **Con volúmenes limpios:** el cupo se fija al crear cada franja en MySQL, así que las franjas que ya existían (por ejemplo, las que dejan los e2e) conservan el cupo por defecto, 4. En la corrida del postmortem, una franja que dejó llena `TestFranjaLlena` produjo `409` (ver `docs/POSTMORTEM.md`).
   ```bash
   docker compose down -v
   FRANJA_CUPO_TRASLADOS=100000 docker compose up --build -d --wait
   ```
3. Desde la raíz: `bash pruebas/fallas/caida.sh` (~16 min). Deja en `docs/evidencias/postmortem/`:
   - `linea-temporal.txt`: hora UTC de cada paso;
   - `k6.json` y `k6-salida.txt`: cada pedido y el resumen de k6;
   - `metricas.txt`: estado del circuito, 5xx por servicio y alertas, cada 2 min;
   - `reintentos.txt`: los 3 reintentos y las filas de `claves_idempotencia`;
   - `logs.txt`: `WARN` y `ERROR` de Integración y del gateway, e `INFO` de Operaciones al volver;
   - `resumen.md`: estados por minuto y por operación.
4. Con el sistema todavía levantado: `bash pruebas/fallas/verificar-reservas.sh > docs/evidencias/postmortem/reservas.txt`.
5. `docker compose down`.

Para guardar una corrida en **otra carpeta** (sin pisar las evidencias del postmortem), los dos scripts aceptan la variable `EVIDENCIAS`, que por defecto es `docs/evidencias/postmortem`:
```bash
EVIDENCIAS=docs/evidencias/caida-disponibilidad bash pruebas/fallas/caida.sh
EVIDENCIAS=docs/evidencias/caida-disponibilidad bash pruebas/fallas/verificar-reservas.sh > docs/evidencias/caida-disponibilidad/reservas.txt
```

Memoria: con observabilidad, el sistema usa cerca de 4 GB. Conviene cerrar otras aplicaciones y no compilar nada mientras corre.
