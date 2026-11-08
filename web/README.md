# Aplicación web de Rumbo

Interfaz única de los usuarios de Rumbo ([ADR-029](../docs/adr/ADR-029-aplicacion-web.md)): React + Vite + TypeScript, con Mantine (componentes), React Query (datos del servidor) y React Router. Habla **solo con el API Gateway**, en `/api/<servicio>/...` ([`docs/interfaces-internas.md`](../docs/interfaces-internas.md) §11 a §16).

## Cómo correrla

### Con Docker (junto con todo el sistema)
```bash
docker compose up --build      # desde la raíz del repo
```
Queda en **http://localhost:5173**, servida por NGINX como archivos estáticos (cualquier ruta devuelve `index.html` y el ruteo lo hace el navegador).

### En desarrollo (recarga en caliente)
Requiere Node.js 22 y el backend levantado (al menos `docker compose up -d gateway`).
```bash
cd web
npm install
npm run dev                    # http://localhost:5173
```
- **`VITE_API_URL`**: URL del gateway. Por defecto `http://localhost:8080`. Para cambiarla, copiar `.env.example` a `.env.local`. Se fija al compilar: en Docker es el argumento `VITE_API_URL` del `Dockerfile`.
- El gateway habilita CORS para `http://localhost:5173` (`WEB_ORIGEN`, §11.4). Si la web corre en otro origen, hay que cambiar esa variable.

### Calidad
```bash
npm run lint      # oxlint (el linter de la plantilla de Vite)
npm run build     # tsc + vite build
npm test          # vitest: parser SSE y cliente HTTP
```
Los tres corren en el CI (job `web`).

## Usuarios de demostración
Los siembra Identidad al arrancar (§15), con las credenciales de `.env`:

| Usuario | Variables | Pantalla inicial |
| --- | --- | --- |
| Operador ("Operador de Rumbo") | `OPERADOR_EMAIL`, `OPERADOR_PASSWORD` | `/franjas` |
| Cliente de `org-almacenamiento` ("Usuario de almacenamiento") | `CLIENTE_EMAIL`, `CLIENTE_PASSWORD` | `/traslados` |
| Administrador | `ADMIN_EMAIL`, `ADMIN_PASSWORD` | `/franjas` |

## Pantallas por rol
| Pantalla | Operador | Administrador | Cliente |
| --- | --- | --- | --- |
| `/franjas`: franjas de un día con su ocupación; clic → sus traslados | ✓ | ✓ | — |
| `/traslados`: listado con filtros por fecha y estado | Todas las organizaciones | Todas las organizaciones | Solo la suya ("Traslados de …") |
| `/traslados/:id`: detalle; "Marcar en curso" / "Marcar completado" | ✓ | ✓ | — |
| `/traslados/:id`: "Cancelar traslado" (con motivo) | ✓ | ✓ | ✓ |
| `/buscar`: búsqueda de texto con filtros (Solr) | ✓ | ✓ | ✓ (su organización) |
| `/avisos`: bandeja de avisos, "Marcar como leído" | ✓ | ✓ | ✓ |
| `/admin/busqueda`: "Reconstruir índice de búsqueda" | — | ✓ | — |

Un usuario con varios roles ve todas sus secciones. El menú solo decide qué se muestra: la autorización la hace cada servicio con la identidad que pone el gateway, así que un cliente ve solo lo suyo aunque cambie la URL.

**La web no reserva franjas** ([ADR-021](../docs/adr/ADR-021-capacidad-de-autoservicio.md)): la reserva es la API de autoservicio para otros sistemas (`docs/contracts/`). La web muestra lo que llegó por ahí y lo gestiona.

## Cómo está hecha
| Archivo | Qué hace |
| --- | --- |
| `src/api.ts` | Cliente HTTP único: `Authorization: Bearer`, `Idempotency-Key` por intención en la cancelación, Problem Details a mensajes, `401` → ingreso, `502`/`503`/`504` → "Rumbo no está disponible…". No reintenta escrituras |
| `src/sesion.ts`, `src/componentes/ProveedorSesion.tsx` | Token en `sessionStorage` (vence en 1 h) y perfil de `GET /api/identidad/yo` |
| `src/sse.ts` | Parser de Server-Sent Events (eventos partidos entre lecturas, comentarios `: ping`, varios eventos juntos) |
| `src/tiempoReal.ts` | Un único stream de avisos por sesión, leído con `fetch` (no `EventSource`, que no permite `Authorization`). Ante cada aviso: notificación, campana e invalidación de React Query. Reconecta con backoff (1, 2, 4… hasta 30 s) y `Last-Event-ID` |
| `src/paginas/` | Una pantalla por ruta |

Las horas se muestran en `dd/mm/aaaa HH:MM` de Argentina (`America/Argentina/Cordoba`) y los filtros de fecha usan `AAAA-MM-DD`.

## Por qué no se despliega
La web corre solo en local ([ADR-026](../docs/adr/ADR-026-despliegue-en-render.md)). En la nube está solo la capacidad publicada (gateway, Identidad, Operaciones e Integración), y no están Notificaciones ni Solr, que la web necesita para la bandeja, el tiempo real y la búsqueda. La consigna no pide desplegar la web.
