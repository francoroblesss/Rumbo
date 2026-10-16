# Rumbo

Plataforma de gestión operativa de **traslados programados de personas o carga**. Proyecto integrador de Arquitectura de Software (Ingeniería en Informática, UCC, 2026).

## Dominio y objetivo
Rumbo presta servicios de traslado a organizaciones (clínicas, colegios, empresas y empresas de stock) con **choferes y vehículos propios**. El sistema permite:

- recibir solicitudes de traslado, puntuales o recurrentes;
- planificar recorridos que agrupan varios traslados;
- asignar chofer y vehículo según disponibilidad, capacidad y accesibilidad;
- seguir la ejecución de cada recorrido y registrar sus incidentes;
- registrar la actividad de los choferes para su preliquidación mensual.

## Flujo principal
1. Un **cliente** solicita un traslado: qué personas o qué carga, origen, destino, hora y si es puntual o recurrente.
2. Un **operador** revisa la solicitud: la acepta, propone otro horario o la rechaza.
3. El operador **planifica los recorridos** y **asigna chofer y vehículo**. Rumbo valida que no haya compromisos superpuestos, que alcance la capacidad y que se cumplan los horarios comprometidos.
4. El **chofer** ejecuta el recorrido: inicia, marca paradas, abordajes y ausencias, reporta incidentes y cierra informando los km.
5. El cliente consulta el estado de sus traslados. La actividad registrada alimenta la **preliquidación** de los choferes.

Además, otros sistemas pueden **solicitar el traslado de una cosa desde un origen hasta un destino** a través del contrato publicado en [`docs/contracts/`](docs/contracts/README.md) (versión vigente: `/v2`).

## Ejecución local
Requisitos: Docker Desktop (con Docker Compose v2). Para desarrollar sin Docker: Go 1.26+ (o cualquier Go ≥ 1.21, que descarga la 1.26 solo) y Node.js 20+.

```bash
cp .env.example .env          # una sola vez; cambiar las contraseñas de desarrollo
docker compose up --build     # levanta infraestructura, microservicios y gateway
```

Consumo de memoria: el sistema completo usa unos **2,5–3,5 GB** de RAM; conviene darle a Docker Desktop al menos 4 GB. Para ahorrar:
- `docker compose up --build operaciones` levanta solo ese servicio y sus dependencias.
- La **observabilidad** (Grafana LGTM, ~1 GB) es opcional: se activa descomentando `COMPOSE_PROFILES` y `OTEL_EXPORTER_OTLP_ENDPOINT` en `.env`. Sin ella, los servicios igual loguean en JSON por stdout (`docker compose logs`).
- `.env` trae `COMPOSE_PARALLEL_LIMIT=1`: las imágenes se compilan de a una, que es el pico de memoria.

| Componente | Dirección |
| --- | --- |
| API Gateway (único punto de entrada) | http://localhost:8080 — contrato público en `/v2/...`, API interna en `/api/v1/<servicio>/...` |
| Panel de RabbitMQ | http://localhost:15672 |
| Grafana: logs, métricas, trazas y tablero (`admin`/`admin`; solo con el perfil `observabilidad`) | http://localhost:3000 |
| Mock del clima que consume Rumbo | http://localhost:4020 |
| MySQL (por ejemplo, desde SQL Workbench) | `localhost:13306` |
| Aplicación web: consola del operador (desarrollo; ingreso con `ADMIN_EMAIL` / `ADMIN_PASSWORD` de `.env`) | `cd frontend && npm install && npm run dev` → http://localhost:5173 |

