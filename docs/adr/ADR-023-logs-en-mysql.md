# ADR-023 — Logs en MySQL leídos por Grafana (D11, reemplaza parcialmente a ADR-014)

- **Fecha:** 2026-10-28
- **Estado:** aceptada
- **Reemplaza a:** de [ADR-014](ADR-014-observabilidad.md), la parte de **logs**:
  - el envío a **Loki** por OpenTelemetry (puente `otelslog`);
  - el panel 11 del tablero ("Logs de error", consulta a Loki).
- **Sigue vigente de ADR-014:**
  - los logs JSON por stdout con `servicio`, `instancia`, `correlation_id`, `trace_id` y `span_id`;
  - las métricas y las trazas con OpenTelemetry hacia la imagen `grafana/otel-lgtm` (Prometheus y Tempo);
  - los paneles 1 a 10 del tablero, el SLO y las alertas.

## Contexto
- **Clase práctica 8 (observabilidad):**
  - cada log se guarda como una fila de una tabla de **MySQL** (`app_logs`);
  - **Grafana** la lee con un usuario que **solo tiene `SELECT`** y la muestra junto a las métricas de Prometheus;
  - la métrica detecta la tendencia y el log explica el evento.
- **En la última clase** se pidió que el tablero muestre **principalmente los logs de error**, pero también los de **info**. El tablero de ADR-014 mostraba solo los errores, y desde Loki.
- **El grupo que nos consume** usó el mismo esquema y la cátedra lo aprobó. Su ADR de observabilidad registra un problema:
  - cada log era un `INSERT` sincrónico, y la request esperaba a que terminara;
  - en su prueba de carga, con 100 pedidos simultáneos, el MySQL de logs se quedó sin conexiones, algunas requests pasaron los 2 s y se perdieron logs: el primer límite del sistema fue la escritura de logs.
- **Teórico 7:**
  - una dependencia lenta puede ser peor que una caída;
  - lo que no es crítico no puede arrastrar a lo crítico: hay que aislar la capacidad (**bulkhead**) con colas **acotadas**.
- **Teórico 4:** el `correlation_id` se propaga para reconstruir una operación entre servicios.
- **Regla del grupo:** si el práctico y el teórico chocan, gana el teórico. El práctico simplifica el `INSERT` sincrónico a propósito; acá se toma su idea (MySQL + Grafana con solo lectura) y se cambia cómo se escribe.

## Decisión
1. **Destino de los logs.** Una base `observabilidad` con una tabla `logs`, en un **MySQL aparte** (contenedor `mysql-logs`), separado del MySQL de negocio:
   - columnas: `creado_en`, `nivel`, `servicio`, `instancia`, `mensaje`, `correlation_id`, `trace_id` y `atributos` (JSON con el resto de los campos);
   - índices por tiempo, por nivel y servicio en un período, y por `correlation_id`;
   - el esquema está en `infra/mysql-logs/01-esquema.sql`.

   Va aparte por bulkhead: una ráfaga de logs no le saca capacidad a las transacciones de las reservas (ADR-022), y si el MySQL de logs se cae, el negocio no se entera. Como Grafana, está en el perfil `observabilidad` de `compose.yaml`.
2. **Usuarios con el mínimo permiso** (`infra/mysql-logs/02-usuarios.sh`, contraseñas en `.env`):
   - `rumbo_logs`: solo `INSERT`, para los servicios;
   - `grafana_lector`: solo `SELECT`, para Grafana.
3. **Qué se guarda:** los logs **INFO, WARN y ERROR** de todos los servicios.
   - Los errores son los que se ven primero en el tablero, pero los de info también están, para reconstruir una operación completa.
   - DEBUG no se guarda.
   - Nunca se loguean secretos ni cuerpos completos (`docs/interfaces-internas.md` §7).
4. **Cómo se escribe:** un `slog.Handler` más en `servicios/comun/observabilidad`, combinado con el de stdout con `slog.NewMultiHandler` de la librería estándar (reemplaza el multiplexor propio).
   - El log se **encola** sin esperar, en una cola acotada de 1000.
   - Una goroutine lo **inserta en lotes** de hasta 100 filas, cada 1 s como máximo, con un timeout de 2 s por `INSERT` y **2 conexiones** como tope.
   - Si la cola está llena o MySQL falla, el log se **descarta** y se cuenta en `rumbo_logs_descartados_total{motivo}`. Igual queda en stdout (`docker compose logs`), y se avisa por stdout como mucho una vez por minuto.
   - Al apagar, se vacía la cola antes de cerrar.
   - Se activa con `LOGS_MYSQL_DSN`. Sin esa variable (tests, perfil apagado), solo stdout.
