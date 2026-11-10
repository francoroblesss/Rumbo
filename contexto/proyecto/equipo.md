# Equipo

Quién hizo qué, por pull request (ver `gh pr list --state merged`). Todos los integrantes tienen que poder explicar todo el sistema, no solo su parte.

| Integrante (GitHub) | Entrega 1 y Entrega 2 | Presentación Grupal |
| --- | --- | --- |
| Franco (`francoroblesss`) | Correcciones de la Entrega 1 y diagramas C4; base común de la Entrega 2 (`servicios/comun`); el sistema sigue aceptando con RabbitMQ caído (ADR-017); rutas sin versión (ADR-019); capacidad de autoservicio, contrato 4.0.0 y D4 (ADR-021, ADR-022); logs en MySQL (ADR-023) | Base de la Presentación (contratos §11–§16, secreto interno, `/salud`, CORS y stream en el gateway, ADR-024); despliegue en Render (ADR-026) y monitor externo (ADR-030); avisos que se recuperan al reconectar el stream; copia del contrato de clima 2.1.0 (ADR-032); cierre de la documentación |
| Sofia (`2424884sofia`) | Capacidad publicada en Integración de carga; consumo del contrato de clima 2.0.1 | Notificaciones con bandeja y avisos en tiempo real (ADR-027); tests de punta a punta en el CI (ADR-028); postmortem de la caída provocada de Operaciones |
| Delap (`Tomidelap`) | Autenticación, API Gateway y observabilidad; capacidad, evidencia de observabilidad y validación de D5 | Perfil y usuarios de demostración en Identidad; aplicación web (ADR-029) |
| Llanca (`santiagollancaman`) | Ciclo de la solicitud en Operaciones y consumo del clima | Operaciones para la Presentación (API de la web, avance de estado, clima en las reservas, ADR-025, contrato 4.0.1); prueba de carga de reservas sobre la misma franja; búsqueda con Solr (F-27) y buckets del histograma de latencia; alertas de circuito abierto y de outbox sin publicar y `GET /disponibilidad` en la prueba de la caída (acciones de mejora 2 y 6 del postmortem); balanceador que no saca réplicas sanas por un `503` de una dependencia (ADR-031) |
