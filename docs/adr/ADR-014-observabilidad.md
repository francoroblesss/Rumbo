# ADR-014 — Observabilidad: logs, métricas, trazas, tablero y alerta (D11)

- **Fecha:** 2026-10-14
- **Estado:** aceptada (primera versión, Entrega 2) — el recorrido de la traza del flujo principal queda **reemplazado por [ADR-021](ADR-021-capacidad-de-autoservicio.md)** (la reserva va por HTTP a Operaciones, sin el comando). Los **logs** (Loki y el panel 11) quedan **reemplazados por [ADR-023](ADR-023-logs-en-mysql.md)**: se guardan en MySQL y Grafana los lee de ahí. El resto sigue vigente.

## Contexto
La consigna (§2.10) pide entender el estado de una operación **sin inspeccionar cada componente a mano**: como mínimo, **logs estructurados, métricas y un tablero**. La Entrega 2 exige logs **estructurados y correlacionados** y una **primera traza distribuida** del flujo principal. La Defensa exige logs, métricas y trazas completos, con el tablero y la alerta.

En clase se pidió:
- logs bien organizados y fáciles de leer;
- un lugar donde ver las métricas, por ejemplo "cuántos 404 hubo la semana pasada";
- un archivo con los logs y la reacción ante una caída (POSTMORTEM).

Del teórico 4: el `correlation_id` se propaga y se mide la profundidad de las colas, la DLQ y los reintentos. Del teórico 7: se miden los timeouts, los circuitos abiertos y los fallbacks. D11 pide también el **objetivo de servicio (SLO)** y los **criterios de alerta**.

## Decisión

### Stack
- **OpenTelemetry** como único estándar de instrumentación en los servicios Go: SDK oficial (`go.opentelemetry.io/otel`), `otelgin` para Gin, `otelhttp` para el gateway y los clientes HTTP, y el puente `otelslog` para los logs. Lo encapsula el módulo `servicios/comun/observabilidad` (`Iniciar`, `Instrumentar`, `InstrumentarHTTP`, `ClienteHTTP`).
- **Backend:** una sola imagen, **`grafana/otel-lgtm`**, que trae un colector OpenTelemetry (OTLP en 4317/4318), **Prometheus** (métricas), **Loki** (logs), **Tempo** (trazas) y **Grafana** (tableros y alertas) en `http://localhost:3000`, con sus datos en el volumen `lgtm-datos`. Es lo más simple de levantar y de demostrar, sin armar cuatro herramientas por separado.

### Logs
- `log/slog` en **JSON**: por stdout (se leen con `docker compose logs`) y a Loki por OTLP.
- Campos fijos: `servicio`, `instancia`, `correlation_id`, `trace_id` y `span_id`, más los específicos del caso (`docs/interfaces-internas.md` §7).
- Nunca se loguean secretos ni tokens.
- **Correlación:** el gateway asigna `X-Correlation-ID`, que viaja por HTTP y en los mensajes de RabbitMQ. Con un `correlation_id` o un `trace_id` se encuentran en Loki todos los logs de una operación, de todos los servicios.

### Métricas (nombres fijos en `docs/interfaces-internas.md` §8)
- **RED por servicio y por instancia:** `rumbo_http_duracion_segundos` (tasa, errores por `estado` y latencia).
- **Mensajería:** `rumbo_mensajes_procesados_total{cola,resultado}` (incluye la DLQ) y `rumbo_mensajes_publicados_total`.
- **Outbox:** `rumbo_outbox_pendientes{servicio}`.
- **Caché:** `rumbo_cache_operaciones_total{cache,resultado}`.
- **Resiliencia:** `rumbo_circuito_estado{dependencia}`, más `rumbo_gateway_rechazos_total{motivo}` (no autenticado, límite de tráfico, ruta interna).
- **Negocio:** `rumbo_solicitudes_total{origen,estado}` y `rumbo_clima_consultas_total{resultado}`.
- **Retención:** la del volumen de `lgtm`, que conserva al menos 7 días en local (para poder responder "qué pasó la semana pasada").

### Trazas
- Propagación **W3C Trace Context** por HTTP y **por RabbitMQ** (headers del mensaje), incluido el paso por el outbox y por las solicitudes pendientes de envío, donde se guarda el contexto de traza para que el relay continúe la traza.
- El **flujo principal** se ve en **una sola traza**:

  gateway → Integración (`POST /v2/traslados`) → `publicar integracion.solicitud_carga.recibida` → Operaciones `consumir …` → `publicar operaciones.solicitud.recibida` → Integración `consumir …`.

### Tablero "Rumbo — estado general" (Grafana, provisionado desde el repo)
1. Requests por segundo por servicio.
2. Tasa de errores 5xx por servicio.
3. Latencia p95 por servicio.
4. **Requests por instancia de Integración**: evidencia del balanceo (ADR-009).
5. Hit ratio de las cachés (`solicitudes` y `clima`).
6. Pendientes de outbox por servicio.
7. Mensajes procesados por resultado (ok, reintento, DLQ).
8. Estado de los circuitos (`operaciones`, `clima`).
9. Consultas de clima por resultado.
10. Rechazos del gateway por motivo.
11. Panel de logs de error (Loki) con su `correlation_id` y `trace_id`.

### Objetivo de servicio (SLO) y alertas
- **SLO de la capacidad publicada:**
  - **99 %** de los `POST /v2/traslados` responden en **menos de 300 ms**;
  - **menos de 1 %** de respuestas `5xx`;
  - ventana de 30 días.
- **Alertas** (reglas de Grafana, provisionadas desde el repo):
  - **`SLO latencia traslados`:** p95 de `POST /v2/traslados` mayor a 300 ms durante 5 min;
  - **`SLO errores traslados`:** tasa de `5xx` de `/v2/traslados` mayor a 1 % durante 5 min;
  - **`Mensajes en DLQ`:** algún mensaje enviado a una DLQ en los últimos 5 min (teórico 4: la DLQ tiene dueño y alerta).

## Alternativas consideradas
- **Prometheus, Loki, Tempo o Jaeger y Grafana por separado.** Descartada: mismo resultado con cuatro piezas más que configurar y levantar en cada notebook.
- **Solo `docker compose logs` y métricas en texto.** Descartada: no cumple "sin inspeccionar cada componente a mano" ni da trazas.
- **Instrumentación propia** (métricas y trazas escritas a mano). Descartada: OpenTelemetry es el estándar y la regla del grupo es no reinventar.

## Consecuencias
- **Ganamos:**
  - un solo lugar para logs, métricas, trazas, tablero y alertas;
  - correlación de punta a punta, incluida la mensajería;
  - evidencia del balanceo, la caché y el breaker para la defensa.
- **Pagamos:**
  - un contenedor pesado (`grafana/otel-lgtm`) en cada notebook;
  - `otel-lgtm` es pensado para desarrollo y demo: en producción se separarían las piezas y se definiría la retención.

## Cómo se verifica
- En Grafana → Explore → Tempo, la traza de un `POST /v2/traslados` muestra los spans de gateway, Integración y Operaciones, incluidos los de publicar y consumir.
- En Loki, filtrando por el `correlation_id` de esa request, aparecen los logs de los tres servicios.
- El tablero muestra las requests repartidas entre `integracion-1` e `integracion-2`.
- Forzar `5xx` hace que la alerta pase a *firing*.
- Evidencia en `docs/evidencias/entrega-2.md`.
