# Proyecto final — Rumbo

**Estado (4/10/2026):** Entrega 1 lista en el repo local; **falta el primer commit y push**. Próximo hito: Entrega 1 — vie 9/10/2026. Proyecto **grupal**.

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
| Capacidad propia documentada + mock (D8) | ADR-002, `docs/contracts/` |
| D3 y D5 iniciales | ADR-003, ADR-004 |
| Estructura inicial de servicios y dependencias | `servicios/`, `frontend/`, `infra/`, `compose.yaml` (probado con Docker) |

## Pendientes
1. **Primer commit y push** (repo público). Quedan solo locales: `.env`, `Enunciado TP Final.pdf` y la transcripción del audio.
2. **Agregar a los docentes como colaboradores** del repo (lo hace el grupo).
3. **Diagrama de contenedores:** cambiar "esquema operaciones/recursos" por "base operaciones/recursos".
4. ~~Método de trabajo con Git~~ definido (README → Forma de trabajo). Falta **proteger `main` en GitHub** (Settings → Branches: requerir PR y que pase el CI).
5. Completar `equipo.md`.
6. Narrar los casos completos de `insumos-arquitectura.md` §4 antes de implementar Operaciones.

## Próximas decisiones (Entrega 2, vie 23/10)
D2 (estilos internos: ya hay una estructura preliminar), D6 (búsqueda), D7 (caché), D9 (consumo del clima, cuando se conozca el contrato), D10 (resiliencia), D12 (balanceo), D13 (capacidad y costos), validación de D1/D3/D5, primera versión de D11. D4 (consistencia) se apoya en la transacción con bloqueo por recurso prevista en ADR-005 (MySQL). Bonus: anunciarlos en la Entrega 2.
