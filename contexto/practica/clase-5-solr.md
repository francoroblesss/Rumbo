# Práctico — Clase 5: Motores de búsqueda con Apache Solr

Fuente: repo del docente, `CLASE_5/` (`PRESENTACION/Presentacion.pdf` 24 págs., `README.md`, `docker-compose.yml`, `schema.json`, `products.json`, `DEMO/`, `ACTIVIDADES/`, `ACTIVIDADES_RESUELTAS/`).

Subtítulo: "De Information Retrieval a una Search API en Go". Flujo: Client → API → Solr → resultados ordenados.

## Teoría de la presentación
- **Problema:** e-commerce con cientos de miles/millones de productos (`id, title, description, category, brand, price`); consultas libres ("zapatillas", "zapatillas running adidas", "calzado para correr"). Queremos: encontrar rápido, filtrar (categoría, marca, precio), ordenar por relevancia. ¿Alcanza un `SELECT ... LIKE '%zapatillas%'`? Hace falta texto libre, relevancia, ranking, filtros, escalabilidad.
- **Base de datos vs motor de búsqueda:** BD = fuente de verdad, transacciones, consultas estructuradas, estado del negocio. Motor = documentos, texto libre, análisis de texto, relevancia, ranking, filtros/facetas. "**Solr no reemplaza necesariamente a MongoDB/SQL: cumple otra responsabilidad.**" (Nota: las SQL modernas tienen full-text; la comparación es contra un LIKE simple.)
- **Information Retrieval:** recuperar los documentos más relevantes para una consulta. Query → Candidatos → Score → Ranking (por score) → Resultados.
- **Análisis de texto** (lo define el **Analyzer**, en indexación y búsqueda): "Zapatillas ADIDAS para Running!" → tokenización → minúsculas → eliminación de stopwords (opcional; "para" se elimina por la lista de stopwords en español) → tokens `zapatillas, adidas, running`.
- **Ranking:** ¿un match en `title` vale igual que en `description`? El motor calcula un score por documento y ordena. **Boost:** `qf=title^5 description` (title peso 5, description peso 1).
- **De Product a documento Solr:** campos de **texto** (`title`, `description`), de **filtro** (`category`, `brand`), **numéricos** (`price`). "No necesariamente se indexa todo el modelo de dominio: se eligen los datos necesarios para búsqueda, filtrado y respuesta."
- **Parámetros:**
  - `q` — consulta principal; participa en recuperación y ranking.
  - `defType=edismax` — parser eDisMax, más flexible, permite boost de campos.
  - `fq` — filtro que **restringe sin afectar el score** (categorías, marcas, rangos de precio). Se puede repetir.
  - `qf` — campos donde buscar y con qué peso.
  - `rows` — máximo de resultados (**default 10**); no cambia `numFound`.
  - `fl` — campos a devolver; `score` aparece si está en `fl`.
  - Respuesta: `{"response": {"numFound": N, "docs": [...]}}`.
- **Con eDisMax el operador por defecto es OR**: un espacio no garantiza AND → escribir `zapatillas AND running`.
- Solr encuentra sin recorrer cada producto gracias al **índice invertido** creado en la indexación.
- **Arquitectura en Go:** Client → **Gin Handler** (valida, delega) → **SearchService** (orquesta, puede enriquecer) → **interfaz SearchEngine** (contrato; permite cambiar la implementación) → **SolrClient** (URLs, HTTP, parsea JSON) → Solr. "**MongoDB puede continuar como fuente de verdad. Solr devuelve IDs/scores y se enriquece desde Mongo si es necesario.**"
- **Cierre:** "Un sistema RAG puede utilizar un motor de búsqueda/retriever para recuperar contexto antes de llamar a un LLM."
- **Preguntas de arquitectura:** ¿Por qué el Handler no conoce Solr? ¿Cómo conviven MongoDB y Solr? ¿Qué indexamos y qué no? **¿Qué ocurre con la consistencia cuando Mongo cambia antes que Solr?**
- **Bonus sugeridos:** cache-aside, facetas por marca/categoría, boost de title sobre description, **endpoint de reindexación**.

