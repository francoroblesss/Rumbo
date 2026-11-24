# ADR-036 — Ajustes de observabilidad y degradación para el final (reemplaza parcialmente a ADR-007)

- **Fecha:** 23/11/2026
- **Estado:** aceptada
- **Reemplaza parcialmente a:** [ADR-007](ADR-007-cache.md), en el guardado después de una lectura fallida: si `Get` falló, ya no se intenta el `Set`. El resto de ADR-007 (cache-aside, TTL con jitter, invalidación por evento, degradación a MongoDB) sigue vigente. Completa a [ADR-035](ADR-035-reintento-integracion-operaciones.md) y a [ADR-014](ADR-014-observabilidad.md) sin reemplazarlos.

## Contexto
Al verificar los PR del final con el sistema real ([`evidencias/observabilidad-final/`](../evidencias/observabilidad-final/README.md) y [`evidencias/reintentos-integracion/`](../evidencias/reintentos-integracion/README.md)) aparecieron cuatro problemas que se ven justo en lo que el profesor pidió mirar en Grafana:

1. **`503` "fantasma" de Operaciones.** Cuando Integración corta un intento por su timeout de 1 s y lo reintenta (ADR-035), el pedido original sigue en Operaciones; al terminar con el contexto cancelado, Operaciones lo trataba como falla de infraestructura: log `ERROR` y `503`. Nadie recibía ese `503`, pero el panel 2 (% de `5xx`) mostró picos de ~50 % y la alerta `SLO errores traslados` podría dispararse por una caída que no existió.
2. **Memcached caído pagaba dos timeouts.** Con Memcached detenido, la consulta por id tardaba ~242 ms: 100 ms del `Get`, la lectura de MongoDB y otros 100 ms del `Set`, que iba a fallar igual.
3. **Contadores que nacen con su primer valor.** Una serie de Prometheus que aparece con el valor 1 no tiene muestra anterior, e `increase()` no cuenta ese primer evento. ADR-035 ya lo resolvió para `rumbo_reintentos_total` de Integración; seguían afectados el reintento del clima (panel 18), los logs descartados (panel 13) y las consultas de clima (panel 9): justo los eventos raros, que son los que importa ver.
4. **Logs perdidos al arrancar con volúmenes nuevos.** El healthcheck de `mysql-logs` usaba `mysqladmin ping -h localhost`, que va por el socket: durante la inicialización, la imagen de MySQL levanta un servidor temporal **sin red**, el ping da "sano" y los servicios arrancan antes de que el puerto 3306 acepte conexiones. Se perdieron 6 logs en la corrida de Delap.

## Decisión
1. **Pedido abandonado = `499`, no `503`.** En el mapeo de errores de la API de Operaciones, si el contexto del pedido ya está cancelado, se registra `499` (la convención de NGINX, "Client Closed Request") y un log `WARN` ("el que llamó cortó antes de la respuesta"). La transacción no se confirmó, así que el reintento con la misma `Idempotency-Key` la hace una sola vez (ADR-022).
2. **Sin `Set` después de un `Get` fallido.** Si leer de Memcached falló, se lee de MongoDB y se responde sin intentar guardar; la próxima lectura vuelve a probar Memcached. Si el `Get` funcionó pero el valor no se pudo decodificar, sí se guarda (para pisar el valor roto).
3. **Toda serie de contador arranca en 0** al crear el contador, con sus atributos conocidos: `rumbo_reintentos_total{dependencia="clima"}`, `rumbo_logs_descartados_total{motivo}` (`cola_llena`, `mysql`), `rumbo_clima_consultas_total{momento, resultado}` y `rumbo_gateway_rechazos_total{motivo}` (`no_autenticado`, `limite`, `interna`: así un `401` suelto de la demo se ve en el panel 10).
4. **Healthcheck de MySQL por TCP** (`-h 127.0.0.1`) en `mysql` y en `mysql-logs`: da "sano" recién cuando el servidor definitivo acepta conexiones por red, que es como se conectan los servicios.

## Alternativas consideradas
- **Para 1, que Integración no corte el intento** (timeout más largo). Descartada: es el timeout por intento el que permite reintentar dentro de los 4 s (ADR-035).
- **Para 1, seguir respondiendo `503` y filtrarlo en Grafana.** Descartada: el `503` también se cuenta en la alerta y en el log `ERROR`; corregir el dato en el origen es más simple que excepciones en cada consulta.
- **Para 2, un breaker para Memcached.** Descartado por ahora: con un timeout de 100 ms y sin `Set`, una caída cuesta ~1 timeout por consulta; un breaker agregaría estado y configuración para ahorrar esos 100 ms.
- **Para 3, cambiar las consultas del tablero** (`increase()` con `or` sobre la serie anterior). Descartada: hay que repetirlo en cada panel y alerta; inicializar la serie lo resuelve una vez.

## Consecuencias
- **Ganamos:** el panel 2 y la alerta de errores muestran solo fallas reales; con Memcached caído la consulta paga un timeout en vez de dos; los paneles 9, 10, 13 y 18 cuentan el primer evento; los logs del arranque llegan a MySQL.
- **Pagamos:**
  - un error real de infraestructura que ocurre justo en un pedido cuyo cliente ya cortó queda como `WARN` y `499`; los pedidos que siguen esperando lo siguen mostrando como `ERROR` y `503`, e Integración loguea su propio `WARN` del reintento;
  - `499` no es un código estándar de HTTP: es solo para la métrica y el log (nadie lo recibe);
  - con Memcached caído no se vuelve a llenar la caché hasta que una lectura funcione (la primera lectura buena es un miss y guarda).

## Cómo se verifica
- Tests: `TestClienteQueSeFueNoEsUn503` (API de Operaciones: contexto cancelado → `499`) y `TestCacheCaidaDegradaAMongo` (con la caché caída no hay ningún `Set`). `go test ./...` de `comun`, gateway, Operaciones e Integración en verde (23/11/2026).
- Con el sistema levantado (ensayo del final): `pruebas/fallas/reintentos.sh` sin picos de `5xx` en el panel 2 de Operaciones; `pruebas/carga/cache-encendida-apagada.sh` con la consulta con Memcached caído cerca de 120 ms en el panel 16 (antes ~242 ms); `docker compose down -v && docker compose up -d --wait` y en el panel 14 los logs del arranque de todos los servicios.
