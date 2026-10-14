# ADR-009 — Balanceo de carga de Integración de carga (D12)

- **Fecha:** 2026-10-14
- **Estado:** aceptada

## Contexto
La consigna (§2.3) pide balancear al menos un servicio desplegado en **dos o más instancias**, que el mecanismo **reconozca cuando una instancia no está disponible**, y que la distribución y el estado de las instancias se puedan **comprobar con la observabilidad**.

Integración de carga es el servicio de la capacidad publicada (ADR-006): lo usa un sistema de otro grupo, recibe el polling de seguimiento y es el que se despliega para la Presentación. Además es **stateless**: su estado está en MongoDB y Memcached, no en la memoria del proceso.

Del teórico 6 y la clase práctica 7:
- un balanceador responde "¿a qué réplica sana envío esta request?";
- **round robin** cuando las réplicas tienen capacidad y costo similares;
- *health checks* para sacar de rotación una réplica caída;
- servicios **stateless** para no depender de sesiones pegajosas.

## Decisión
- **NGINX** (`nginx:alpine`, como en la clase práctica 7) como balanceador **interno** delante de **dos réplicas**, `integracion-1` e `integracion-2`. El API Gateway envía el tráfico de `/v2/traslados` a `balanceador-integracion`, no a una réplica.
- **Algoritmo:** **round robin**, porque son réplicas idénticas, con el mismo código y los mismos recursos.
- **Detección de réplicas caídas** (*health checks* pasivos de NGINX): `max_fails=3 fail_timeout=30s`. Tres errores de conexión o timeout dentro de 30 s sacan a la réplica de rotación durante 30 s. Después NGINX vuelve a probarla.
- **Reintento en otra réplica:** solo para errores de conexión, timeout, `502` y `503` (`proxy_next_upstream`), como máximo 1 réplica más, y **solo en métodos idempotentes**. NGINX no reintenta `POST` salvo que se configure `non_idempotent`, y **no se configura**: ADR-004 deja el reintento de escrituras en manos del consumidor, con su `Idempotency-Key`.
- **Timeouts:** conexión 1 s y lectura 5 s, alineados con el deadline del gateway (ADR-004).
- **Conexiones reutilizadas:** `keepalive 32` hacia las réplicas.
- **Evidencia observable de la distribución:**
  - el log de acceso de NGINX registra la réplica que atendió cada request (`$upstream_addr`) y el `X-Correlation-ID`;
  - cada réplica identifica sus logs y su métrica `rumbo_http_duracion_segundos` con el atributo `instancia` (`integracion-1` / `integracion-2`), así el tablero muestra las requests por réplica.
- **Trabajo en segundo plano con dos réplicas:**
  - **relay:** cada réplica **reclama** una solicitud pendiente de forma atómica (`findOneAndUpdate` con un vencimiento de 30 s) antes de publicarla, para que las dos no la envíen dos veces;
  - **consumidor de estados:** las dos réplicas consumen la misma cola y compiten por los mensajes, y el control por `agregado_version` hace que el resultado sea el mismo sin importar cuál procese.

## Alternativas consideradas
- **Least connections.** Descartada por ahora: las requests de Integración tienen duración parecida. Se puede revisar si las pruebas de carga muestran desbalance.
- **IP hash o sesiones pegajosas.** Descartada: Integración es stateless; atar un cliente a una réplica le quita disponibilidad cuando esa réplica cae.
- **Balancear Operaciones.** Descartada para esta etapa: tiene outbox, tareas programadas y consumidores que requieren coordinación entre réplicas. No es la pieza expuesta a otros grupos.
- **HAProxy o Envoy.** Descartados: NGINX alcanza y es el visto en clase.

## Consecuencias
- **Ganamos:**
  - la capacidad publicada sigue disponible si cae una réplica;
  - se puede escalar horizontalmente agregando réplicas;
  - la distribución se puede demostrar con logs y métricas.
- **Pagamos:**
  - un salto de red más (gateway → NGINX → réplica);
  - los health checks son pasivos: la primera request que llega a una réplica caída falla (y se reintenta en la otra si es idempotente) antes de que NGINX la saque de rotación;
  - el relay necesita coordinación entre réplicas (reclamo atómico).

## Cómo se verifica
- Con las dos réplicas arriba, 20 `GET` seguidos se reparten entre `integracion-1` e `integracion-2` (log de NGINX y métrica por instancia).
- Con `docker compose stop integracion-1`, los `GET` siguen respondiendo y el log muestra que solo atiende `integracion-2`. Al levantarla, vuelve a recibir tráfico.
