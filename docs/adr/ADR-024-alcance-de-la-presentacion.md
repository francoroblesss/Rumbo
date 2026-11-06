# ADR-024 — Alcance implementado para la Presentación Grupal

- **Fecha:** 2026-11-06
- **Estado:** aceptada
- **Reemplaza parcialmente a:** [ADR-001](ADR-001-limites-de-los-servicios.md) (Recursos queda sin funcionalidad en esta etapa), [ADR-011](ADR-011-busqueda.md) (filtros de la búsqueda) y [ADR-012](ADR-012-consumo-clima.md) (la consulta "al aceptar" deja de existir; dónde se consulta ahora está en [ADR-025](ADR-025-clima-en-las-reservas.md)).

## Contexto
- La Presentación Grupal (consigna §6) exige el frontend completo hasta la funcionalidad compartida, la capacidad propia desplegada, la del proveedor en el flujo principal, al menos tres servicios operativos, dos almacenamientos, caché medida, dos estilos internos, un evento de dominio con consumidor idempotente y DLQ, y búsqueda indexada.
- `SPEC.md` describe 29 funcionalidades: recorridos, asignación, ejecución del chofer, planes recurrentes, padrón y preliquidación. Es el dominio completo de Rumbo. Quedan entre 4 y 6 días para la Presentación.
- Desde [ADR-021](ADR-021-capacidad-de-autoservicio.md), el sistema externo **reserva una franja** y recibe la respuesta en el momento: no hay solicitudes que un operador revise ni contrapropuestas. El grupo acordó con el profesor que la capacidad publicada funcione así. El flujo de revisión de la Entrega 2 (aceptar, contraproponer, rechazar, vencer) quedó sin nadie que lo use.
- El teórico 1 recuerda el costo de cada servicio más (deploys, monitoreo, puntos de falla) y la cátedra penaliza agregar piezas que no responden a un problema concreto.

## Decisión
**1. El flujo de punta a punta de la Presentación es el de las reservas de franjas:**
1. el sistema externo consulta franjas y reserva por la API pública;
2. Operaciones revisa el clima de los traslados al aeropuerto (ADR-025);
3. el operador ve en la web las franjas con su ocupación y los traslados reservados, y los avanza: `PROGRAMADO` → `EN_CURSO` → `COMPLETADO`;
4. el sistema externo ve el avance por consulta, y un usuario de su organización lo ve en la web y puede cancelar;
5. Notificaciones avisa en tiempo real ([ADR-027](ADR-027-avisos-en-tiempo-real.md));
6. Solr indexa los traslados y la web los busca.

**2. Se implementa:**

| Pieza | Qué |
| --- | --- |
| Operaciones | API de la web (`docs/interfaces-internas.md` §12), clima en las reservas (§13), búsqueda (§16) |
| Notificaciones | Bandeja y stream SSE (§14) |
| Identidad | Perfil (`/yo`) y usuarios de demostración sembrados (§15) |
| Aplicación web | Roles **operador**, **cliente de organización** y **administrador** ([ADR-029](ADR-029-aplicacion-web.md)) |
| Integración y gateway | Sin cambios de negocio. El gateway suma CORS, el stream y el secreto interno ([ADR-026](ADR-026-despliegue-en-render.md)) |

**3. Ciclo reducido del traslado:** se producen `PROGRAMADO`, `EN_CURSO`, `COMPLETADO` y `CANCELADO`. El operador hace los avances desde la web, como simplificación de la ejecución del chofer.
- `ASIGNADO`, `NO_REALIZADO` e `INTERRUMPIDO` **siguen en el enum del contrato público**, con la aclaración "no se producen en esta versión" (contrato 4.0.1).
- Sacarlos sería un cambio incompatible (versión mayor 5.0.0) para un consumidor que ya los contempla. Dejarlos no le cuesta nada.

**4. Se saca el código del flujo de revisión de solicitudes:** endpoints de aceptación, contrapropuesta, rechazo y resolución de cancelación tardía, la tarea de vencimiento, sus casos de uso, sus métodos de dominio y los eventos que solo emitían ellos.
- Queda en el historial de Git (tag `entrega-2`).
- Código que nadie llama no se puede demostrar ni defender, y confunde a quien lee el núcleo.

**5. Queda documentado pero no implementado:**

| Qué | `SPEC.md` |
| --- | --- |
| Solicitudes de traslado desde la web, contrapropuestas y su vencimiento | F-02 a F-05, F-08, F-13 |
| Planes recurrentes | F-03, F-09, F-25 |
| Recorridos, asignación de chofer y vehículo, ejecución del chofer e incidentes | F-10 a F-12, F-14 a F-18 |
| Padrón de personas | F-01 |
| ABM de usuarios, clientes, choferes, vehículos y tarifas; preliquidación | F-19 a F-24 |
| Rol chofer en la web | — |

- **Recursos** queda en esqueleto (arranca y responde `/salud`). Sin recorridos no tiene a quién darle disponibilidad ni actividad que registrar. El servicio y su base se mantienen, porque el límite de ADR-001 sigue siendo correcto para cuando se implementen.
- El **evento de dominio** de la Presentación es `operaciones.solicitud.aceptada` (la reserva). Lo consumen cuatro colas: Integración (copia), Notificaciones (aviso), clima (ADR-025) y el indexador de Solr. `operaciones.recorrido.finalizado` → Recursos queda para cuando haya recorridos.

**6. Búsqueda:** se implementan los filtros que tienen datos: texto, organización, estado, rango de fechas y alerta climática, con orden por fecha o relevancia y paginación. `tipo_servicio`, `chofer_id` y `vehiculo_id` de ADR-011 no se implementan: con esta etapa estarían siempre vacíos.

## Alternativas consideradas
- **Implementar recorridos, asignación y ejecución del chofer** (el plan del grupo del 28/10).
  - Descartada por plazo y por necesidad: la consigna ya se cumple sin eso. La operación crítica con garantías de consistencia es la reserva de franja (ADR-022, test de 20 reservas simultáneas sobre 3 lugares) y el evento de dominio con consumidor idempotente y DLQ ya existe.
  - Era cerca del 40 % del trabajo restante, con el riesgo de llegar con todo a medias.
- **Dejar el código de la revisión sin usar.** Descartada (punto 4).
- **Sacar del contrato los estados que no se producen.** Descartada (punto 3).
- **Solicitudes desde la web del cliente.** Descartada por el grupo: duplicaría la capacidad publicada con otro flujo (revisión y contrapropuesta) que el profesor pidió no usar en la integración.

## Consecuencias
- **Ganamos:**
  - un flujo completo y demostrable de punta a punta que toca todos los requisitos de §6: capacidad, clima, web, mensajería, búsqueda, tiempo real, nube;
  - menos código muerto en el núcleo;
  - tiempo para la evidencia, el postmortem y el despliegue.
- **Pagamos:**
  - el dominio implementado es una parte chica del documentado. En la defensa se explica qué quedó fuera y por qué, con este ADR;
  - Recursos aparece en la arquitectura sin funcionalidad;
  - la ejecución real del chofer se reemplaza por un avance manual del operador.

## Cómo se verifica
- La tabla "Implementación para la Presentación" de `SPEC.md` dice el estado de cada F-xx, y coincide con lo que se ve en la web.
- El test e2e ([ADR-028](ADR-028-tests-de-punta-a-punta.md)) recorre el flujo del punto 1.
- `grep` de `Contraproponer`, `VencerContrapropuestas` y `/aceptacion` en `servicios/` no encuentra nada.
