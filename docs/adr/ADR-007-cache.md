# ADR-007 — Caché en las consultas de traslados del contrato público (D7)

- **Fecha:** 2026-10-14
- **Estado:** aceptada

## Contexto
La consigna (§2.7) pide caché en **al menos un flujo de lectura relevante**, elegida por una **necesidad observable** y con **métricas que muestren su impacto**. El profesor pidió además no implementar cachés propias: usar las herramientas vistas en clase.

El flujo de lectura más repetido de Rumbo en esta etapa es el **seguimiento del contrato público** (ADR-002, ADR-006). El sistema consumidor consulta cada solicitud (`GET /v2/traslados/{id}`) muchas veces mientras espera que cambie de estado, pero el estado cambia pocas veces: recibida, aceptada, en curso, completada. La mayoría de esas lecturas devuelven exactamente lo mismo que la anterior. Además, Integración de carga corre con **dos réplicas** detrás de NGINX (ADR-009).

Las 7 preguntas del teórico 3:
1. **Operación costosa y fuente de verdad:** la lectura de la solicitud externa en MongoDB (base `integracion`). La fuente de verdad del negocio es Operaciones; Integración guarda una copia.
2. **Quién la necesita y cuánto se repite:** el sistema consumidor, con polling (hasta una consulta por minuto por solicitud y por organización, más los reintentos).
3. **Cuánta desactualización se tolera:** pocos segundos. El contrato ya acepta consistencia eventual: el estado llega por eventos.
4. **Qué invalida la clave:** cada evento de Operaciones que cambia la solicitud y cada escritura propia (respuesta a contrapropuesta, cancelación).
5. **Patrón:** cache-aside.
6. **Ante miss, expiración, evicción o caída:** se lee de MongoDB. La caída de la caché nunca produce un error.
7. **Cómo se demuestra que funciona:** hit/miss, latencia p95 de `GET /v2/traslados/{id}` y lecturas a MongoDB, medidas con y sin caché en las pruebas de carga (D13).

## Decisión
- **Memcached** (`memcached:1.6-alpine`, cliente `github.com/bradfitz/gomemcache`), como en la clase práctica 3. Es una caché **distribuida**: las dos réplicas de Integración comparten la misma copia. Una caché local en cada réplica divergiría entre réplicas (teórico 3).
- **Patrón cache-aside** sobre la lectura de una solicitud por id, implementado como **decorador del repositorio** (`SolicitudesCache` envuelve al repositorio de MongoDB y cumple la misma interfaz, como en la clase práctica 3).
- **Clave:** `integracion:solicitud:{organizacion_id}:{solicitud_id}:v1`. Incluye la organización para que nunca se sirva la solicitud de una organización a otra (teórico 3: la clave incluye todo lo que cambia la respuesta). El sufijo `v1` permite cambiar el formato sin leer datos viejos.
- **Valor:** el documento de la solicitud serializado en JSON.
- **TTL:** **60 s** más un jitter aleatorio de 0 a 10 s, como red de seguridad y para que las claves no venzan todas juntas (cache stampede).
- **Invalidación:** se **borra** la clave (`Delete`) después de aplicar en MongoDB un evento de Operaciones o una escritura propia: actualizar la fuente y recién entonces borrar (teórico 3).
- **Timeout** del cliente Memcached: **100 ms**.
- **Degradación:** ante error o timeout de Memcached, se registra un log de advertencia, se cuenta `rumbo_cache_operaciones_total{resultado="error"}` y se lee de MongoDB. Los errores al guardar en la caché se ignoran.
- **Qué no se cachea:**
  - el listado con `actualizado_desde`, porque cambia con cada consulta y su clave no se repetiría;
  - cualquier decisión de negocio: la caché solo sirve lecturas.
- **Métrica:** `rumbo_cache_operaciones_total{cache="solicitudes", resultado="hit|miss|error"}`.

## Alternativas consideradas
- **Caché local (ccache) en cada réplica.** Descartada: con dos réplicas, una invalidación en una réplica no borra la copia de la otra, y el consumidor podría ver estados viejos según qué réplica lo atienda.
- **Redis.** Descartada: Memcached cubre lo que se necesita (clave-valor con TTL) y es la herramienta vista en clase.
- **Cachear el listado del polling.** Descartada: la clave depende de `actualizado_desde`, que cambia en cada consulta, así que el hit rate sería casi nulo.
- **Write-through** (actualizar la caché al aplicar el evento). Descartada: borrar es más seguro que actualizar dos lugares, porque evita escribir una versión vieja si dos eventos se cruzan (teórico 3).

## Consecuencias
- **Ganamos:**
  - menos lecturas a MongoDB y menor latencia en la consulta más repetida;
  - consistencia entre réplicas;
  - si la caché cae, el servicio sigue funcionando.
- **Pagamos:**
  - una dependencia más que operar;
  - un salto de red en cada hit;
  - una ventana de hasta 60 s en la que, si se perdiera una invalidación, se podría ver un estado anterior. El TTL la acota.

## Cómo se verifica
- **Tests:** un hit no consulta el repositorio de MongoDB; un miss lo consulta y guarda; aplicar un evento borra la clave; con Memcached caído se responde igual desde MongoDB.
- **Medición (D13):** latencia p95 de `GET /v2/traslados/{id}` y lecturas a MongoDB, con y sin caché, bajo la misma carga, en el informe de capacidad.
