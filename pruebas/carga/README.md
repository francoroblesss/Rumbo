# Pruebas de carga (ADR-015), medición de D5 (ADR-016) y reservas sobre la misma franja (ADR-022)

Scripts de [k6](https://grafana.com/docs/k6/) que miden la capacidad publicada "Solicitar traslado" con el método de [ADR-015](../../docs/adr/ADR-015-capacidad-y-costos.md). Los resultados y su análisis están en [`docs/capacidad/informe-entrega-2.md`](../../docs/capacidad/informe-entrega-2.md).

| Archivo | Qué hace |
| --- | --- |
| `traslados.js` | Escalones de tasa constante de 1, 2, 5, 10, 20, 50 y 100 req/s, de 30 s cada uno. Mezcla: 90 % `GET /traslados/{id}` y 10 % `POST /traslados`. Escribe `resultado-<E>.json` con p95, p99, % de errores y req/s logradas por escalón |
| `d5.js` | Genera llamadas salientes para ADR-016 durante el escalón de 10 req/s de C: cancelaciones (Integración → Operaciones), aceptaciones al aeropuerto (Operaciones → clima) y consultas directas al mock del clima |
| `ejecutar.sh` | Corre los escenarios **A** (1 réplica, sin Memcached), **B** (1 réplica, con Memcached) y **C** (2 réplicas, con Memcached), en ese orden. Guarda `docker stats` cada 5 s en `stats-<E>.txt` |
| `resumir.mjs` | Cruza `resultado-<E>.json` con `stats-<E>.txt` y arma las tablas del informe, aplicando la regla del límite de ADR-015 |
| `latencias-d5.mjs` | Lee de Tempo y Prometheus (a través de Grafana) el p50 y el p99 de las llamadas salientes de la ventana de `d5.js` |
| `misma-franja.js` | Reservas simultáneas (`POST /traslados`) en escalones de 10, 25, 50 y 100 reservas/s (`ramping-arrival-rate`, 5 s de rampa y 30 s de meseta). `MODO=misma`: todas sobre una franja; `MODO=repartidas`: control, rotando entre 173 franjas. Escribe `resultado-<modo>.json` |
| `ejecutar-misma-franja.sh` | Corre los dos modos. Guarda los contadores de esperas por bloqueo de InnoDB cada 2 s en `bloqueos-<modo>.txt` y la ocupación de la franja antes y después en `verificacion-misma.json` |
| `resumir-misma-franja.mjs` | Arma la tabla del [informe de la Presentación](../../docs/capacidad/informe-presentacion.md) y verifica que la franja no se sobrerreservó |

## Cómo correrlo
Requisitos: Docker y Node.js (solo para `resumir.mjs` y `latencias-d5.mjs`). La observabilidad tiene que estar activa (`COMPOSE_PROFILES=observabilidad` y `OTEL_EXPORTER_OTLP_ENDPOINT=http://lgtm:4318` en `.env`) para medir D5.

```bash
FRANJA_CUPO_TRASLADOS=100000 docker compose up --build -d   # cupo alto: si no, las reservas se cortan con 409; esperar a que todo esté sano (docker compose ps)
bash pruebas/carga/ejecutar.sh        # ~12 min: A, B y C (y d5.js durante C)
node pruebas/carga/resumir.mjs        # tablas por escenario y uso por contenedor para el costo
node pruebas/carga/latencias-d5.mjs   # p50 y p99 de las llamadas salientes (ADR-016)
```

Al terminar, el sistema queda en el escenario C (todo levantado).

- **Destino:** `http://balanceador-integracion`, dentro de la red de Docker (`<proyecto>_default`; se puede forzar con `RED=...`). Se mandan los headers de identidad que pondría el gateway (`X-Rumbo-Organizacion: org-almacenamiento`, `X-Rumbo-Roles: sistema_externo`). No se pasa por el gateway porque su límite de 60 pedidos por minuto cortaría la prueba.
- **Windows (Git Bash):** el script exporta `MSYS_NO_PATHCONV=1` y usa `pwd -W` para que Docker reciba bien la ruta del volumen.
- **CPU por escalón:** `resumir.mjs` usa la **mediana** de las muestras de cada escalón, porque los healthchecks de `mongo` (`mongosh`) y `rabbitmq` (`rabbitmq-diagnostics`) producen picos de 60 % a 180 % de un núcleo cada pocos segundos, sin relación con la carga. Para el costo usa el **promedio**, porque el proveedor también cobra esos picos.
- **Otros contenedores:** el muestreo solo lee los contenedores de este proyecto (`docker compose ps -q`). Conviene cerrar otros proyectos de Docker mientras se mide, porque compiten por la misma CPU.
- Las salidas `k6-*.log` no se versionan (son la barra de progreso de k6). Los `resultado-*.json` y `stats-*.txt` de la medición del informe sí se versionan, como evidencia.

## Reservas simultáneas sobre la misma franja (Presentación)
Mide cuánto pesa la serialización de la reserva por el `SELECT … FOR UPDATE` de la franja ([ADR-022](../../docs/adr/ADR-022-consistencia-y-concurrencia.md), D4). Resultados en [`docs/capacidad/informe-presentacion.md`](../../docs/capacidad/informe-presentacion.md). **Solo en local**: nunca contra la nube ([ADR-026](../../docs/adr/ADR-026-despliegue-en-render.md)).

```bash
# Cupo alto en la franja (traslados y kg), sin tocar compose.yaml. Basta con lo que usa la reserva.
FRANJA_CUPO_TRASLADOS=100000 FRANJA_CUPO_KG=100000000 docker compose up --build -d --wait gateway memcached
bash pruebas/carga/ejecutar-misma-franja.sh   # ~6 min: modo misma y después repartidas
node pruebas/carga/resumir-misma-franja.mjs   # tabla por escalón y verificación de la franja
```

- **Franja:** por defecto, la de 10 h de dentro de 3 días. Se fuerza con `FRANJA=2026-11-20T10:00:00-03:00`. Cada fila de `franjas` guarda el cupo con el que se creó: si la franja ya existe con el cupo por defecto (4), el script se niega a correr. Las franjas del modo `repartidas` también tienen que ser nuevas (o tener cupo alto): conviene correrla sobre una base sin datos de demo.
- **Destino:** igual que `traslados.js`, `http://balanceador-integracion` desde un contenedor de k6 en la red de Compose, porque el límite del gateway (60 pedidos por minuto) cortaría la prueba. En `setup()` se pide **un** token al gateway (`POST http://gateway:8080/auth/token` con `EXTERNO_CLIENT_ID` y `EXTERNO_CLIENT_SECRET` de `.env`) y de sus claims salen los headers de identidad que pondría el gateway (`X-Rumbo-Usuario`, `X-Rumbo-Organizacion`, `X-Rumbo-Roles`). Se agrega `X-Rumbo-Interno` con `SECRETO_INTERNO` de `.env`: sin él, Integración responde `401` (ADR-026).
- **Bloqueos:** contadores `Innodb_row_lock_waits`, `Innodb_row_lock_time` e `Innodb_row_lock_current_waits` (`SHOW GLOBAL STATUS`, con el usuario root de MySQL), cada 2 s. Se eligieron frente a `SHOW ENGINE INNODB STATUS` porque acumulan las esperas y su tiempo (se resta antes y después de cada meseta), mientras que ese comando solo muestra las esperas del instante. `performance_schema` está apagado en `compose.yaml` para ahorrar memoria.
- **Verificación:** al final del modo `misma`, `traslados_reservados` de la franja tiene que haber subido exactamente la cantidad de `201` (contador `reservas_201` de k6).
