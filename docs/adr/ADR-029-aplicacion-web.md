# ADR-029 — Aplicación web (React + Vite + TypeScript con Mantine)

- **Fecha:** 2026-11-06
- **Estado:** aceptada
- **Relacionado:** [ADR-021](ADR-021-capacidad-de-autoservicio.md) (la web no ofrece la capacidad publicada), [ADR-024](ADR-024-alcance-de-la-presentacion.md) (roles y pantallas de esta etapa), [ADR-026](ADR-026-despliegue-en-render.md) (la web corre en local) y [ADR-027](ADR-027-avisos-en-tiempo-real.md) (tiempo real).

## Contexto
- La consigna (§2.2 y §6) pide **una interfaz web que centralice la interacción del usuario** y vaya "desde el ingreso del usuario hasta la funcionalidad compartida".
- La consola de la Entrega 2 se eliminó: revisaba solicitudes, un flujo que ya no existe (ADR-021, ADR-024).
- **Roles de esta etapa** (ADR-024): **operador** (franjas, traslados, avance de estado, cancelación, búsqueda y bandeja), **cliente de organización** (los traslados que reservó su organización, cancelación, búsqueda y bandeja) y **administrador** (lo del operador más reconstruir el índice).
- La web habla **solo con el gateway** (`docs/interfaces-internas.md` §11). La identidad la pone el gateway a partir del token (teórico 6); la web nunca manda headers de identidad.
- El grupo ya eligió React + Vite + TypeScript (`CLAUDE.md`). Regla del grupo: no reinventar lo que resuelve una librería.
- Teórico 9: `EventSource` no permite `Authorization`, así que el stream se lee con `fetch`. Un reintento por error de red tiene que ser seguro: misma `Idempotency-Key`.

## Decisión
1. **SPA con React + Vite + TypeScript** en `web/`, sin servidor propio: los archivos estáticos los sirve un NGINX en `compose.yaml` (`http://localhost:5173`), y en desarrollo, `npm run dev`. **No se despliega en la nube** (ADR-026).
2. **Librerías:**

   | Para qué | Librería | Por qué |
   | --- | --- | --- |
   | Componentes (tablas, formularios, modales, notificaciones, layout) | **Mantine** (`@mantine/core`, `@mantine/hooks` y `@mantine/notifications`) | Componentes accesibles y completos con poca configuración; evita escribir CSS y componentes propios |
   | Estado del servidor | `@tanstack/react-query` | Caché, reintentos e **invalidación** cuando llega un aviso por el stream |
   | Rutas | `react-router` (modo librería) | |
   | Tests | `vitest` | |

3. **Un cliente HTTP único** (`src/api.ts`):
   - agrega `Authorization: Bearer`;
   - genera una `Idempotency-Key` **por intención** (un doble clic o un reintento de la misma cancelación reusa la clave);
   - interpreta Problem Details (`detail` y los `errores` por campo);
   - ante un `401` vuelve al ingreso;
   - ante un `502`, `503` o `504`, muestra un mensaje de "no disponible". No hay reintentos automáticos de escrituras.
4. **Sesión:** el token va en `sessionStorage`: se borra al cerrar la pestaña y vence en 1 h. El perfil sale de `GET /api/identidad/yo`.
5. **Tiempo real:** un stream por sesión con `fetch` y un parser SSE propio y testeado (`src/sse.ts`; el formato es texto simple y no hay librería que lo lea con `Authorization` sin agregar otra dependencia).
   - Ante cada aviso: una notificación de Mantine e invalidación de las consultas de React Query, así la pantalla se actualiza sola.
   - Si el stream se corta, reconecta con backoff y `Last-Event-ID`.
6. **La web no ofrece la capacidad publicada** (ADR-021): no reserva franjas. Muestra lo que llegó por ella y lo gestiona.
7. **Sin capa de agregación (BFF):** cada pantalla usa uno o dos endpoints que ya existen. Una capa más no resolvería ningún problema concreto en esta etapa.

## Alternativas consideradas
- **MUI.** Viable, pero es más pesada y su personalización es más verbosa. Mantine resuelve lo mismo con menos código.
- **CSS propio o Tailwind.** Descartadas: cada tabla, formulario y modal se arma a mano (regla de no reinventar).
- **Next.js u otro framework con servidor.** Descartada: no hace falta renderizado en el servidor, y sumaría un proceso más que operar.
- **`EventSource`.** Descartada: no permite el header `Authorization`, y poner el token en la URL lo deja en los logs (teórico 9).
- **Token en `localStorage`.** Descartada: persiste entre sesiones y queda expuesto más tiempo ante un XSS.
- **Cookie `httpOnly`.** Descartada por ahora: exige que el gateway maneje cookies y una protección contra CSRF.
- **Desplegarla en Vercel (ADR-018).** Descartada en ADR-026: la consigna no lo pide y el backend en la nube no tiene Notificaciones ni Solr.

## Consecuencias
- **Ganamos:**
  - una interfaz completa con poco código propio;
  - la pantalla se actualiza sola con los avisos;
  - cancelaciones seguras ante doble clic o reintento.
- **Pagamos:**
  - un parser SSE propio, chico y con tests;
  - el token es accesible desde JavaScript mientras la pestaña está abierta (mitigado por `sessionStorage` y el vencimiento de 1 h);
  - la web solo funciona contra el sistema local.

## Cómo se verifica
- `npm run lint`, `npm run build` y `npm test` en verde en el CI (job `web`).
- **Recorrido manual** con `docker compose up`:
  - el operador ve una reserva aparecer sola (CA-41), la pasa a `EN_CURSO` y a `COMPLETADO`, y ve la alerta climática;
  - el cliente ve solo lo de su organización, recibe los avisos y cancela;
  - el administrador reconstruye el índice.
- Test del parser SSE con eventos partidos entre lecturas, comentarios `: ping` y varios eventos juntos.
