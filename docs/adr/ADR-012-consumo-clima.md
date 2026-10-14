# ADR-012 — Consumo de la capacidad de clima del sistema de aeropuerto (D9)

- **Fecha:** 2026-10-14
- **Estado:** aceptada

## Contexto
La consigna (§4.1) pide consumir una capacidad publicada por otro grupo:
- **dentro de un flujo importante** del negocio, con consecuencias (no una llamada aislada);
- **desde el microservicio responsable**, no desde el frontend ni el gateway;
- respetando el contrato publicado;
- con un **test de contrato**;
- con timeouts y tratamiento de errores;
- definiendo **qué ve el usuario cuando el proveedor falla**.

El grupo del sistema de aeropuerto publicó **API de Clima v1** (`clima-v1.yaml`, 1.0.0, commit `d9113be` de `MaxiBarella/Proyecto-arquitectura`). Una sola operación, `GET /v1/airports/{icao}/conditions`:
- devuelve la **observación actual** del aeropuerto de Córdoba (`SACO`) y su **operabilidad** (`operable` y `reasons`: `HIGH_WIND`, `HIGH_GUSTS`, `LOW_VISIBILITY`, `THUNDERSTORM`);
- se autentica con `X-API-Key`, tiene un límite de 60 pedidos por minuto y devuelve errores Problem Details (`code`);
- es de solo lectura, así que se puede reintentar.

El dato es **actual, no un pronóstico**: sirve para decisiones del mismo día.

## Decisión

### Uso en el negocio (nueva regla de dominio, `dominio.md` §8 y `SPEC.md` R-41)
- Una ubicación de un traslado se puede marcar como el **aeropuerto** (`SACO`). En esta etapa la marca el **operador al aceptar** la solicitud (`aeropuerto_extremo`: `ORIGEN` o `DESTINO`).
- Para un traslado con origen o destino en el aeropuerto cuya hora comprometida esté **dentro de las próximas 6 h**, Operaciones consulta la operabilidad:
  1. **al aceptar la solicitud** (implementado en esta entrega);
  2. **al iniciar el recorrido** (cuando exista la ejecución de recorridos).
- **Si el aeropuerto no es operable:** el traslado queda con una **alerta climática** (`NO_OPERABLE` y los motivos). Se publica `operaciones.traslado.alerta_climatica` (aviso al operador y al cliente) y **el operador decide**: mantener, proponer otro horario o cancelar. Rumbo no cancela ni reprograma solo, porque nunca cambia un compromiso sin el cliente (`dominio.md` §6).
- **Si el proveedor falla** (timeout, error, límite superado o breaker abierto): **se sigue sin el dato**. La alerta queda `NO_VERIFICADO` y el operador ve "clima no verificado". Es **fail-open** explícito (teórico 7): un dato informativo no puede frenar la operación; la decisión sigue siendo del operador.

### Integración técnica (en Operaciones)
- **Puerto de salida** `ConsultaClima.Operabilidad(ctx, icao) (OperabilidadAeropuerto, error)` en `puertos/`. El **adaptador** `adaptadores/clima` traduce el contrato del proveedor a nuestro modelo (`operable`, `motivos`, `observado_en`). Así el dominio no conoce su JSON ni sus códigos, que es lo que pide un contrato que no controlamos (teórico 8).
- **Llamada:** `GET {CLIMA_URL}/v1/airports/{CLIMA_ICAO}/conditions` con `X-API-Key: {CLIMA_API_KEY}`. La clave va en `.env` y nunca en el repo, como pide el proveedor.
- **Timeout:** **1 s** por intento. **Reintento:** **1**, con espera aleatoria de 100 a 300 ms, solo ante timeout, error de red o `5xx`. Ante `429` o `503` se reintenta solo si `Retry-After` es de 1 s o menos; si no, se considera no disponible. **Nunca** se reintenta un `4xx` (`400`, `401`, `404`).
- **Circuit breaker** (`github.com/sony/gobreaker/v2`, dependencia `clima`): se abre con 5 fallas consecutivas y prueba de nuevo a los 30 s.
- **Caché local de 5 min** (`github.com/karlseguin/ccache/v3`, como en la clase práctica 3), con clave igual al ICAO. Sirve para no pasar el límite de 60 pedidos por minuto y para no consultar dos veces el mismo dato reciente. Es local porque Operaciones corre en una sola réplica y el dato es igual para todos.
- **Compatibilidad:** se ignoran los campos desconocidos. Un motivo o código desconocido se trata como genérico, como pide el contrato del proveedor.
- **Observabilidad:**
  - `rumbo_clima_consultas_total{resultado="operable|no_operable|no_verificado"}`;
  - `rumbo_circuito_estado{dependencia="clima"}`;
  - `rumbo_cache_operaciones_total{cache="clima"}`;
  - la llamada aparece como span hijo dentro de la traza de la aceptación.
- **Contrato fijado:** la copia en `docs/contracts/consumidos/clima-v1.yaml` es contra lo que se programa y testea. En desarrollo, Compose levanta su mock (`clima-mock`, Prism).
- **Test de contrato:** valida con `kin-openapi` que el pedido que arma el adaptador (ruta, método y header) cumple el contrato, y que el adaptador interpreta bien los ejemplos del contrato (`operable` y `not_operable`) y sus errores (`401`, `404`, `429`, `503`). Si el proveedor publica un cambio incompatible, el test falla al actualizar la copia.

## Alternativas consideradas
- **Bloquear la aceptación si el aeropuerto no es operable** (fail-closed). Descartada: la consecuencia de negocio la decide el operador, y el dato es el clima actual, no el de la hora del vuelo.
- **Consultar el clima en cada búsqueda o desde el frontend.** Descartada: no es un flujo con consecuencias y la consigna exige que la llamada salga del microservicio responsable.
- **Usar el clima para todas las solicitudes.** Descartada: solo afecta a los traslados al o desde el aeropuerto.
- **Caché distribuida para el clima.** Descartada: Operaciones tiene una sola réplica y el dato no depende del usuario. Una caché local alcanza (teórico 3).

## Consecuencias
- **Ganamos:**
  - la capacidad del otro grupo tiene una consecuencia real en la operación (la alerta y la decisión del operador);
  - si el proveedor cae, se puede seguir operando;
  - cambiar de proveedor toca solo el adaptador.
- **Pagamos:**
  - la decisión queda en manos del operador, no es automática;
  - un dato con hasta 5 min de antigüedad (caché);
  - la segunda consulta (al iniciar el recorrido) queda pendiente hasta que exista la ejecución de recorridos.

## Cómo se verifica
- **Tests del adaptador** con el contrato: mapeo de `operable` y `not_operable`; `401` y `404` sin reintento; `503` con un reintento; timeout → `NO_VERIFICADO`; breaker abierto después de 5 fallas.
- **Test del caso de uso:** aceptar una solicitud al aeropuerto dentro de las 6 h con el aeropuerto no operable deja la alerta `NO_OPERABLE` y publica `operaciones.traslado.alerta_climatica`; con el proveedor caído, la aceptación funciona igual y queda `NO_VERIFICADO`.
- **Caída provocada:** con `docker compose stop clima-mock`, aceptar sigue funcionando y el traslado queda "clima no verificado".
