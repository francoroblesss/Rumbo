# ADR-010 — Arquitectura interna de los servicios (D2)

- **Fecha:** 2026-10-14
- **Estado:** aceptada

## Contexto
La consigna (§2.1) exige que **al menos dos servicios tengan patrones de arquitectura diferentes**, trabajados en la materia y aplicados de forma reconocible (Presentación Grupal). D2 pide justificar los estilos elegidos, las dependencias internas y por qué cada alternativa se ajusta al problema de cada servicio.

El teórico 8 aclara que cada estilo responde una pregunta distinta:
- **capas / MVC:** sigue siendo válido cuando hay pocas reglas y consultas simples;
- **Hexagonal / Clean:** sirve cuando el núcleo depende de bases, frameworks o proveedores que cambian, y hay que proteger las reglas;
- **DDD:** aporta el lenguaje, los agregados y las invariantes;
- **CQRS** y **Event Sourcing:** solo se justifican si lectura y escritura divergen o si el historial es valioso.

## Decisión

### Operaciones → **Arquitectura hexagonal (puertos y adaptadores)**, con modelo de dominio
Operaciones es el núcleo (ADR-001):
- tiene **las reglas e invariantes más complejas**: ciclo de la solicitud, contrapropuesta que vence a las 24 h, cancelación con plazo, estados derivados del traslado, alerta climática;
- **depende de más cosas externas** que cualquier otro servicio: MySQL, RabbitMQ, Recursos, Identidad y el sistema de clima de otro grupo, cuyo contrato no controlamos.

Por eso el núcleo no puede depender de esos detalles.

| Capa | Paquete | Contenido | Puede importar |
| --- | --- | --- | --- |
| Dominio | `dominio/` | `Solicitud` (agregado raíz) con su `Traslado`, objetos de valor (`Cosa`, `Ubicacion`, `Contrapropuesta`, `AlertaClimatica`), estados y **métodos que validan cada transición** y devuelven errores de dominio. **Sin tags `json` ni `db`.** El tiempo se recibe como parámetro (`ahora`) | solo la biblioteca estándar |
| Puertos | `puertos/` | Interfaces de **entrada** (lo que ofrece Operaciones: los casos de uso) y de **salida** (lo que necesita: `UnidadDeTrabajo` con repositorio de solicitudes, outbox, claves de idempotencia y mensajes procesados en la misma transacción; `ConsultaClima`) | `dominio` |
| Aplicación | `aplicacion/` | Casos de uso: registrar solicitud externa, aceptar, contraproponer, rechazar, responder contrapropuesta, cancelar, resolver cancelación tardía, vencer contrapropuestas. Cada uno: abrir la unidad de trabajo → cargar el agregado con bloqueo → invocar al dominio → guardar → agregar el evento al outbox | `dominio`, `puertos` |
| Adaptadores | `adaptadores/api`, `adaptadores/mysql`, `adaptadores/mensajeria`, `adaptadores/clima` | HTTP (Gin) → casos de uso; MySQL (`database/sql`) → repositorios y outbox; RabbitMQ → consumidor del comando y relay del outbox; HTTP al proveedor de clima → `ConsultaClima` | `puertos`, `dominio`, librerías |
| Composición | `main.go` | Único lugar que conoce todo y conecta las piezas (inyección manual de dependencias) | todo |

**Regla de dependencias:** el código depende **hacia adentro**. Ni `dominio` ni `aplicacion` importan Gin, `database/sql`, `amqp091-go` ni el cliente HTTP del clima. Los adaptadores implementan los puertos (teórico 8, correspondencia con DSW: handler de Gin = adaptador de entrada, interfaz de repositorio = puerto de salida, struct sin tags = agregado).

**De DDD** se toma lo táctico que resuelve un problema concreto: la `Solicitud` es el **agregado** que protege sus invariantes. Una transacción cambia **un solo agregado**, y la coordinación con otros servicios va por eventos (teórico 8). **No se usa** CQRS con almacenes separados ni Event Sourcing (ADR-003: el dominio pide estados, no reconstruir historia).

### Identidad y clientes, Recursos, Notificaciones e Integración de carga → **Arquitectura en capas**
Cada uno tiene lógica más acotada, una sola base de datos y pocas dependencias:
- `controllers/` (Gin) → `services/` (reglas, dependen de **interfaces**) → `repositories/` (interfaz + implementación + mock);
- `models/` para lo que se persiste (con tags);
- `clients/` y `messaging/` cuando el servicio habla con otros.

Es el estilo de la clase práctica 5 (constructores y `context.Context` como primer parámetro), que el equipo ya conoce. Hexagonal en estos servicios sumaría contratos y conversiones sin un cambio real que proteger (teórico 8: "interfaces sin variación relevante pueden ser sobreingeniería").

### API Gateway → **Proxy con cadena de middlewares**
No tiene negocio. Es una cadena: correlación → trazas → bloqueo de rutas internas → autenticación → límite de tráfico → reverse proxy.

### Piezas compartidas
Lo transversal que no es negocio (observabilidad, mensajería con reintentos y DLQ, identidad propagada y errores Problem Details) está en el módulo `servicios/comun`, usado por todos, con su contrato en `docs/interfaces-internas.md`.

## Alternativas consideradas
- **Todos en capas.** Descartada: no cumple la consigna, y en Operaciones el núcleo quedaría atado a MySQL, RabbitMQ y al proveedor de clima.
- **Todos en hexagonal.** Descartada: es sobreingeniería en servicios simples (teórico 8).
- **Clean Architecture con presenters y output boundaries en Operaciones.** Descartada: Hexagonal ya da la misma regla de dependencias con menos piezas. Los casos de uso devuelven la vista y el adaptador HTTP la serializa.
- **CQRS con modelo de lectura separado en Operaciones.** Descartada en esta etapa: lectura y escritura usan la misma forma de la solicitud. La búsqueda (D6) es un índice derivado, no un segundo modelo de escritura.

## Consecuencias
- **Ganamos:**
  - las reglas de Operaciones se prueban sin base ni broker (tests de dominio y de casos de uso con dobles);
  - cambiar el proveedor de clima o el motor de base de datos toca solo un adaptador (ya pasó con PostgreSQL → MySQL, ADR-005);
  - la diferencia de estilos responde al problema de cada servicio.
- **Pagamos:**
  - más archivos y conversiones (agregado ↔ fila, agregado ↔ JSON) en Operaciones;
  - dos estilos de código en el mismo repositorio, que el equipo tiene que reconocer.

## Cómo se verifica
- `dominio/` y `aplicacion/` no importan paquetes de infraestructura: se revisa con `go list -deps ./dominio ./aplicacion`, que no tiene que mostrar `gin`, `database/sql`, `amqp091-go` ni `net/http`.
- Hay tests de dominio y de casos de uso que corren sin Docker.
