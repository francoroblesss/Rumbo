# ADR-015 — Capacidad y costos (D13)

- **Fecha:** 2026-10-16 (corrida de 15:25 a 15:37 UTC)
- **Estado:** aceptada (primera medición, Entrega 2)

## Contexto
D13 pide los **resultados de las pruebas de carga**, el **principal límite identificado**, la **alternativa de escalado** y una **estimación de costos**. La consigna (§2.11) pide tests de carga documentados y analizados. En clase se explicó que se trata de ver cómo se comporta el proyecto ante un **pico de demanda**, con escalones crecientes, y de comparar con y sin balanceo y caché.

## Método
- **Herramienta:** **k6** (`grafana/k6` en Docker), con los scripts en `pruebas/carga/`.
- **Objetivo medido:** la capacidad publicada (Integración de carga), atacada **dentro de la red de Docker a través del balanceador** (`balanceador-integracion`), con los headers de identidad que pone el gateway. No se ataca a través del gateway porque su límite de tráfico (60 pedidos por minuto por organización) cortaría la prueba en el primer segundo. Ese límite se prueba aparte y su comportamiento esperado es `429` con `Retry-After`: con una ráfaga de 14 pedidos se obtuvieron 11 `200` (la ráfaga de 10 más uno recargado) y después `429` con `Retry-After: 1` (ver [evidencias](../evidencias/entrega-2.md)).
- **Mezcla de carga:** 90 % de consultas por id (`GET /v2/traslados/{id}`, el polling) y 10 % de creaciones (`POST /v2/traslados`).
- **Escalones de tasa constante:** **1, 2, 5, 10, 20, 50 y 100 requests/s**, de **30 s** cada uno.
- **Escenarios:**

  | Escenario | Réplicas de Integración | Memcached |
  | --- | --- | --- |
  | **A** | 1 (`integracion-2` detenida) | detenido |
  | **B** | 1 | activo |
  | **C** | 2 | activo |

- **Por escalón se registra:** p95 y p99 de latencia, tasa de error, throughput logrado y uso de CPU y memoria de cada contenedor (`docker stats`).
  - Revisión 2026-10-16: la CPU de cada escalón se resume con la **mediana** de sus muestras y no con el promedio. Los healthchecks de `mongo` (`mongosh`) y `rabbitmq` generan picos de 60 % a 180 % de un núcleo que no dependen de la carga; en reposo, `mongo` marca ~1 % y ~66 % en la muestra que coincide con el healthcheck. El promedio se sigue usando para el costo, porque el proveedor también cobra esos picos.
- **Regla del límite:** la **capacidad** de un escenario es el último escalón con **p95 ≤ 300 ms y errores ≤ 1 %** (el SLO de ADR-014). El **principal límite** es el contenedor con mayor uso de CPU (en % de un núcleo) en el primer escalón que no cumple, o en el de 100 req/s si cumplen todos.

## Resultados
Datos completos (con la CPU de cada contenedor por escalón) en [`docs/capacidad/informe-entrega-2.md`](../capacidad/informe-entrega-2.md). CPU en % de un núcleo, mediana por escalón.

| Escenario | Escalón (req/s) | Logradas (req/s) | p95 (ms) | p99 (ms) | Errores (%) | CPU integracion-1 | CPU integracion-2 | CPU mongo | CPU memcached | CPU balanceador |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| A | 1 | 1,0 | 4,5 | 6,9 | 0 | 0,4 | — | 2,2 | — | 0,0 |
| A | 10 | 10,0 | 5,0 | 9,5 | 0 | 3,3 | — | 1,9 | — | 0,4 |
| A | 50 | 50,0 | 4,1 | 6,6 | 0 | 12,4 | — | 7,0 | — | 2,0 |
| A | 100 | 100,0 | 3,8 | 6,3 | 0 | **22,7** | — | 11,0 | — | 4,0 |
| B | 1 | 1,0 | 4,6 | 5,0 | 0 | 0,3 | — | 18,0¹ | 0,0 | 0,0 |
| B | 10 | 10,0 | 2,7 | 5,0 | 0 | 1,9 | — | 9,4¹ | 0,3 | 0,5 |
| B | 50 | 50,0 | 2,7 | 5,7 | 0 | 7,3 | — | 3,6 | 1,2 | 2,0 |
| B | 100 | 100,0 | 2,4 | 4,1 | 0 | **13,5** | — | 7,8 | 2,1 | 3,9 |
| C | 1 | 1,0 | 4,0 | 4,2 | 0 | 0,2 | 0,4 | 41,2¹ | 0,1 | 0,0 |
| C | 10 | 10,0 | 3,2 | 6,1 | 0 | 1,4 | 2,1 | 2,4 | 0,4 | 0,5 |
| C | 50 | 50,0 | 2,4 | 5,0 | 0 | 4,4 | 4,2 | 2,6 | 1,1 | 1,9 |
| C | 100 | 100,0 | 2,3 | 3,3 | 0 | **7,5** | 6,2 | 4,5 | 2,0 | 3,7 |

¹ Muestras que coincidieron con el healthcheck de `mongo`: no acompañan a la carga.

Los escalones de 2, 5 y 20 req/s, que también cumplen, están en el informe. Todos los escalones de los tres escenarios cumplen el SLO, con 0 % de errores. El único p99 alto (406 ms en A, a 2 req/s) es un pedido que NGINX intentó contra `integracion-2` detenida: esperó 1 s de `proxy_connect_timeout` y lo reenvió a `integracion-1` (health check pasivo de ADR-009).

