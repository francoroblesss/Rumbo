# Práctico — Clase 2: MongoDB, Go y el Repository Pattern

Fuente: repo del docente, `CLASE_2/` (`PRESENTACION/Presentacion.pdf` 29 págs., `DEMO/`).

Idea central: **la lógica de negocio no debe depender de MongoDB, sino de una abstracción** que permita reemplazar la implementación concreta (real o mock).

## Agenda
1. MongoDB con Docker · 2. Acceso desde Go · 3. Interfaces en Go · 4. Repository Pattern (real + mock) · 5. API de Items.

## MongoDB con Docker
```bash
docker pull mongo:4.4.6
docker run -p 27017:27017 -e MONGO_INITDB_ROOT_USERNAME=root -e MONGO_INITDB_ROOT_PASSWORD=root --name my-mongo mongo:4.4.6
docker ps
docker exec -ti my-mongo bash
mongo --username root --password root --authenticationDatabase admin
show dbs        # si lista las bases, la instancia funciona
```
- **Modelo documental:** documentos dentro de colecciones, sin esquema fijo; soporta strings, números, fechas y arrays nativamente.
- Shell: `use hotels`, `db.hotels.find().pretty()`, `db.hotels.find({ name: "Grand Hotel" })`.
- Ejemplo `insertMany` de hoteles con campos `id, name, city, country, rating, available_rooms, price_per_night, amenities[], created_at, updated_at` (Grand Hotel, Cozy Inn, Luxury Suites).
- CRUD: `insertMany([...])` · `find().sort({ rating: 1 }).limit(2)` · `updateOne({name: ...}, { $set: { address: "999 St" } })` · `deleteOne({...})`, `deleteMany({ rating: { $lt: 3 } })`. **`skip()` + `limit()` = paginación simple.**

## Paquete DAO (Data Access Object)
Structs Go que mapean a documentos de MongoDB: interacción tipada, seguridad de tipos, abstracción de la persistencia.

## Conexión desde Go (`DEMO/database/connection.go`)
Driver `go.mongodb.org/mongo-driver` **v1.17.9**, Gin **v1.12.0**, `go 1.25.0`, módulo llamado `main`.
```go
client, err := mongo.NewClient(options.Client().ApplyURI("mongodb://root:root@localhost:27017"))
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()
err = client.Connect(ctx)   // log.Fatal si falla
```
`main.go` hace `defer client.Disconnect(context.Background())`.

## El problema de los tests y el Repository Pattern
- Si el código accede directo a Mongo, los tests dependen de una instancia activa, conexión y datos precargados → **lentos, frágiles**. Con interfaces → **mock en memoria**.
- **Tres pasos** (ejemplo hoteles en la presentación):
  1. Interfaz: `type HotelsRepo interface { GetHotelByID(id int64) (HotelDAO, error) }` — genérica, representa necesidades de la app; **no conoce Mongo, mocks ni infraestructura**.
  2. Impl. Mongo: `type HotelsMongoDB struct { client *mongo.Client }`, consulta `client.Database("hotel-platform").Collection("hotels")`.
  3. Impl. Mock: `type HotelsMock struct { docs map[string]HotelDAO }`, busca en memoria.
- **Service depende de la abstracción** (Service → Repository → MongoDB es un detalle de infraestructura). En producción se le inyecta Mongo; en tests, el Mock. "El mismo código del Service funciona con ambas implementaciones. Solo cambia el repositorio que le inyectamos."
- **Producción** (disponibilidad, integridad, performance, concurrencia) vs **Test** (aislamiento, velocidad, reproducibilidad).
- **Principios de repositorios:** independencia del ORM/base; testabilidad; **un repositorio por entidad agregada** (encapsula su CRUD); flexibilidad para cambiar el almacenamiento.
- Ganancias de las interfaces: desacoplamiento, testabilidad, flexibilidad (`HotelsRepo` → `HotelsMongoDB`, `HotelsMock`, `HotelsMySQL`), separación de responsabilidades.

### Organización sugerida (presentación)
```
hotels-api/
├── dao/hotels/hotels_dao.go
├── repositories/hotels/{hotels_mongo.go, hotels_mock.go}
├── services/hotels/{hotels_service.go, hotels_service_test.go}
├── go.mod
└── main.go
```
DAO = estructuras de datos · Repository = acceso a datos (Mongo o Mock) · Service = negocio · Controller = HTTP request/response.

## Ejercicio: API de Items (`CLASE_2/DEMO`)
Consigna: modelar ítems, repositorio Mongo con CRUD (`GetItemByID`, `CreateItem`, `UpdateItem`, `DeleteItem`), service usando la abstracción, controller (recibe request, obtiene params/body, llama al service, devuelve response). "Cada capa debe tener una responsabilidad clara."

Implementación de la demo:
| Archivo | Contenido |
| --- | --- |
| `models/items/items_model.go` | `ItemModel{ID string \`bson:"_id,omitempty" json:"id"\`; Title; Price float64}` |
| `repositories/items/items_repo.go` | `ItemsRepo interface { CreateItem(ItemModel) error; GetItemByID(id string) (ItemModel, error) }` |
| `repositories/items/items_mongo.go` | `ItemsMongoDB{Client *mongo.Client}`; base `items-api`, colección `items`; `FindOne(context.TODO(), bson.M{"_id": id}).Decode`, `InsertOne` |
| `repositories/items/items_mock.go` | `ItemsMock{MockDB map[string]ItemModel}`; error "el ID no puede estar vacío" / "item no encontrado" |
| `services/items/item_service.go` | `ItemsService{Repo repo.ItemsRepo}` ("Inyectamos la interfaz, NO la base de datos real"); `GetItem` valida id vacío |
| `controllers/items/items_controller.go` | `ItemsController{Service}`; `GetByID` → 404 con `gin.H{"error": err.Error()}` o 200 con el item |
| `services/items/item_service_test.go` | `TestGetItem_Success`: mapa en memoria con `"123"` → `ItemsMock` → `ItemsService` → asserts con `t.Errorf` |
| `main.go` | DI: `ConnectDB()` → `ItemsMongoDB{Client}` → `ItemsService{Repo}` → `ItemsController{Service}`; `GET /items/:id` en `:8080`. Bloque comentado alternativo con `ItemsMock` ("¡Magia! El Service lo acepta igual"). Rutas `POST /items` y `PUT /items/:id` comentadas |

Diagrama de capas comentado al final de `main.go`:
```
[Cliente/Frontend] ⇅ DTO
[Controladores]  (mapeo DTO ⇄ Model)  ⇅ Model
[Service]  ⇅ Model
[Repository]
[Acceso a datos (Model)]  (traduce Model a SQL/Query)  ⇅ Rows
[Base de datos]
```
