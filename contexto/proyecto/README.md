# Proyecto final — Rumbo

**Estado (11/10/2026):** Entrega 1 entregada y evaluada (tag `entrega-1`); correcciones del profesor aplicadas (ADR-005 MySQL, ADR-006 contrato genérico v2; detalle en `insumos-arquitectura.md` §6). Próximo hito: Entrega 2 — vie 23/10/2026 (objetivo del grupo: terminarla el vie 16/10). Proyecto **grupal**.

## Archivos
- `consigna.md` — transcripción fiel de `Enunciado TP Final.pdf`.
- `dominio.md` — dominio y alcance de Rumbo (qué es, actores, reglas, estados, búsqueda, fuera de alcance, consultas al profesor). Manda sobre cualquier otra fuente.
- `insumos-arquitectura.md` — ideas técnicas para evaluar en los ADR y lo dicho en la clase de presentación del proyecto.
- `equipo.md` — integrantes y división de trabajo (completar).

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
| Estructura inicial de servicios y dependencias | `servicios/`, `frontend/`, `infra/`, `compose.yaml` (probado con Docker) |

## Pendientes
1. **Agregar a los docentes como colaboradores** del repo (cuando el grupo pase sus usuarios).
2. **Diagramas:** PostgreSQL → MySQL, "esquema" → "base" y la capacidad publicada como "Solicitar traslado" `/v2`.
3. Completar `equipo.md`.
4. Narrar los casos completos de `insumos-arquitectura.md` §4 antes de implementar Operaciones.

## Próximas decisiones (Entrega 2, vie 23/10)
D2 (estilos internos: ya hay una estructura preliminar), D6 (búsqueda), D7 (caché), D9 (consumo del clima, cuando se conozca el contrato), D10 (resiliencia), D12 (balanceo), D13 (capacidad y costos), validación de D1/D3/D5 (en D5: ajustar timeouts con latencias medidas, por la devolución del profesor), primera versión de D11. D4 (consistencia) se apoya en la transacción con bloqueo por recurso prevista en ADR-005 (MySQL). Bonus: anunciarlos en la Entrega 2.
