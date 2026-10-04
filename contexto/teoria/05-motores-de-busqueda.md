# Teórico 5 — Motores de búsqueda, relevancia y RAG

Fuente (PDF de la cátedra, ya no se guarda local): `motores-de-busqueda.pdf`

## Por qué un motor de búsqueda
`LIKE '%rock%'` busca literal, no rankea, no analiza lenguaje y mezcla carga de descubrimiento con la DB transaccional.
El motor **no reemplaza** a la DB ni a la API de Catálogo: construye una **vista optimizada para recuperar y rankear**.
Justificarlo cuando: mucho texto libre, consultas abiertas, ranking por relevancia, filtros/facetas/typos, carga que no debe competir con transacciones. Si no, full-text de la DB puede alcanzar. *Complejidad mínima.*

## Information Retrieval
- Documento con campos: unos para texto (`titulo`, `descripcion`), otros para filtro (`fecha`, `ciudad`, `precio`), otros para ranking (`disponible`). **Diseñar el documento para la lectura, no copiar tablas.**
- **Precision** = relevantes devueltos / devueltos. **Recall** = relevantes devueltos / relevantes totales. Tensión: restringir sube precision, sinónimos suben recall.
- **Análisis de texto:** tokenización → normalización (minúsculas, acentos) → stopwords → stemming/lematización. El mismo análisis en índice y consulta.
- **Índice invertido:** término → posting list de documentos.
- Booleano (AND/OR/NOT) = coincidencia binaria, no ordena.

## Pipeline: análisis → recuperación → filtros → ranking
- **Filtro** elimina (obligatorio). **Sinónimo** expande. **Ranking** reordena elegibles. No confundirlos.
- **TF-IDF → BM25:** frecuencia con rendimiento decreciente, términos raros pesan más, normaliza por longitud. No entiende semántica.
- Relevancia textual ≠ **relevancia de negocio** (disponibilidad, fecha, precio, distancia). Reglas comerciales explícitas y auditables.
- Métricas de producto: **zero-result rate**, CTR, conversión post-búsqueda, latencia.
- UX: texto / filtro / **faceta** (distribución + conteos) / ranking. Autocomplete, typos, sinónimos con evidencia. Distinguir causas de "sin resultados".

## El índice como read model derivado (clave para el proyecto)
```
Catálogo (tx: entidad + outbox) → relay → broker → Indexador → upsert en índice ← consulta usuario
```
- **"Search ayuda a descubrir; el servicio dueño valida antes de producir un efecto de negocio."** (precio final, stock, permisos).
- **Indexador = consumidor idempotente:** id estable (mismo id de Catálogo) + **upsert** + **versión monotónica** (`si evento.version > indexada → aplicar; si no → descartar`), comparación y upsert atómicos.
- **Freshness** por campo: texto tolera minutos; disponibilidad se valida en la fuente. Medir tiempo de indexación.
- **Reindexación:** índice nuevo versionado, carga histórica + cambios en curso, validar, switch controlado. El índice siempre debe poder **reconstruirse desde los dueños**.

## Escala
Lucene (biblioteca) → Solr / Elasticsearch / OpenSearch. **Shards** (partes del índice), **réplicas** (tolerancia + lecturas), query routing, hot shards.
Si Search cae, **no debe impedir comprar/reservar**.

## Semántica, híbrida, RAG
- **Embeddings** + búsqueda vectorial: capta intención ("música pesada" ≈ "heavy metal"), falla en IDs exactos y restricciones duras.
- **Híbrida** = BM25 + vectorial con política de fusión calibrada; filtros siguen siendo explícitos.
- **RAG:** retriever → contexto → LLM → respuesta con fuentes. Puede fallar por mala recuperación, docs viejos, permisos, fragmentación. **No confirma datos transaccionales.**

## Casos
Wikimedia CirrusSearch (índice separado near-real-time), Netflix (búsqueda multimodal híbrida), Uber H3 (índice geoespacial).

## Puntos clave para defender
Índice invertido · precision/recall · BM25 · filtro ≠ sinónimo ≠ ranking · índice = read model eventual · indexador idempotente con versión · el dueño valida.
