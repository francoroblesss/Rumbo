# Pruebas de carga (ADR-015) y medición de D5 (ADR-016)

Scripts de [k6](https://grafana.com/docs/k6/) que miden la capacidad publicada "Solicitar traslado" con el método de [ADR-015](../../docs/adr/ADR-015-capacidad-y-costos.md). Los resultados y su análisis están en [`docs/capacidad/informe-entrega-2.md`](../../docs/capacidad/informe-entrega-2.md).

| Archivo | Qué hace |
| --- | --- |
| `traslados.js` | Escalones de tasa constante de 1, 2, 5, 10, 20, 50 y 100 req/s, de 30 s cada uno. Mezcla: 90 % `GET /traslados/{id}` y 10 % `POST /traslados`. Escribe `resultado-<E>.json` con p95, p99, % de errores y req/s logradas por escalón |
| `d5.js` | Genera llamadas salientes para ADR-016 durante el escalón de 10 req/s de C: cancelaciones (Integración → Operaciones), aceptaciones al aeropuerto (Operaciones → clima) y consultas directas al mock del clima |
| `ejecutar.sh` | Corre los escenarios **A** (1 réplica, sin Memcached), **B** (1 réplica, con Memcached) y **C** (2 réplicas, con Memcached), en ese orden. Guarda `docker stats` cada 5 s en `stats-<E>.txt` |
| `resumir.mjs` | Cruza `resultado-<E>.json` con `stats-<E>.txt` y arma las tablas del informe, aplicando la regla del límite de ADR-015 |
| `latencias-d5.mjs` | Lee de Tempo y Prometheus (a través de Grafana) el p50 y el p99 de las llamadas salientes de la ventana de `d5.js` |

## Cómo correrlo
Requisitos: Docker y Node.js (solo para `resumir.mjs` y `latencias-d5.mjs`). La observabilidad tiene que estar activa (`COMPOSE_PROFILES=observabilidad` y `OTEL_EXPORTER_OTLP_ENDPOINT=http://lgtm:4318` en `.env`) para medir D5.

```bash
docker compose up --build -d          # esperar a que todo esté sano (docker compose ps)
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
