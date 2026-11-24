# Proyecto final — Rumbo

**Estado (23/11/2026):** Entrega 1, Entrega 2 y Presentación Grupal entregadas (tags `entrega-1`, `entrega-2` y `presentacion`). En curso: **Defensa Individual (examen final, 04/12/2026)**. Los cambios pedidos en la devolución de la Presentación están en `main`: búsqueda con todas las palabras (ADR-033), clima en la web (ADR-034), reintento de Integración a Operaciones (ADR-035), logs y paneles de Grafana y los ajustes de ADR-036. Resumen y evidencia en `docs/evidencias/final.md`. Falta: comprobar que Render vuelva (suspendido desde el 20/11 por la cuota del plan gratuito), medir con el sistema levantado los arreglos de ADR-036 y el tag `final`. El examen es **individual**.

## Archivos
- `consigna.md` — transcripción fiel de `Enunciado TP Final.pdf`.
- `dominio.md` — dominio y alcance de Rumbo (qué es, actores, reglas, estados, búsqueda, fuera de alcance, consultas al profesor). Manda sobre cualquier otra fuente.
- `insumos-arquitectura.md` — ideas técnicas para evaluar en los ADR y lo dicho en la clase de presentación del proyecto.
- `equipo.md` — integrantes y quién hizo qué.

## Documentación entregable
La estructura de la consigna §3 está en la raíz del repo: `README.md`, `SPEC.md`, `docs/ARCHITECTURE.md` (con diagramas en `docs/diagramas/`), `docs/adr/` (índice D1–D13 y formato de ADR; el formato de la "sección 8" que cita la consigna no existe, así que se usa uno propio), `docs/contracts/` (contrato OpenAPI + documentación + mock) y `docs/POSTMORTEM.md`. **Se actualiza durante el desarrollo**, no al final.

## Entrega 1 — Diseño y contrato con mock (vie 9/10)
| Punto | Dónde |
| --- | --- |
| README, alcance y arquitectura (1ª versión) | `README.md`, `SPEC.md`, `docs/ARCHITECTURE.md` |
| Diagramas de contexto y contenedores | `docs/diagramas/` |
| Límites de servicios, responsabilidades y datos (D1) | ADR-001 |
| Capacidad propia documentada + mock (D8) | ADR-002 (contrato vigente: ADR-006, v2), `docs/contracts/` |
| D3 y D5 iniciales | ADR-003 (motor: ADR-005, MySQL), ADR-004 |
| Estructura inicial de servicios y dependencias | `servicios/`, `frontend/` (hoy `web/`, ADR-029), `infra/`, `compose.yaml` (probado con Docker) |

## Presentación Grupal (mié 11 o vie 13/11)
| Punto de la consigna | Dónde |
| --- | --- |
| Evidencia por requisito | `docs/evidencias/presentacion.md` |
| Arquitectura y limitaciones (§10) | `docs/ARCHITECTURE.md` |
| Capacidad propia en la nube | `README.md` ("Parte desplegada"), `docs/contracts/README.md`, ADR-026, ADR-030 |
| Postmortem de la caída provocada | `docs/POSTMORTEM.md` |

## Defensa Individual (examen final, 04/12/2026)
| Punto | Dónde |
| --- | --- |
| Devolución de la Presentación → qué se cambió, y evidencia por requisito | `docs/evidencias/final.md` |
| Decisiones nuevas | ADR-033 a ADR-036 |
| Arquitectura y limitaciones al día | `docs/ARCHITECTURE.md` (§10, "Cambios del final") |

## Decisiones pendientes en la Entrega 2 (histórico)
D2 (estilos internos: ya hay una estructura preliminar), D6 (búsqueda), D7 (caché), D9 (consumo del clima, cuando se conozca el contrato), D10 (resiliencia), D12 (balanceo), D13 (capacidad y costos), validación de D1/D3/D5 (en D5: ajustar timeouts con latencias medidas, por la devolución del profesor), primera versión de D11. D4 (consistencia) se apoya en la transacción con bloqueo por recurso prevista en ADR-005 (MySQL). Bonus: anunciarlos en la Entrega 2.
