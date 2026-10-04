# Práctico — Clase 1: Monolito → microservicios, REST, interfaces y mocks

Fuente: repo del docente, `CLASE_1/` (`PRESENTACION/Clase 1.pdf` 19 págs., `DEMO/1.Paso..6.Paso`, `TP/`).

## Presentación
- Docente de práctico: **Tomás Cassanelli** (Ing. en Sistemas; dicta Desarrollo de Software y Arquitectura de Software). Estilo: clases prácticas; "Confianza + compromiso = buenos resultados".
- **Stack ("NECESITAMOS TENERLAS A TODAS!")**: Git, Visual Studio Code, Golang, Chrome/Brave, Node.js (npm, TypeScript, entorno frontend), Postman/Swagger (probar y documentar APIs), SQL Workbench, Docker. Se suman: **MongoDB** (NoSQL documental), **MemCached/Redis** (caché), **Solr** (búsqueda/indexación), **RabbitMQ** (mensajería asíncrona).
- **Arquitectura de software:** cómo se organiza un sistema; de ella dependen calidad, mantenibilidad y escalabilidad.
- **Monolito:** UI, negocio y datos en un único despliegue. Variantes:
  - *Single-process monolith* — todo en un proceso.
  - *Monolito modular* — módulos lógicos, construidos y desplegados juntos.
  - ⚠️ *Monolito distribuido* — **anti-patrón**: servicios separados con alto acoplamiento que obligan a desplegar todo junto.
- **Microservicios:** pequeños, independientes, poco acoplados, **cada uno con su propia base de datos** y despliegue autónomo. Ventajas: escalabilidad independiente, mantenibilidad, flexibilidad (distintos lenguajes), paralelización (equipos autónomos).

## REST en Go (Gin)
- **Regla de oro:** `router.MÉTODO("/entidad_en_plural/:id", Controlador.Acción)`.
- Método HTTP: GET, POST, PUT, PATCH, DELETE. Entidad: sustantivo en plural (`users`, `albums`, `properties`).

## Ejercicios en clase (caso gimnasio)
- **Problema:** sistema de un gimnasio chico (Go, una sola base relacional) que creció: 5 sucursales, clases de Hyrox, funcional y yoga **con cupos estrictos**, tienda online (indumentaria, suplementos), café de especialidad, integración con apps externas, club de beneficios. Despliegues y mantenimiento cada vez más riesgosos.
  1. Listar al menos **tres problemas críticos / cuellos de botella** del monolito.
  2. **Primera decisión:** migrar a microservicios (bajo acoplamiento, alta cohesión). ¿En cuántos dividirían? Nombre + responsabilidad de cada uno (sin código ni bases; "¡no se olviden de la tienda y el café!").
  3. **"A cranearlo":** escribir las líneas del router en Go con **4 endpoints RESTful** para cada dominio: Usuarios (identidad), Actividades (clases/Hyrox), E-Commerce (tienda/café), Beneficios (loyalty).

## Interfaces y mocks entre microservicios
- Caso: microservicio de **Actividades** quiere reservar una clase de Hyrox; por regla de negocio primero pregunta a **Usuarios** si la cuota está al día (GET HTTP por la red).
- **Problema:** si Usuarios no está terminado o no hay red, Actividades queda trabado: "dependemos 100% de que el otro exista y funcione para poder probar nuestro propio código".
- **Solución:** depender de **contratos (interfaces)**, no de implementaciones. Analogía: el **televisor** (controlador) solo conoce el **enchufe estándar** (contrato); se puede conectar a la pared (HTTP real) o a un generador a batería falso (**mock**).

### Demo paso a paso (`CLASE_1/DEMO`, módulo `demo`, Gin v1.12.0, go 1.25.0)
| Paso | Qué se agrega | Código clave |
| --- | --- | --- |
| 1 | **Dominio** (en el TP iría en `/domain`): entidad sin dependencias externas | `type User struct { ID int64; Name string; Active bool }` |
| 2 | **Contrato** (carpeta `/clients`: consumo de APIs de terceros u otros microservicios) | `type UserRepo interface { GetUserByID(id int64) (User, error) }` |
| 3 | **Implementación real** (adaptador HTTP; simula la respuesta) | `type UserHTTP struct{}` + `func (repo UserHTTP) GetUserByID(...)`. Al tener el método, cumple la interfaz implícitamente |
| 4 | **Mock** "salvavidas" para desarrollo local y testing, responde al instante sin red | `type UserMock struct{}` que devuelve `"Socio de Prueba (Mock)"` |
| 5 | **Controlador** (`/controllers`) que depende **de la interfaz** | `type ActivitiesController struct { UserClient UserRepo }`; `Reserve` responde 400 si error o `!user.Active`, 200 con "¡Éxito! Reserva confirmada para: ..." |
| 6 | **main + inyección de dependencias** | `actCtrl := &ActivitiesController{UserClient: UserMock{}}` — "para ir a producción, solo se cambia `UserMock{}` por `UserHTTP{}`". `POST /activities/reserve`, puerto **8082** |

## TP de la clase: "Desarmando el monolito del gimnasio" (`CLASE_1/TP`)
- Base: módulo `practico`, todo en un `main.go` + `controllers/controllers.go`, Gin en `:8080`. Rutas del monolito:
  - Usuarios: `POST /users`, `GET /users/:id`
  - Actividades: `GET /activities`, `POST /activities/:id/reserve`
  - Tienda/Café: `GET /products`, `POST /products/buy`
  - Beneficios: `GET /users/:id/points`, `POST /rewards/redeem`
  - Los handlers solo devuelven `c.String(200, "...")` con textos fijos.
- **Consigna (README):** reescribir el sistema dividido en servicios/módulos ejecutables de forma independiente, cada uno una capacidad de negocio con su propia API. Mínimo: separación por responsabilidades; un proyecto/módulo por dominio; endpoints coherentes; arranque independiente; **validar que funciona con más de un puerto encendido a la vez**.
- **Pista:** "Si la solución solo necesita un puerto para correr, todavía no está resuelta como se espera."
- **Versión de la presentación ("Actividad para la semana que viene"):** dividir en **cuatro** microservicios (Usuarios, Actividades, Tienda E-Commerce/Café, Beneficios Loyalty) y **aplicar interfaces y mocks en al menos uno**. Motivación: "si se rompe el módulo del café, ¡la gente no puede entrar a entrenar!".
- La presentación menciona la demo "en rama `clase-1`"; en el repo actual todo está en `main`, carpeta `CLASE_1`.