- **B contra A (caché, ADR-007):** a 100 req/s, el p95 baja de 3,8 a 2,4 ms (−37 %), la CPU de Integración de 22,7 % a 13,5 % (−41 %) y la de MongoDB de 11,0 % a 7,8 % (−29 %). Hit ratio de 99,0 % en B y 97,8 % en C. En A, con Memcached detenido, el decorador lee de MongoDB y no hay errores (fail-open).
- **C contra B (balanceo, ADR-009):** a 100 req/s, cada réplica usa la mitad de CPU que la réplica única (7,5 % y 6,2 % contra 13,5 %) y reciben 54 % y 46 % de los requests. El p95 casi no cambia (2,4 → 2,3 ms), porque con una réplica todavía sobra margen.

## Principal límite identificado
Ningún escenario llegó al límite: los tres cumplen el SLO hasta 100 req/s. Por la regla, el principal límite es el contenedor con más CPU en el escalón de 100 req/s, y en los tres escenarios es **la réplica de Integración**: `integracion-1` con 22,7 % (A), 13,5 % (B) y 7,5 % (C) de un núcleo. Su CPU crece de forma aproximadamente lineal con la tasa (≈ 0,23 % por req/s en A). Por extrapolación, una réplica sin caché llegaría a un núcleo cerca de las 400 req/s; es una estimación que hay que confirmar con escalones mayores.

## Alternativa de escalado
Como **ningún escenario llegó al límite**, la capacidad **supera las 100 req/s medidas** y el **siguiente paso es medir escalones mayores** (por ejemplo 200, 400 y 800 req/s) con el mismo método, para encontrar el escalón que no cumple y su contenedor limitante.

## Estimación de costos
- **Proveedor de referencia:** **Railway**, uno de los servicios simples sugeridos en clase. Cobra por uso de CPU y memoria, y permite desplegar las mismas imágenes de Docker.
- **Fórmula:** costo mensual ≈ Σ por contenedor (vCPU promedio × precio de vCPU por mes + GB de RAM promedio × precio de GB por mes), con el uso promedio de cada contenedor medido en el escalón de **10 req/s** del escenario **C** (carga normal esperada).
- **Precios:** **USD 20 por vCPU por mes** (USD 0,00000772 por vCPU por segundo) y **USD 10 por GB de RAM por mes** (USD 0,00000386 por GB por segundo), según <https://railway.com/pricing>, consultada el 2026-10-16. Planes: Hobby, USD 5 por mes con USD 5 de crédito de uso incluido; Pro, USD 20 por mes con USD 20 de crédito.
- **Estimación:** uso **promedio** en el escalón de 10 req/s del escenario C (5 muestras de `docker stats`). No se incluyen `lgtm`, que es observabilidad de desarrollo (en producción se usaría un servicio administrado), ni `clima-mock` (en producción se llama a la API del otro grupo).

  | Contenedor | vCPU promedio | GB de RAM promedio | Costo CPU (USD/mes) | Costo RAM (USD/mes) | Total (USD/mes) |
  | --- | ---: | ---: | ---: | ---: | ---: |
  | rabbitmq | 0,390 | 0,195 | 7,80 | 1,95 | 9,75 |
  | mongo | 0,289 | 0,276 | 5,78 | 2,76 | 8,54 |
  | mysql | 0,057 | 0,258 | 1,14 | 2,58 | 3,72 |
  | integracion-2 | 0,023 | 0,013 | 0,45 | 0,13 | 0,58 |
  | operaciones | 0,022 | 0,013 | 0,44 | 0,13 | 0,57 |
  | integracion-1 | 0,015 | 0,014 | 0,30 | 0,14 | 0,45 |
  | notificaciones | 0,004 | 0,008 | 0,09 | 0,08 | 0,16 |
  | recursos | 0,004 | 0,007 | 0,08 | 0,07 | 0,14 |
  | memcached | 0,004 | 0,005 | 0,08 | 0,05 | 0,13 |
  | balanceador-integracion | 0,005 | 0,003 | 0,10 | 0,03 | 0,13 |
  | identidad | 0,001 | 0,008 | 0,01 | 0,08 | 0,09 |
  | gateway | 0,001 | 0,006 | 0,01 | 0,06 | 0,07 |
  | **Total** | **0,81** | **0,81** | **16,27** | **8,06** | **≈ 24,3** |

  - **≈ USD 24 por mes. No entra en el crédito del plan más barato** (Hobby, USD 5). Con el plan Pro (USD 20 con USD 20 de crédito) se pagarían ≈ USD 4 extra.
  - **Casi el 75 % del costo es RabbitMQ y MongoDB**, y la mayor parte de su CPU promedio son los healthchecks (`rabbitmq-diagnostics` y `mongosh`), no la carga: con la mediana, MongoDB usa 0,024 vCPU en ese escalón. En Railway esos healthchecks no corren igual, así que el costo real sería menor. Lo que realmente pesa es la RAM fija de las tres bases y del broker, no el tráfico.
  - Los servicios Go son baratos: los siete, juntos, cuestan menos de USD 2,5 por mes.

## Consecuencias
- La capacidad medida y su límite quedan como **línea de base**, para comparar después de cada cambio (por ejemplo, la búsqueda o la integración con el clima real).
- Los umbrales del breaker (ADR-008) y los timeouts (ADR-016) se revisan con estas mediciones.
- Se confirma lo que dice el teórico 3: la caché sirve cuando una lectura se repite mucho (el polling por id). Baja la carga de la fuente de verdad y, como el decorador es fail-open, su caída no corta el servicio.
- Se confirma lo que dice el teórico 6: las réplicas detrás del balanceador suman **capacidad y tolerancia a la caída de una instancia**, pero no bajan la latencia mientras una sola réplica tenga margen.
- Limitaciones de la medición: todo corre en una sola notebook, con k6 incluido, así que los números sirven para comparar escenarios y no como capacidad de producción. Además, con solo 20 solicitudes en la base, MongoDB trabaja con todos los datos en memoria.
