# Práctico — Clase 3: Memoria caché (ccache y Memcached)

Fuente: repo del docente, `CLASE_3/` (`PRESENTACION/Presentacion.pdf` 15 págs. — su portada dice "Clase 4"; `DEMO/`).

## Por qué caché
- Las bases en disco (MongoDB) persisten bien pero son lentas para lecturas masivas comparadas con la RAM.
- Caso: cientos de usuarios consultando alojamientos en temporada alta (**Salinas Gran Hotel, cabañas en Mina Clavero**). 1.000 queries idénticas desperdician recursos: la caché guarda la primera y sirve las 999 restantes desde memoria en microsegundos.

## Configuración y gestión
- **TTL:** cuánto tiempo un dato es válido. Máximo absoluto o TTL que se resetea con cada consulta.
- **Políticas de reemplazo** al llegar al límite: eliminar los más antiguos, los menos consultados o los que llevan más tiempo sin consultarse.
- **Tamaño máximo (Max Memory):** RAM asignada para no consumir todo el servidor.
- **Claves:** funciona como diccionario; la clave suele incluir tipo + id, p. ej. `PRODUCTO:12`.

## Patrones
| Patrón | Flujo | Nota |
| --- | --- | --- |
| **Cache-Aside** | App consulta caché → hit: devuelve · miss: va a la BD → guarda en caché y devuelve | La app gestiona la caché (el que se implementa en clase) |
| **Read-Through** | App siempre consulta la caché; si no está, **la caché** lo obtiene de la BD | La app no consulta la BD directamente |
| **Write-Through** | Al modificar: se actualiza caché y BD | Caché actualizada inmediatamente; prioriza consistencia |
| **Write-Behind (Write-Back)** | Primero caché (respuesta rápida), luego la caché actualiza la BD | Escrituras más rápidas; **riesgo si la caché falla antes de persistir** |
| **Refresh-Ahead** | La caché detecta un dato próximo a expirar y lo refresca desde la BD | Evita misses, mantiene lecturas rápidas |

## Local vs distribuida
| | Local (ccache) | Distribuida (Memcached) |
| --- | --- | --- |
| Dónde | RAM de la propia API Go | Servicio externo independiente |
| A favor | Rapidísima, cero latencia de red, fácil de integrar | Consistencia total entre instancias |
| En contra | ⚠️ Se desincroniza con múltiples servidores | ⚠️ Latencia de red y dependencia externa |

## Resiliencia: degradación elegante
- **La caché NO es la fuente de verdad.** Si falla, la aplicación no debe fallar.
- ❌ Devolver **HTTP 500** por un fallo de caché: inaceptable en producción.
- ✅ Capturar el error, loguearlo, ignorar la caché temporalmente e ir a la base: más lento, pero el sistema sigue vivo.

## Ejercicios (consignas de la presentación) y su implementación en `DEMO`
La demo reutiliza la API de Items de la Clase 2 (controller, database, model, mock, interfaz `ItemsRepo`, service y test **idénticos**). Módulo `main`, go 1.25.0; deps: Gin v1.12.0, mongo-driver v1.17.9, `github.com/karlseguin/ccache/v3` v3.0.8, `github.com/bradfitz/gomemcache`. Cada ejercicio tiene su `main_ejercicioN.go`; los 1–3 están **comentados** y el 4 es el activo.

### Ej. 1 — Medir el problema
- Consigna: latencia artificial `time.Sleep(500 * time.Millisecond)`, consultar varias veces el mismo producto, medir. Objetivo: línea base, comparar luego con caché, entender el costo real de la BD.
- Código (`items_mongo_ej1.go`): `ItemsMongoDB.GetItemByID` con `time.Sleep(5000 * time.Millisecond)` (en el código son **5 s**, no 500 ms) antes del `FindOne`. Wiring directo Mongo → Service → Controller, `GET /items/:id` en `:8080`.

### Ej. 2 — Caché local (cache-aside con ccache)
- Consigna: `value := cache.Get(key)`; si `nil` → miss → BD → `cache.Set(key, value, expiration)`. Comparar tiempos con/sin caché.
- Código (`items_ccache_ej2.go`): **decorador** `ItemsCCache{NextRepo ItemsRepo; Cache *ccache.Cache[items.ItemModel]}` ("el escudo"). Clave `"ITEM:" + id`; hit si `cached != nil && !cached.Expired()`; miss → `NextRepo.GetItemByID` → `Cache.Set(key, item, 5*time.Minute)`. `CreateItem` delega al `NextRepo`.
- Wiring: `ccache.New(ccache.Configure[models.ItemModel]().MaxSize(1000))`; `ItemsCCache{NextRepo: mongoRepo, Cache: localCache}` se inyecta en el Service.

### Ej. 3 — Memcached (caché distribuida)
- `docker run --name memcached -p 11211:11211 -d memcached` → `localhost:11211`; cliente `memcache.New("localhost:11211")`.
- Consigna: reemplazar la local por Memcached, cache-aside, comparar latencias sin caché / local / Memcached. Objetivo: latencia de red vs de proceso.
- Código (`items_memcached_ej3.go`): `ItemsMemcachedEj3{Client *memcache.Client; NextRepo ItemsRepo}`. Hit → `json.Unmarshal`. **Manejo frágil a propósito:** si `err != memcache.ErrCacheMiss` (servidor caído/red) → devuelve el error al usuario ("ERROR CRÍTICO: Memcached se cayó y rompimos la API!"). Miss → Mongo → `json.Marshal` + `Set(&memcache.Item{Key, Value, Expiration: 60})`.

### Ej. 4 — Tolerancia a fallos (activo)
- Consigna: `docker stop memcached`; la API debe seguir **sin errores 500**; fallback a la BD. Preguntas: ¿qué pasa si falla la escritura en Memcached? ¿retornar error o ignorarlo? ¿cómo afecta a la respuesta?
- Código (`items_memcached_ej4.go`): `ItemsMemcachedEj4`. Cualquier error del `Get` (miss **o** caído) → log "DEGRADACIÓN ELEGANTE..." → Mongo. El `Set` posterior **ignora el error** (`_ = repo.Client.Set(...)`, `Expiration: 60`).
- `main_ejercicio4.go`: puerto por env `PORT` con default `8080` (`os.Getenv("PORT")`).

## Cierre
Separación de responsabilidades con interfaces en Go · optimización drástica de recursos y latencia · backend tolerante a fallos. "La caché mejora rendimiento, no es crítica."