## Setup (README de `CLASE_5`, una vez por clase)
```bash
docker compose up -d --wait && docker compose ps     # solr running/healthy; panel http://localhost:8983/solr/
curl -fsS -X POST -H 'Content-Type: application/json' --data-binary @schema.json 'http://localhost:8983/solr/products/schema'   # "status":0 (si ya existen los campos, seguir)
curl -fsS -X POST -H 'Content-Type: application/json' --data-binary @products.json 'http://localhost:8983/solr/products/update?commit=true'
curl -fsS -G 'http://localhost:8983/solr/products/select' --data-urlencode 'q=*:*' --data-urlencode 'rows=15' --data-urlencode 'wt=json'   # "numFound":15
docker compose stop     # conserva datos  |  docker compose down  → borra contenedor e índice (repetir setup)
```
- `docker-compose.yml`: imagen `solr:9`, puerto 8983, `command: solr-precreate products`, **healthcheck** `curl -fsS http://localhost:8983/solr/products/admin/ping` (interval 3s, timeout 2s, retries 30, start_period 10s).
- `schema.json` (`add-field`): `title` y `description` **`text_general`**; `category` y `brand` **`string`**; `price` **`pdouble`**; todos indexed+stored, no multivalued. (La diapositiva muestra otra versión con `text_es` y `pint`; el archivo real del repo usa `text_general` y `pdouble`.)
- `products.json`: **15 productos** `p1..p15` (calzado, indumentaria, accesorios; marcas Adidas, Nike, Puma, Asics, New Balance, Reebok; precios 12.000–155.000).
- Cada estudiante usa una **variante** asignada por formulario (`FORMULARIO_GOOGLE`, V01–V10; esa carpeta está en el `.gitignore` del repo y no se publica).

## Demo (`DEMO/`, módulo `search-demo`, go 1.22, **solo stdlib**)
- Recorrido: cliente HTTP → handler → Service → Engine → SolrClient → Solr.
- **Parte A** `consultas.sh` (función `consulta` con `curl -fsS -G ... --data-urlencode 'wt=json'`): (1) `q=*:*` para verificar documentos; (2) `q=zapatillas`, `defType=edismax`, `qf=title^5 description`, `fl=id,title,score`; (3) `q=zapatillas AND running` + `fq=category:calzado` + `rows=5`; (4) `q=running` + `fq=category:calzado` + `fq=brand:Adidas` (filtros acumulados).
- **Parte B** `main.go` (todo en un archivo, `net/http` con `http.NewServeMux`, `mux.Handle("GET /products/search", ...)`, `:8080`):
  - Modelos `Query{Text, Category, Brand; Limit}`, `Result{id,title,brand,category,price,score}`, `SearchResponse{Results, Total}` (contrato público `results`/`total`, no `response.docs`/`numFound`).
  - `Engine interface { Search(context.Context, Query) (SearchResponse, error) }` — "necesito alguien que pueda buscar"; no dice Solr ni HTTP; un test podría usar un fake.
  - `SolrClient{baseURL, http *http.Client}`: arma `url.Values` (`q`, `defType=edismax`, `qf=title^5 description`, `rows`, `fl`, `wt=json`); filtros con **`fq={!term f=category}valor`** / `{!term f=brand}` para que sean **literales y no sintaxis Solr del usuario**; `http.NewRequestWithContext`; `defer resp.Body.Close()`; status fuera de 2xx → error; decodifica la forma externa y la traduce. Errores con `fmt.Errorf("...: %w", err)`.
  - `Service`: si `Limit == 0` → 10 ("esta regla no pertenece al Controller ni al cliente de Solr").
  - Controller: `q` obligatorio (`TrimSpace`; 400 "q es obligatorio"); `limit` entero positivo si se envía (400 "limit debe ser un entero positivo"); error del servicio → **502 Bad Gateway** "búsqueda no disponible" (falló una dependencia aguas abajo).
  - `main`: `http.Client{Timeout: 2 * time.Second}`; "esta es la única parte que conoce la dirección de Solr".
  - Pruebas: `curl` con `q=running`, `+category=calzado`, `q=zapatillas&brand=Adidas&limit=5`; `curl -i` sin `q` y con `limit=0`.
  - (El README de la demo menciona "cinco documentos indexados"; el dataset actual tiene 15.)