5. **Retención:** un evento de MySQL borra una vez por día los logs de **más de 7 días**. Alcanza para "qué pasó la semana pasada" y la tabla no crece sin límite.
6. **Tablero "Rumbo — estado general":** el panel 11 de Loki se reemplaza por paneles sobre la fuente **Logs (MySQL)** (`infra/grafana/fuente-logs-mysql.yaml`):
   - **11. Errores recientes:** los ERROR del período;
   - **12. Logs por minuto, por nivel:** el salto de ERROR o WARN es la señal para bajar a los logs;
   - **13. Logs descartados:** de Prometheus;
   - **14. Logs:** INFO, WARN y ERROR, con las variables **nivel**, **servicio** y **correlation_id**.

   Desde cada fila, `trace_id` abre la traza en Tempo y `correlation_id` filtra todos los logs de esa operación.
7. **Se saca Loki:** se quitan el puente `otelslog`, el exportador OTLP de logs y su proveedor. La imagen `otel-lgtm` sigue (trae Prometheus, Tempo y Grafana); su Loki queda sin uso.

## Alternativas consideradas
- **Mantener Loki** (ADR-014). Descartada:
  - la cátedra pidió los logs en una base que Grafana lea, como en el práctico;
  - tener los logs en dos lugares duplica sin resolver un problema nuevo.
- **`INSERT` sincrónico por log, como la demo de la clase 8.** Descartada:
  - la request espera a MySQL;
  - es exactamente el límite que encontró el grupo consumidor bajo carga;
  - contradice el teórico 7 (una dependencia no crítica frenando lo crítico).
- **Tabla de logs en el MySQL de negocio** (otra base, misma instancia). Descartada: ahorra un contenedor, pero los logs se escriben todo el tiempo y competirían con las transacciones de las reservas y los bloqueos de D4. Con un contenedor aparte, una ráfaga o una caída quedan aisladas.
- **Un agente que lea stdout y lo cargue en MySQL** (Fluentd o Vector). Descartada: las herramientas estándar no traen salida a MySQL sin plugins, y suman una pieza más que operar para lo mismo que hace un manejador de `slog`.
- **ORM (GORM) para insertar, como el práctico.** Descartada: Rumbo usa `database/sql` con SQL explícito ([ADR-005](ADR-005-mysql-como-motor-relacional.md)), y un `INSERT` de varias filas no necesita más.

## Consecuencias
- **Ganamos:**
  - los logs se consultan con SQL en el mismo tablero que las métricas, filtrados por nivel, servicio u operación;
  - loguear nunca frena una request, aunque el MySQL de logs esté lento o caído;
  - lo que se pierde se ve en una métrica.
- **Pagamos:**
  - un contenedor más (`mysql-logs`, ~512 MB como tope) en el perfil de observabilidad;
  - **los logs pueden perderse** si el MySQL de logs está caído o si la cola se llena (más de 1000 pendientes). Siguen en stdout, pero no en el tablero. Se acepta porque el log no es la fuente de verdad del negocio;
  - un log llega a MySQL hasta 1 s después de escribirse;
  - la "traza" por `correlation_id` es una lista ordenada por tiempo. El árbol de llamadas sigue en Tempo, a un clic del `trace_id`.

## Cómo se verifica
- **Tests** (`servicios/comun/observabilidad/logs_mysql_test.go`):
  - armado de la fila (nivel, columnas, atributos con grupos, recorte del mensaje, DEBUG afuera);
  - lotes de hasta 100;
  - con MySQL trabado, loguear 1150 veces no espera y lo que no entra se cuenta como descartado;
  - con MySQL caído, el lote se cuenta como descartado;
  - **test de integración contra MySQL 8.4 real** (testcontainers) con el mismo `infra/mysql-logs/01-esquema.sql`.
- **Con el sistema levantado** (perfil `observabilidad` y `LOGS_MYSQL_DSN`), probado el 2026-10-28:
  - las reservas dejan filas INFO y WARN de Integración y Operaciones **con el mismo `correlation_id`**;
  - la fuente de Grafana da "Database Connection OK" y las consultas de los paneles 12 y 14 responden;
  - `grafana_lector` no puede borrar ni modificar (`DELETE command denied`);
  - el evento `borrar_logs_viejos` está activo.
- **Caída provocada del MySQL de logs:** las reservas siguieron respondiendo `201` y `409`, y cada servicio avisó por stdout `se descartaron logs que iban a MySQL` con el motivo.
