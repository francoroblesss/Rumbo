# ADR-011 — Búsqueda de traslados con Apache Solr (D6)

- **Fecha:** 2026-10-14
- **Estado:** aceptada (se implementa para la Presentación Grupal) — la ruta del endpoint pasa a `/api/operaciones/traslados/busqueda` y la de reindexación a `/api/operaciones/admin/reindexacion`, sin versión ([ADR-019](ADR-019-contrato-sin-version-en-la-ruta.md)). El resto sigue vigente.

## Contexto
La consigna (§2.6) pide que una entidad del dominio tenga búsqueda con un **motor de búsqueda**, con **paginación**, **filtros relevantes**, al menos **un criterio de ordenamiento**, un índice **sincronizado** con los cambios y un **retraso máximo tolerable** definido. El dominio definió la **búsqueda de traslados** (`dominio.md` §13 bis, `SPEC.md` F-27):
- texto libre: persona, dirección, descripción de lo trasladado, referencia;
- filtros: cliente, tipo de servicio, fechas, estado, chofer, vehículo, origen o destino;
- orden por fecha o por relevancia;
- el cliente solo ve lo suyo.

Del teórico 5:
- se justifica un motor cuando hay **texto libre, ranking y filtros** que no conviene resolver con `LIKE` sobre la base transaccional;
- **el índice es un modelo de lectura derivado**, actualizado por eventos, con un indexador idempotente y versión monotónica;
- tiene que poder **reconstruirse desde el dueño**;
- **la búsqueda ayuda a descubrir y el dueño valida** antes de cualquier efecto;
- si la búsqueda cae, no tiene que impedir operar.

## Decisión
- **Motor:** **Apache Solr 9** (visto en la clase práctica 5), con un core `traslados`, en un contenedor del Compose.
- **Dueño:** Operaciones. El índice es una **vista derivada** de sus datos y vive como adaptador de salida (`adaptadores/solr`, puerto `BuscadorTraslados`). No es un microservicio aparte: no tiene reglas propias, y separarlo sumaría un servicio que operar sin un problema que resolver.
- **Documento indexado** (diseñado para la lectura, no copiando tablas):

  | Tipo de campo | Campos |
  | --- | --- |
  | Identificación | `id` (= id de la solicitud), `traslado_id` |
  | Texto (analizado, español) | `texto` (descripción de lo trasladado, nombres y direcciones de origen y destino, referencia externa, nombres de personas) |
  | Filtro | `organizacion_id`, `tipo_servicio`, `estado_solicitud`, `estado_traslado`, `chofer_id`, `vehiculo_id`, `origen_nombre`, `destino_nombre` |
  | Orden y rango | `hora_comprometida` (o la pedida si todavía no hay compromiso), `actualizado_en` |
  | Control | `version` (la `agregado_version` del último evento aplicado) |

- **Sincronización por eventos:** un **indexador** consume los eventos `operaciones.solicitud.*`, `operaciones.traslado.*` y `operaciones.cancelacion.*` desde su propia cola (`operaciones.indexador`, con reintentos y DLQ de `comun/mensajeria`) y hace un **upsert idempotente**: aplica el documento **solo si la `agregado_version` del evento es mayor** que la indexada. Para eso usa la concurrencia optimista de Solr (`_version_`), así la comparación y la escritura son atómicas (teórico 5).
- **Retraso máximo tolerable: 5 s** (percentil 95) entre la confirmación del cambio en MySQL y su aparición en los resultados. Se mide con la métrica `rumbo_indexacion_demora_segundos` (diferencia entre `ocurrido_en` del evento y el momento en que Solr confirmó la escritura).
- **Consulta:** endpoint del operador `GET /api/v1/operaciones/traslados/busqueda`:
  - parámetros `q`, `organizacion_id`, `tipo_servicio`, `estado`, `desde`, `hasta`, `chofer_id`, `vehiculo_id`, `orden=fecha|relevancia` (default `fecha`), `pagina` y `tamanio` (máximo 100);
  - eDisMax con `qf` ponderado (`texto`);
  - filtros como `fq` con `{!term}` para valores literales (como en la clase práctica 5).

  El **cliente** usa el mismo endpoint, pero siempre con un `fq` de su `organizacion_id` tomado de la identidad del gateway, nunca del parámetro.
- **Reconstrucción:** endpoint de administrador `POST /api/v1/operaciones/admin/reindexacion`, que recorre MySQL y vuelve a indexar todo (el índice se puede reconstruir desde el dueño).
- **Si Solr falla:** la búsqueda responde `503 SERVICIO_NO_DISPONIBLE`. Ninguna operación de negocio depende de Solr. Los eventos quedan en la cola del indexador (o en su DLQ) y se aplican cuando Solr vuelve.
- **El resultado de la búsqueda no habilita acciones:** cancelar o reasignar se valida siempre contra MySQL.
- **Etapa:** la decisión y el diseño quedan en esta entrega; la implementación del indexador y del endpoint es para la **Presentación Grupal**, que es cuando la consigna la exige funcionando.

## Alternativas consideradas
- **`LIKE` o full-text de MySQL.** Descartada: mezcla la carga de búsqueda con la base transaccional de la operación crítica, y no da relevancia ni análisis de texto en español (teórico 5).
- **Elasticsearch u OpenSearch.** Descartadas: Solr es el motor visto en clase y resuelve lo mismo.
- **Microservicio de búsqueda aparte.** Descartado por complejidad mínima: el índice es una vista de los datos de Operaciones.
- **Indexar de forma síncrona en cada transacción.** Descartada: acopla la disponibilidad de Solr a la de las operaciones.

## Consecuencias
- **Ganamos:**
  - búsqueda con relevancia y filtros sin cargar MySQL;
  - un índice reconstruible;
  - si la búsqueda cae, se puede seguir operando.
- **Pagamos:**
  - consistencia eventual: hasta 5 s para ver un cambio;
  - otro contenedor (Solr) y un consumidor más;
  - el esquema del índice hay que mantenerlo junto con el modelo.

## Cómo se verifica (Presentación)
- Un cambio de estado aparece en la búsqueda en menos de 5 s (métrica p95).
- El mismo evento procesado dos veces deja el índice igual; un evento viejo no pisa uno nuevo.
- Con Solr detenido, la búsqueda devuelve 503 y aceptar una solicitud sigue funcionando.
- La reindexación reconstruye el índice desde cero.
