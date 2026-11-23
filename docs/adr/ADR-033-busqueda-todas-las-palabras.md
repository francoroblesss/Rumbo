# ADR-033 — La búsqueda exige todas las palabras del texto (reemplaza parcialmente a ADR-011)

- **Fecha:** 2026-11-20
- **Estado:** aceptada
- **Reemplaza parcialmente a:** [ADR-011](ADR-011-busqueda.md), en la consulta de texto: eDisMax pasa a exigir **todas** las palabras (`mm=100%`). El resto de ADR-011 sigue vigente (motor, documento, indexador idempotente, filtros, orden, reconstrucción y `503`), con el alcance de ADR-024.

## Contexto
- En la devolución de la Presentación Grupal, el profesor buscó **"caja grafana"** desde la web y aparecieron **todas** las cajas, no solo la caja Grafana. Desde la terminal parecía correcto, porque se pedía `orden=relevancia` y la caja Grafana salía primera. Su observación: "el motor de búsqueda no funciona al 100 % porque no coincide con la web".
- Causa: la consulta es eDisMax sin `mm` (*minimum should match*). Sin `mm`, Solr combina las palabras con `q.op=OR`: alcanza con que el documento tenga **una** de ellas. El ranking (`score`) era correcto (más palabras, más puntaje), pero la **recuperación** traía de más. La web ordena por fecha por defecto (`orden=fecha`, §16), así que mostraba todas las cajas en orden de franja.
- Del teórico 5 (motores de búsqueda):
  - separa **qué documentos entran** (consulta y filtros) de **en qué orden salen** (relevancia: "filtro ≠ sinónimo ≠ ranking"). El problema estaba en lo primero;
  - **precision** = relevantes devueltos / devueltos y **recall** = relevantes devueltos / relevantes totales, en tensión: "restringir sube precision". Con `OR` el recall era alto y la precision baja ("caja grafana" devolvía todas las cajas); combinar las palabras con `AND` restringe y sube la precision.

## Decisión
- Cuando hay texto, la consulta lleva **`mm=100%`**: el traslado tiene que tener **todas** las palabras del texto, en cualquiera de los campos de `qf` (descripción, nombres y direcciones de origen y destino, referencia externa). El análisis en español no cambia (minúsculas, sin tildes, plural a singular y stopwords fuera), así que "Cajas de Grafana" también coincide con "caja grafana".
- Sin texto no cambia nada (`q=*:*`).
- El orden por defecto sigue siendo **por fecha y hora**, en la API (§16) y en la web, como dice el dominio (§13 bis) y CA-26: con `mm=100%` el orden ya no cambia **qué** se devuelve, solo en qué orden sale. Quien quiera lo que mejor coincide primero elige "Relevancia". El contrato no cambia.
- No hay que reindexar: `mm` es un parámetro de la consulta, no del esquema.

## Alternativas consideradas
- **Solo cambiar el orden de la web a relevancia.** Descartada: la caja Grafana saldría primera, pero el total seguiría contando todas las cajas y la segunda página seguiría trayendo cajas que no son Grafana. No arregla la recuperación.
- **`q.op=AND`.** Equivalente para esta consulta; se prefiere `mm`, que es el parámetro propio de eDisMax y permite, si hiciera falta, relajarlo (por ejemplo, `mm=2<75%`) sin cambiar la sintaxis.
- **Búsqueda difusa (`~`) o corrección ortográfica.** Descartada por complejidad mínima: nadie la pidió y suma resultados inesperados.

## Consecuencias
- **Ganamos:** la web y la terminal devuelven lo mismo; "caja grafana" trae solo los traslados que tienen las dos palabras (más precision); el total y la paginación son coherentes con lo que se buscó.
- **Pagamos:** menos recall. Una palabra mal escrita, de más o que no está en ningún campo de `qf` deja la búsqueda sin resultados (antes traía algo por las otras palabras). Queda como límite conocido; si molestara, se relaja `mm` (por ejemplo, `2<75%`) sin cambiar el esquema.

## Cómo se verifica
- Test de integración `TestSolr` / "todas las palabras del texto tienen que aparecer" (Solr real con contenedores), ordenando por fecha: `caja grafana`, `CAJA Grafana`, `cajas grafana` y `caja de grafana` → solo la caja Grafana (total 1); `caja` → las tres; `caja herramientas` → 1; `caja inexistente` → 0. Pasa (2026-11-20). Los dos casos que daban por hecho el `OR` se ajustaron: `camión Hospital` ahora espera 0 resultados, y en el de relevancia los dos documentos tienen las dos palabras.
- E2E `TestBusquedaTodasLasPalabras` (dos reservas en franjas distintas con un sufijo único, "Bulto rojo" y "Bulto azul"; `bulto rojo <sufijo>` sin `orden` → total 1 y es el bulto rojo). Usa palabras que no son las de la demo para no sumarle resultados a "caja grafana" en un sistema que dura. Corre en el CI (job e2e).
- Manual: con `Caja Grafana`, `Caja de herramientas` y `Caja de vidrio` reservadas, la web devuelve 3 resultados para "caja grafana" (la reservada y otras dos que ya había en la base y tienen las dos palabras: "Caja de prueba para Grafana" y la del e2e) y 178 para "caja"; por terminal, lo mismo (3 y 178, con y sin `orden=relevancia`). Con "Relevancia" la caja Grafana sale primera (2026-11-20).
- Revisión 2026-11-23: el orden por defecto de la web vuelve a fecha (el dominio manda), se citan el teórico 5 y el trade-off precision/recall, y el e2e usa palabras que no son las de la demo y una franja por reserva.