## Estructura del repositorio
```
servicios/        un módulo Go por microservicio + el gateway (go.work los agrupa)
  comun/          piezas compartidas: observabilidad, mensajería, identidad y errores (docs/interfaces-internas.md)
  gateway/        API Gateway: ruteo, correlation id, autenticación y límite de tráfico
  identidad/      Identidad y clientes (capas)          → MongoDB
  operaciones/    Operaciones, núcleo (hexagonal)        → MySQL
  recursos/       Recursos y preliquidación (capas)      → MySQL
  notificaciones/ Notificaciones (capas)                 → MongoDB
  integracion/    Integración de carga (capas)           → MongoDB
frontend/         aplicación web (React + Vite + TypeScript)
infra/            inicialización de MySQL y MongoDB, NGINX (balanceador) y Grafana (tablero y alertas)
pruebas/carga/    pruebas de carga con k6 (ADR-015)
docs/             arquitectura, ADR, contrato publicado, capacidad, evidencias y postmortem
compose.yaml      levanta todo el sistema
```

## Parte desplegada
La aplicación web se publica en **Vercel** y el backend en **Railway**; solo el API Gateway queda expuesto ([ADR-018](docs/adr/ADR-018-despliegue-en-la-nube.md)).
> _Pendiente: la URL pública se agrega acá y en [`docs/contracts/README.md`](docs/contracts/README.md) cuando se despliegue (requerida para la Presentación Grupal)._

## Forma de trabajo
El proyecto sigue un enfoque guiado por especificación: primero el dominio y las reglas de negocio (`SPEC.md`), después las decisiones de arquitectura registradas antes de programar (`docs/adr/`) y recién entonces el código. Los cambios de decisión no borran la historia: quedan registrados en nuevos ADR.

### Ramas, revisiones e integración (GitHub Flow)
| Elemento | Regla |
| --- | --- |
| `main` | Siempre funcional: es la versión evaluable. Está protegida: no se hace push directo, solo se integra por pull request. |
| Ramas | Una por tarea, corta y creada desde `main`: `feat/…`, `fix/…`, `docs/…`, `adr/…` (ej. `feat/asignacion-recorridos`, `adr/d6-busqueda`). |
| Commits | En español, con prefijo de tipo: `feat: asignar chofer a recorrido`, `docs: ADR-005 búsqueda`. |
| Pull requests | Cada PR describe qué cambia y qué ADR o funcionalidad (`F-xx` de `SPEC.md`) toca. Se integra a `main` cuando pasa la **verificación automática**. La revisión de otro integrante se suma cuando es posible. |
| Integración continua | GitHub Actions (`.github/workflows/ci.yml`) verifica cada PR: formato, compilación, `go vet` y tests de los siete módulos Go (los seis servicios y `comun`), más lint y build del frontend. |
| Entregas | Cada entrega se marca con un tag en `main`: `entrega-1`, `entrega-2`, `presentacion`. |
| Decisiones | Un ADR ya integrado no se edita: un cambio de decisión se registra con un ADR nuevo que reemplaza al anterior. |

## Documentación
| Documento | Contenido |
| --- | --- |
| [`SPEC.md`](SPEC.md) | Alcance, funcionalidades, reglas de negocio y criterios de aceptación |
| [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) | Arquitectura general: servicios, datos, comunicaciones y despliegue, con diagramas |
| [`docs/adr/`](docs/adr/README.md) | Registro de decisiones arquitectónicas (un ADR por decisión) |
| [`docs/contracts/`](docs/contracts/README.md) | Capacidad publicada: contrato versionado, ejemplos y errores; contratos consumidos en `docs/contracts/consumidos/` |
| [`docs/interfaces-internas.md`](docs/interfaces-internas.md) | Contrato interno entre servicios: mensajes, endpoints internos, identidad, errores, logs, métricas y trazas |
| [`docs/capacidad/`](docs/capacidad/informe-entrega-2.md) | Informe de las pruebas de carga (capacidad, caché y balanceo); scripts en [`pruebas/carga/`](pruebas/carga/README.md) |
| [`docs/evidencias/`](docs/evidencias/entrega-2.md) | Evidencia de observabilidad: traza distribuida, logs correlacionados, balanceo y pruebas de fallas |
| [`docs/diagramas/`](docs/diagramas/README.md) | Diagramas C4 como código (C4-PlantUML) |
| [`docs/POSTMORTEM.md`](docs/POSTMORTEM.md) | Informe de la caída provocada |
