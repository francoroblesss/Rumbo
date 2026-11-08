# Demo de la Presentación Grupal — antes de presentar

Lista para preparar la demo (mié 11 o vie 13/11/2026). El guion del ensayo se completa en el cierre.

## Una hora antes
- [ ] **Nube (Render):** los cuatro monitores de UptimeRobot en "Up" ([ADR-030](adr/ADR-030-monitor-externo-en-render.md)). Si alguno está caído, despertar todo:
  ```sh
  IDENTIDAD_URL=https://rumbo-identidad.onrender.com OPERACIONES_URL=https://rumbo-operaciones.onrender.com \
  INTEGRACION_URL=https://rumbo-integracion.onrender.com sh infra/render/despertar.sh https://rumbo-gateway.onrender.com
  ```
- [ ] **`CLIMA_API_KEY` real** cargada en Render (rumbo-operaciones → Environment) y en el `.env` local. Con `demo`, el proveedor responde `401 INVALID_API_KEY` y todas las alertas salen "no se pudo verificar el clima".
- [ ] **Local:** Docker Desktop iniciado, `.env` con el perfil de observabilidad activado (`COMPOSE_PROFILES`, `OTEL_EXPORTER_OTLP_ENDPOINT` y `LOGS_MYSQL_DSN`) y `docker compose up --build -d --wait`. Web en http://localhost:5173 con los usuarios de `.env`; Grafana en http://localhost:3000.

## Un par de minutos antes
- [ ] **Despertar el servicio de clima del otro grupo.** Ellos también están en el plan gratuito de Render: su documentación dice que el servicio se duerme sin pedidos y que el primer pedido tarda cerca de un minuto. Nuestro cliente espera 1 s por intento ([ADR-012](adr/ADR-012-integracion-clima.md), [ADR-025](adr/ADR-025-clima-en-las-reservas.md)), así que si está dormido cuando el operador pasa un traslado a `EN_CURSO`, sale "no se pudo verificar el clima" (es la degradación decidida, fail-open; se puede mostrar a propósito):
  ```sh
  curl -H "X-API-Key: <CLIMA_API_KEY>" https://clima-aeropuerto.onrender.com/airports/SACO/conditions
  ```
  Con la clave real responde `200`. Que un pedido sin clave válida (`401`) también lo despierte es una deducción nuestra por cómo se comporta Render (todo pedido desde internet despierta al servicio), **no está verificado** contra su servicio: usar la clave.

## Después de la Presentación
- [ ] **Pausar los cuatro monitores de UptimeRobot** (vie 13/11 a la noche). Si quedan activos, la cuota de 750 h del mes se agota alrededor del 15/11 y Render suspende los servicios hasta diciembre ([ADR-030](adr/ADR-030-monitor-externo-en-render.md)). Si la evaluación sigue (Defensa Individual), rehacer la cuenta de horas antes de decidir.
