# ADR-035 — Integración reintenta sus llamadas a Operaciones (reemplaza parcialmente a ADR-004 y ADR-008)

- **Fecha:** <fecha del PR>
- **Estado:** aceptada
- **Reemplaza parcialmente a:** [ADR-004](ADR-004-comunicacion-entre-servicios.md), en la fila "Integración de carga → Operaciones" (2 s, sin reintento) y en la regla "las escrituras no se reintentan dentro del sistema" para esa llamada; y a [ADR-008](ADR-008-resiliencia.md), en el "No reintenta" del breaker de Integración → Operaciones. El breaker, el `503` con `Retry-After` y el reintento del consumidor con su `Idempotency-Key` siguen vigentes (con ADR-021).

## Contexto
- En la devolución de la Presentación Grupal, el profesor pidió ver **reintentos del lado del servidor** ("no solo el reintento del lado del cliente: backoff o timeout, retries") y, en general, más resiliencia.
- Desde ADR-021, la reserva es una llamada **síncrona** de Integración a Operaciones: si Operaciones no responde en ese momento, el consumidor recibe `503`. ADR-004 y ADR-008 decidieron que Integración no reintentara, para tener **un solo dueño del reintento** (el consumidor externo) y evitar la amplificación. Con eso, un corte de un segundo (un reinicio, una conexión perdida, un `503` puntual) se convertía en un `503` para el otro grupo.
- El teórico 7 pone al dueño del reintento en **la capa inmediatamente superior** a la dependencia que falla: para Operaciones, esa capa es Integración, no el consumidor. Y permite reintentar escrituras **con idempotencia**: los dos `POST` internos llevan la `Idempotency-Key` del consumidor, que Operaciones guarda en la misma transacción que la reserva o la cancelación (ADR-022, `interfaces-internas.md` §4.1).
- El deadline lo ponen las capas de arriba: el gateway (5 s) y NGINX (`proxy_read_timeout 5s`). La reserva tarda p95 ≤ 76,5 ms y p99 ≤ 139 ms hasta 100 reservas/s ([informe de la Presentación](../capacidad/informe-presentacion.md)).

## Decisión
- Integración reintenta **sus tres llamadas** a Operaciones (disponibilidad, reserva y cancelación) con `github.com/cenkalti/backoff/v5` (ya estaba en el módulo como dependencia de OpenTelemetry):
  - **1 s por intento** (antes 2 s), **3 intentos**, espera **exponencial** de 200 ms ×2 con **±50 % de jitter** (tope de 1 s por espera) y **4 s de tope total**, debajo de los 5 s del gateway y de NGINX;
  - **solo ante fallas transitorias:** error de red, timeout, `502`, `503` o `504`;
  - **no** se reintenta un `4xx` (respuesta del negocio), un `500` (un error de programación no se arregla reintentando), con el **breaker abierto** ni con el contexto cancelado;
  - cada intento pasa por el breaker (como el adaptador de clima de ADR-012) y los `POST` repiten la **misma** `Idempotency-Key`.
- Si se agotan los intentos, la respuesta no cambia: `503 SERVICIO_NO_DISPONIBLE` con `Retry-After: 30`, y el consumidor puede reintentar con su misma clave.
- Cada reintento se cuenta en **`rumbo_reintentos_total{dependencia, operacion}`** y se loguea en `WARN` ("reintento a Operaciones"). El reintento que ya hacía el adaptador de clima se cuenta con la misma métrica (ADR-034), y Grafana los muestra juntos.

## Alternativas consideradas
- **Seguir sin reintento en Integración.** Descartada: el corte más breve de Operaciones llega al otro grupo como `503`, y el reintento queda dos capas lejos de la dependencia que falla.
- **Reintentar solo la disponibilidad (la lectura).** Descartada: la reserva es la operación que más importa y ya es idempotente; dejarla afuera no reduce ningún riesgo.
- **Reintentar también en el gateway.** Descartada: suma una capa más de reintentos (amplificación, teórico 7) y el gateway no sabe qué operaciones son seguras de repetir.
- **Escribir el backoff a mano** (como el adaptador de clima, que hace un solo reintento con jitter). Descartada: con más de un reintento, tope total y clasificación, es justo lo que resuelve una librería (no reinventar la rueda).

## Consecuencias
- **Ganamos:** un corte breve de Operaciones se absorbe adentro de Rumbo y el consumidor no lo ve; el reintento queda en la capa que corresponde, con métricas y logs propios.
- **Pagamos:**
  - con Operaciones caída del todo, cada llamada tarda hasta ~1 s más antes del `503` (los reintentos con espera), hasta que el breaker se abre: con 3 intentos por llamada, se abre en ~2 llamadas en vez de 5, y desde ahí el `503` es inmediato;
  - la amplificación pasa a ser 3 (Integración) × los reintentos del consumidor, acotada por el breaker y por el tope de 4 s;
  - un timeout de 1 s deja menos margen si Operaciones se vuelve lenta (más de 1 s por reserva); hoy el p99 está más de 7 veces por debajo.
  - si el reintento de una reserva llega mientras el pedido original todavía se está procesando, Operaciones puede responder un error en vez del `201` original; no se duplica la reserva (la clave es única y se guarda en la misma transacción) y el consumidor lo resuelve reintentando con la misma clave.

## Cómo se verifica
- Tests de `clients` (con `httptest`): `503` y después `201` con la misma clave y el mismo cuerpo; timeout y después `200`; tres `503` agotan los intentos antes del tope; `4xx`, `500` y contexto cancelado no se reintentan; el breaker sigue abriéndose con 5 fallas: <resultado>.
- Prueba con el sistema real (`pruebas/fallas/reintentos.sh`, evidencia en `docs/evidencias/reintentos-integracion/`): tres congelamientos de Operaciones de 1,5 s con tráfico por el gateway. <reintentos por operación, `503` que vio el consumidor y resultado de `verificar-reservas.sh`, con fecha>.
