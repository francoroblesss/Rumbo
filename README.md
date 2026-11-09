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
El dominio completo (solicitudes, planificación de recorridos, asignación de chofer y vehículo, ejecución, incidentes y preliquidación) está en [`SPEC.md`](SPEC.md). Para la **Presentación Grupal** se implementa el flujo de las **reservas de franjas** ([ADR-024](docs/adr/ADR-024-alcance-de-la-presentacion.md)):

1. Un **sistema externo** (el de gestión de almacenamiento, de otro grupo) consulta las franjas con lugar y **reserva el traslado** de una cosa desde un origen hasta un destino, por API y con respuesta inmediata, con el contrato publicado en [`docs/contracts/`](docs/contracts/README.md) (versión vigente: 4.0.1; las rutas no llevan versión, ADR-019).
2. Si el traslado va o viene del aeropuerto, Rumbo revisa el **clima** con el servicio de otro grupo y marca una alerta si no es operable ([ADR-025](docs/adr/ADR-025-clima-en-las-reservas.md)).
3. El **operador** ve en la web las franjas con su ocupación y los traslados reservados, y los avanza: `EN_CURSO` → `COMPLETADO`.
4. El sistema externo consulta su traslado y lo ve avanzar; un **usuario de esa organización** lo ve en la web y puede cancelarlo.
5. **Notificaciones** avisa en tiempo real al operador y a la organización ([ADR-027](docs/adr/ADR-027-avisos-en-tiempo-real.md)).
6. Operador y cliente **buscan traslados** con texto libre y filtros ([ADR-011](docs/adr/ADR-011-busqueda.md)).

## Ejecución local
Requisitos: Docker Desktop (con Docker Compose v2). Para desarrollar sin Docker: Go 1.26+ (o cualquier Go ≥ 1.21, que descarga la 1.26 solo) y Node.js 20+.

```bash
cp .env.example .env          # una sola vez; cambiar las contraseñas de desarrollo
docker compose up --build     # levanta infraestructura, microservicios y gateway
```