## Actividad 1 — Construcción de consultas (`curl` directo, sin Go)
- 6 consultas en 4 scripts (`SOLR_URL=http://localhost:8983/solr/products/select`), siempre `--data-urlencode` por parámetro y `wt=json`; entregar comando + JSON completo en el formulario.

| Archivo | Consulta |
| --- | --- |
| `01_busquedas.sh` | Búsqueda de texto de la variante (eDisMax en title y description) |
| `02_filtros_y_campos.sh` | Texto + filtro (marca o categoría) como `fq`, `rows=5`, `fl=id,title,brand,category,price,score` |
| `03_relevancia.sh` | Misma búsqueda dos veces: sin boost y con `qf` priorizando `title` (el boost **no es un filtro**) |
| `04_consultas_adicionales.sh` | Dos términos con el operador asignado (`AND` u `OR`) en `q`; texto + **rango de precio** `fq=price:[MIN TO MAX]` (inclusivo, va en `fq`, no en `q`) |

- **Resuelta** (ejemplos con `zapatillas`/`running`, `qf=title description`): consultas 1–2 texto; 3 `fq=category:calzado`; 4–6 `fq` calzado + Adidas, `rows=5`, `fl`; 7 `fl=id,title,score`; desafío `qf=title^5 description`.
- **Conclusión esperada:** `q` expresa la intención; `fq` agrega condiciones obligatorias; `score` ordena a los elegibles. **Un score no es una nota absoluta**: se interpreta dentro de la misma consulta y configuración.
- Preguntas de reflexión: ¿cuántos coinciden con "zapatillas"? ¿y con "zapatillas AND running"? ¿cómo cambia el score si agregamos un filtro?

## Actividad 2 — API Go + Gin + Solr (estilo "Clase 5" con constructores)
- Módulo `main`, go 1.22, Gin v1.10.0. Estructura: `controllers/products`, `services/products`, `models/products`, `repositories/products/{solr_repository.go, response.go}`, `main.go`.
- **Requisitos:** `GET /products/search?q=&category=&brand=&limit=`; `q` obligatorio; `limit` entero positivo, default 10; `category`/`brand` opcionales como `fq` exactos; `defType=edismax`, `qf=title description`; **400** parámetros inválidos, **502** falla de Solr, **200** con `results`.
- TODOs: controller (endpoint + validación) y `solr_repository.go` (URL `<baseURL>/select`, parámetros, `{!term f=...}`, `rows`, `fl`, `wt=json`, request con context, validar status, parsear, mapear).
- **Solución:**
  - `models`: `Query` ("no usa los nombres q, fq o rows: detalles de Solr que se traducen recién en el repositorio"), `Result`, **`Engine interface { Search(context.Context, Query) ([]Result, error) }`**.
  - `repositories`: `Client{baseURL, http}` + `NewClient(baseURL, httpClient)`; `response` **privado** ("la forma externa de Solr no debe filtrarse a otras capas"); mapea docs a `[]models.Result`.
  - `services`: `Service{engine}` + `NewService(engine)`; `Limit <= 0` → 10.
  - `controllers`: `Handler{service}` + `NewHandler(s)` + `Register(router)` → `router.GET("/products/search", h.search)`; `readLimit`; `c.Request.Context()`; respuesta `gin.H{"results": results}`.
  - `main`: `NewClient("http://localhost:8983/solr/products", &http.Client{Timeout: 2 * time.Second})` → `NewService` → `NewHandler(...).Register(router)` → `:8080`.

| Capa | Qué sabe | Qué no debe conocer |
| --- | --- | --- |
| Controller | HTTP, parámetros, status codes | URL o JSON de Solr |
| Service | reglas de aplicación (limit por defecto) | detalles de HTTP/Solr |
| Engine | contrato de búsqueda | implementación concreta |
| SolrClient | URL, parámetros y JSON de Solr | status HTTP de entrada |

- La presentación pide además `"total"` en la respuesta y lista los criterios: q obligatorio, filtros → fq, limit → rows, error de Solr controlado, **Handler no conoce Solr, Solr detrás de una interfaz**. (La solución de la actividad devuelve solo `results`; la demo devuelve `results` + `total`.)
- El README de la actividad menciona tests (`products_controller_test.go` y uno del repositorio) que **no están en el repo**.
