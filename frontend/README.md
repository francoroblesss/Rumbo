# Rumbo — Aplicación web

Interfaz web de Rumbo (React + Vite + TypeScript). Habla solo con el **API Gateway**, nunca directamente con los microservicios (ADR-001).

```bash
npm install
npm run dev      # http://localhost:5173 (con el sistema levantado: docker compose up --build)
npm run build    # compila a dist/
npm run lint
```

En desarrollo, Vite reenvía `/api/...` al gateway (`http://localhost:8080`), así que no hace falta CORS. Si el gateway corre en otro puerto: `GATEWAY_URL=http://localhost:18080 npm run dev`. Para un build que llame a un gateway en otro dominio (por ejemplo en Railway, ADR-018): `VITE_API_URL=https://<gateway> npm run build`.

Para ingresar se usa el administrador inicial (`ADMIN_EMAIL` y `ADMIN_PASSWORD` de `.env`).

## Pantallas (Entrega 2)
**Consola del operador**, sobre la API del operador (`/api/v1/operaciones/solicitudes`, `docs/interfaces-internas.md` §4.2):
- **Ingreso** con email y contraseña (`/api/v1/identidad/login`). El token se guarda en `sessionStorage` (dura lo que la pestaña); ante un `401` se vuelve al ingreso.
- **Lista de solicitudes** paginada (20 por página), con filtro por estado. Se actualiza cada 15 s por *polling*, que alcanza para cambios poco frecuentes (teórico 9), y se pausa con la pestaña oculta para no gastar el límite del gateway (60 pedidos por minuto).
- **Detalle**: qué, origen, destino, hora pedida, contrapropuesta, traslado y **alerta climática** (ADR-012).
- **Acciones** según el estado: aceptar con la hora pedida (indicando si un extremo es el aeropuerto, para la regla de clima), contraproponer horario, rechazar con motivo y resolver una cancelación tardía.
  - Cada acción manda una `Idempotency-Key` por operación lógica (acción + cuerpo): si el pedido falla y se reintenta lo mismo, viaja la misma clave y no se duplica el efecto (teórico 9).
  - Los errores se muestran desde el Problem Details (`detail` y errores por campo).

Pendiente para la Presentación: pantallas del cliente y del chofer, búsqueda (ADR-011) y actualizaciones en tiempo real (bonus, por SSE según el teórico 9).
