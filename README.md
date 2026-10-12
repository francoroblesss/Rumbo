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
Requisitos: Docker Desktop (con Docker Compose v2). Para desarrollar sin Docker: Go 1.25+ y Node.js 20+.

```bash
cp .env.example .env          # una sola vez; cambiar las contraseñas de desarrollo
docker compose up --build     # levanta infraestructura, microservicios y gateway
```

| Componente | Dirección |
| --- | --- |
| API Gateway (único punto de entrada) | http://localhost:8080 — contrato público en `/v2/...`, API interna en `/api/v1/<servicio>/...` |
| Panel de RabbitMQ | http://localhost:15672 |
| MySQL (por ejemplo, desde SQL Workbench) | `localhost:13306` |
| Aplicación web (desarrollo) | `cd frontend && npm install && npm run dev` → http://localhost:5173 |

## Estructura del repositorio
```
servicios/        un módulo Go por microservicio + el gateway (go.work los agrupa)
  gateway/        API Gateway: ruteo, correlation id, autenticación y límite de tráfico
  identidad/      Identidad y clientes (capas)          → MongoDB
  operaciones/    Operaciones, núcleo (hexagonal)        → MySQL
  recursos/       Recursos y preliquidación (capas)      → MySQL
  notificaciones/ Notificaciones (capas)                 → MongoDB
  integracion/    Integración de carga (capas)           → MongoDB
frontend/         aplicación web (React + Vite + TypeScript)
infra/            scripts de inicialización: una base de datos y un usuario por servicio
docs/             arquitectura, ADR, contrato publicado y postmortem
compose.yaml      levanta todo el sistema
```

## Parte desplegada
> _Pendiente: URL pública de la capacidad publicada (requerida para la Presentación Grupal)._

## Forma de trabajo
El proyecto sigue un enfoque guiado por especificación: primero el dominio y las reglas de negocio (`SPEC.md`), después las decisiones de arquitectura registradas antes de programar (`docs/adr/`) y recién entonces el código. Los cambios de decisión no borran la historia: quedan registrados en nuevos ADR.

### Ramas, revisiones e integración (GitHub Flow)
| Elemento | Regla |
| --- | --- |
| `main` | Siempre funcional: es la versión evaluable. Está protegida: no se hace push directo, solo se integra por pull request. |
| Ramas | Una por tarea, corta y creada desde `main`: `feat/…`, `fix/…`, `docs/…`, `adr/…` (ej. `feat/asignacion-recorridos`, `adr/d6-busqueda`). |
| Commits | En español, con prefijo de tipo: `feat: asignar chofer a recorrido`, `docs: ADR-005 búsqueda`. |
| Pull requests | Cada PR describe qué cambia y qué ADR o funcionalidad (`F-xx` de `SPEC.md`) toca. Se integra a `main` cuando pasa la **verificación automática**. La revisión de otro integrante se suma cuando es posible. |
| Integración continua | GitHub Actions (`.github/workflows/ci.yml`) verifica cada PR: formato, compilación, `go vet` y tests de los seis módulos Go, más lint y build del frontend. |
| Entregas | Cada entrega se marca con un tag en `main`: `entrega-1`, `entrega-2`, `presentacion`. |
| Decisiones | Un ADR ya integrado no se edita: un cambio de decisión se registra con un ADR nuevo que reemplaza al anterior. |

## Documentación
| Documento | Contenido |
| --- | --- |
| [`SPEC.md`](SPEC.md) | Alcance, funcionalidades, reglas de negocio y criterios de aceptación |
| [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) | Arquitectura general: servicios, datos, comunicaciones y despliegue, con diagramas |
| [`docs/adr/`](docs/adr/README.md) | Registro de decisiones arquitectónicas (un ADR por decisión) |
| [`docs/contracts/`](docs/contracts/README.md) | Capacidad publicada: contrato versionado, ejemplos y errores |
| [`docs/POSTMORTEM.md`](docs/POSTMORTEM.md) | Informe de la caída provocada |