Consumo de memoria: el sistema completo usa unos **2,5–3,5 GB** de RAM; conviene darle a Docker Desktop al menos 4 GB. Para ahorrar:
- `docker compose up --build operaciones` levanta solo ese servicio y sus dependencias.
- La **observabilidad** (Grafana LGTM, ~1 GB, y el MySQL de logs) es opcional: se activa descomentando `COMPOSE_PROFILES`, `OTEL_EXPORTER_OTLP_ENDPOINT` y `LOGS_MYSQL_DSN` en `.env`. Sin ella, los servicios igual loguean en JSON por stdout (`docker compose logs`).
- `.env` trae `COMPOSE_PARALLEL_LIMIT=1`: las imágenes se compilan de a una, que es el pico de memoria.
- La **aplicación web** también se puede correr en desarrollo, fuera de Docker y con recarga en caliente: `cd web && npm install && npm run dev` (Node.js 22; queda en http://localhost:5173 contra el gateway de `VITE_API_URL`). Detalle en [`web/README.md`](web/README.md).

| Componente | Dirección |
| --- | --- |
| Aplicación web (operador, cliente y administrador; usuarios de demo abajo) | http://localhost:5173 |
| API Gateway (único punto de entrada) | http://localhost:8080 — contrato público en `/auth/token`, `/disponibilidad` y `/traslados`, API interna en `/api/<servicio>/...` |
| Panel de RabbitMQ | http://localhost:15672 |
| Grafana: logs (MySQL), métricas, trazas y tablero (`admin`/`admin`; solo con el perfil `observabilidad`) | http://localhost:3000 |
| Panel de Solr (búsqueda) | http://localhost:8983 |
| Mock del clima que consume Rumbo | http://localhost:4020 |
| MySQL (por ejemplo, desde SQL Workbench) | `localhost:13306` |

**Usuarios de demostración** (se siembran al arrancar con los valores de `.env`, [`interfaces-internas.md`](docs/interfaces-internas.md) §15): operador (`OPERADOR_EMAIL` / `OPERADOR_PASSWORD`), cliente de la organización `org-almacenamiento` (`CLIENTE_EMAIL` / `CLIENTE_PASSWORD`) y administrador (`ADMIN_EMAIL` / `ADMIN_PASSWORD`). Con el `.env.example` sin cambios son `operador@rumbo.local`, `cliente@almacenamiento.local` y `admin@rumbo.local`. El sistema externo usa `EXTERNO_CLIENT_ID` / `EXTERNO_CLIENT_SECRET` en `POST /auth/token`.

## Estructura del repositorio
```
servicios/        un módulo Go por microservicio + el gateway (go.work los agrupa)
  comun/          piezas compartidas: observabilidad, /salud, mensajería, identidad, secreto interno y errores (docs/interfaces-internas.md)
  gateway/        API Gateway: ruteo, correlation id, autenticación, límite de tráfico, CORS y secreto interno
  identidad/      Identidad y clientes (capas)          → MongoDB
  operaciones/    Operaciones, núcleo (hexagonal)        → MySQL y Solr
  recursos/       Recursos y preliquidación (capas)      → MySQL (esqueleto en esta etapa)
  notificaciones/ Notificaciones (capas)                 → MongoDB
  integracion/    Integración de carga (capas)           → MongoDB
web/              aplicación web (React + Vite + TypeScript + Mantine, ADR-029)
infra/            inicialización de MySQL y MongoDB, NGINX (balanceador), Grafana (tablero y alertas) y Render (despertar)
pruebas/carga/    pruebas de carga con k6 (ADR-015)
pruebas/e2e/      tests de punta a punta contra el sistema levantado (ADR-028; corren en el CI)
pruebas/fallas/   caída provocada de Operaciones (postmortem)
docs/             arquitectura, ADR, contrato publicado, capacidad, evidencias, postmortem y lista de la demo
compose.yaml      levanta todo el sistema
render.yaml       despliegue de la capacidad publicada en Render (ADR-026)
```

## Parte desplegada
**URL pública del API Gateway: https://rumbo-gateway.onrender.com** (capacidad publicada "Reservar traslado", contrato [4.0.1](docs/contracts/README.md)).

Se despliega en **Render**, plan gratuito, con el Blueprint [`render.yaml`](render.yaml) ([ADR-026](docs/adr/ADR-026-despliegue-en-render.md), que reemplaza parcialmente a [ADR-018](docs/adr/ADR-018-despliegue-en-la-nube.md)). Render vuelve a desplegar solo con cada merge a `main`.

| En la nube | Solo en local (`docker compose up`) |
| --- | --- |
| API Gateway, Identidad, Integración y Operaciones (el camino de la capacidad publicada), con MongoDB Atlas, Aiven for MySQL y CloudAMQP | La aplicación web, Notificaciones, Recursos, Solr, Memcached, NGINX con 2 réplicas de Integración y Grafana |

- Solo el gateway se usa desde afuera. Los servicios tienen URL propia en Render, pero sin el secreto interno que pone el gateway responden `401` ([ADR-026](docs/adr/ADR-026-despliegue-en-render.md)).
- Los secretos (`SECRETO_INTERNO`, `TOKEN_SECRETO`, credenciales de las bases y del broker) se cargan en el panel de Render; nunca están en el repositorio.
- **Siempre despierta durante la evaluación:** en el plan gratuito un servicio se duerme a los 15 minutos sin tráfico, y solo lo despierta un pedido desde internet (no el de otro servicio de Render). Por eso un monitor externo (UptimeRobot) le pide `/salud` a los cuatro cada 5 minutos, hasta la Presentación ([ADR-030](docs/adr/ADR-030-monitor-externo-en-render.md)). Si alguna vez quedan dormidos: `IDENTIDAD_URL=https://rumbo-identidad.onrender.com OPERACIONES_URL=https://rumbo-operaciones.onrender.com INTEGRACION_URL=https://rumbo-integracion.onrender.com sh infra/render/despertar.sh https://rumbo-gateway.onrender.com` (~15 s).
- Las rutas `/api/notificaciones/...` y `/api/recursos/...` responden `502` en la nube: esos servicios no se despliegan.

## Forma de trabajo
El proyecto sigue un enfoque guiado por especificación: primero el dominio y las reglas de negocio (`SPEC.md`), después las decisiones de arquitectura registradas antes de programar (`docs/adr/`) y recién entonces el código. Los cambios de decisión no borran la historia: quedan registrados en nuevos ADR.

### Ramas, revisiones e integración (GitHub Flow)
| Elemento | Regla |
| --- | --- |
| `main` | Siempre funcional: es la versión evaluable. Está protegida: no se hace push directo, solo se integra por pull request. |
| Ramas | Una por tarea, corta y creada desde `main`: `feat/…`, `fix/…`, `docs/…`, `adr/…` (ej. `feat/asignacion-recorridos`, `adr/d6-busqueda`). |
| Commits | En español, con prefijo de tipo: `feat: asignar chofer a recorrido`, `docs: ADR-005 búsqueda`. |
| Pull requests | Cada PR describe qué cambia y qué ADR o funcionalidad (`F-xx` de `SPEC.md`) toca. Se integra a `main` cuando pasa la **verificación automática**. La revisión de otro integrante se suma cuando es posible. |
| Integración continua | GitHub Actions (`.github/workflows/ci.yml`) verifica cada PR: formato, compilación, `go vet` y tests de los siete módulos Go (los seis servicios y `comun`), y los tests de punta a punta contra el sistema levantado (`pruebas/e2e`). |
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
| [`docs/capacidad/`](docs/capacidad/informe-presentacion.md) | Informes de las pruebas de carga: [Entrega 2](docs/capacidad/informe-entrega-2.md) (capacidad, caché y balanceo) y [Presentación](docs/capacidad/informe-presentacion.md) (reservas simultáneas sobre la misma franja); scripts en [`pruebas/carga/`](pruebas/carga/README.md) |
| [`docs/evidencias/`](docs/evidencias/presentacion.md) | Evidencias por requisito: [Presentación](docs/evidencias/presentacion.md) y [Entrega 2](docs/evidencias/entrega-2.md) (traza distribuida, logs correlacionados, balanceo y fallas) |
| [`docs/diagramas/`](docs/diagramas/README.md) | Diagramas C4 como código (C4-PlantUML) |
| [`docs/POSTMORTEM.md`](docs/POSTMORTEM.md) | Informe de la caída provocada de Operaciones; evidencia en [`docs/evidencias/postmortem/`](docs/evidencias/postmortem/) |
