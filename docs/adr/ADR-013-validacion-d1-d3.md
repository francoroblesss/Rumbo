# ADR-013 — Validación de los límites de servicios (D1) y la persistencia (D3) en la Entrega 2

- **Fecha:** 2026-10-14
- **Estado:** aceptada

## Contexto
La consigna pide en la Entrega 2 **validar** D1 (ADR-001) y D3 (ADR-003, con el motor reemplazado por MySQL en ADR-005) contra lo que efectivamente se construyó. Validar significa contrastar cada decisión con la evidencia del sistema y dejar asentado si se confirma, se ajusta o se reemplaza.

## Validación de D1 — Límites de los servicios (ADR-001)
| Criterio de ADR-001 | Evidencia en la Entrega 2 | Resultado |
| --- | --- | --- |
| La operación crítica es local a un servicio | Aceptar, contraproponer y cancelar una solicitud corren en **una transacción de MySQL** dentro de Operaciones, junto con el outbox (`adaptadores/mysql`). No hay transacciones distribuidas | Se confirma |
| Cada dato tiene un único dueño | Integración guarda solo su copia (vista) y la reemplaza por eventos. Las decisiones (24 h, vencimiento, estados) las toma Operaciones por los endpoints internos (`docs/interfaces-internas.md` §4.1) | Se confirma |
| Ningún servicio lee la base de otro | Usuarios de base por servicio, con acceso denegado a las bases ajenas (verificado en ADR-005) | Se confirma |
| No hay monolito distribuido | Integración sigue aceptando solicitudes con Operaciones detenido (ADR-008). Operaciones procesa el comando cuando vuelve | Se confirma |
| Integración traduce, no decide | El contrato público genérico (`cosa`) se traduce al comando interno (§5.3). Integración no tiene reglas de negocio | Se confirma |

**Ajuste menor (sin cambio de decisión):** Operaciones también consume la capacidad de clima del otro grupo (ADR-012). Eso ya estaba previsto en ADR-001 ("Operaciones → sistema del aeropuerto").

## Validación de D3 — Persistencia (ADR-003 y ADR-005)
| Criterio | Evidencia | Resultado |
| --- | --- | --- |
| Operaciones necesita ACID y relaciones | Tablas `solicitudes`, `traslados`, `outbox`, `claves_idempotencia` y `mensajes_procesados` (migraciones en `adaptadores/mysql/migraciones/`). El cambio de negocio, el evento y la marca de idempotencia se guardan **en la misma transacción** | Se confirma |
| Integración: un documento atómico | Solicitud, clave de idempotencia y `pendiente_envio` en un solo documento de `solicitudes_externas` (ADR-007/009, PR de Integración) | Se confirma |
| Patrones de acceso con índice | Operaciones: `(estado, contrapropuesta_vence)` para el vencimiento, `(organizacion_id, actualizado_en)` y `(estado, actualizado_en)` para los listados, `(publicado_en, id)` para el relay del outbox | Se confirma |
| Una base por microservicio, dos tipos | MySQL (`operaciones`, `recursos`) y MongoDB (`identidad`, `notificaciones`, `integracion`) | Se confirma |

**Ajuste menor (sin cambio de decisión):** el relay del outbox toma las filas pendientes con `SELECT … FOR UPDATE SKIP LOCKED`. Así, si en el futuro hay más de una réplica de Operaciones, no publican la misma fila dos veces. No cambia ninguna decisión de ADR-003/005.

## Decisión
**D1 y D3 se confirman** para la Entrega 2, con los dos ajustes menores registrados arriba. No se reemplaza ningún ADR.
