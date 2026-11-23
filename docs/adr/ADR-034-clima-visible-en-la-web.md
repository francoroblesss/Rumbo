# ADR-034 — Condiciones del clima del aeropuerto visibles en la web

- **Fecha:** 2026-11-20
- **Estado:** aceptada
- **Relación:** complementa a [ADR-012](ADR-012-consumo-clima.md) y [ADR-025](ADR-025-clima-en-las-reservas.md) (no los reemplaza: la regla de clima de las reservas no cambia) y a [ADR-029](ADR-029-aplicacion-web.md).

## Contexto
- En la devolución de la Presentación Grupal, el profesor pidió **mostrar de forma gráfica las condiciones climáticas** en la web: que se vea qué tiempo hace, no solo la alerta.
- Hasta ahora el clima solo se veía como una **alerta** del traslado (`NO_OPERABLE` o `NO_VERIFICADO`, R-46). Operaciones ya consulta el contrato de clima del otro grupo (2.1.0, ADR-032), que devuelve las condiciones observadas: temperatura, viento, ráfagas, dirección, visibilidad, precipitación y tormenta. El adaptador las descartaba porque la regla solo usa la operabilidad.
- El proveedor tiene un límite de 60 pedidos por minuto y un cupo diario en su fuente (ARCHITECTURE §10). Una pantalla que consulte el clima no puede multiplicar los pedidos.
- El contrato **no informa nubosidad**: no se puede saber si está soleado o nublado.
- Rumbo **no tiene precio por traslado** (`dominio.md`: solo tarifas de preliquidación de choferes).

## Decisión
- **`GET /clima` en Operaciones** (`/api/operaciones/clima` por el gateway; operador, administrador y cliente) devuelve las condiciones actuales del aeropuerto y si es operable (`docs/interfaces-internas.md` §13.5).
- Usa **el mismo adaptador** de ADR-012 (timeout de 1 s, un reintento con jitter, breaker) y **la misma caché de 5 min** que las reservas: la web suma a lo sumo un pedido al proveedor cada 5 min por aeropuerto (unos 288 por día si alguien deja la página abierta todo el día), no uno por pestaña. La web no reintenta un `503`: volver a pedir enseguida solo le sumaría pedidos a un proveedor que no responde.
- **Fallback honesto:** si el clima no responde o el breaker está abierto, `503 SERVICIO_NO_DISPONIBLE` con `Retry-After: 30` (lo que tarda el breaker en dejar probar de nuevo, igual que Integración), y la web dice "Clima no disponible". No se inventa un clima ni se muestra uno viejo como actual (teórico 7). La falla queda en la tarjeta: el resto de la página sigue.
- **La web** muestra una tarjeta en la página de traslados: ícono según el tiempo (tormenta, viento, baja visibilidad, lluvia o "sin fenómenos adversos"), temperatura, viento con flecha de dirección, barras de viento y ráfagas contra los límites operativos del proveedor (50 y 65 km/h), visibilidad, precipitación y operabilidad con sus motivos. Los íconos salen de `@tabler/icons-react` (no se dibujan a mano).
- El ícono de viento y de visibilidad se elige con los **motivos del proveedor**, no con umbrales propios: la tarjeta nunca contradice si el aeropuerto es operable.
- **No se agrega** una regla de precio por clima ni otra fuente de clima para la nubosidad.

## Alternativas consideradas
- **Que la web llame directo al proveedor.** Descartada: expone la API key del otro grupo en el navegador, salta la caché y el breaker y multiplica los pedidos por cada pestaña abierta.
- **Guardar las condiciones con cada traslado y mostrarlas en su detalle.** Descartada por complejidad mínima: el profesor pidió ver el clima; las condiciones de cada traslado cambiarían el modelo y los eventos sin un uso que lo pida. La alerta del traslado ya registra lo que importa para el negocio (motivos y hora de observación).
- **Mostrar "soleado/nublado".** Descartada: sin el dato de nubosidad sería inventarlo. Se muestra "sin fenómenos adversos".
- **Que el clima suba el precio del traslado.** Descartada: no hay precio por traslado en el dominio y sería una regla de negocio nueva sin un problema concreto que la pida.

## Consecuencias
- **Ganamos:** el clima se ve de forma gráfica para los tres roles, sin pedidos extra al proveedor y con la misma resiliencia que ya tenía la consulta.
- **Pagamos:** un endpoint y un componente más; la tarjeta puede mostrar datos de hasta unos 10 min de antigüedad (5 de la caché del backend más 5 hasta el próximo pedido de la web; se indica la hora de observación); y unos 288 pedidos por día más al proveedor si la página queda abierta. Las condiciones son solo para mostrar: si no llegan o no se pueden leer, la tarjeta dice "Clima no disponible" pero la operabilidad de las reservas no se ve afectada.

## Cómo se verifica
- Tests: `aplicacion` (mapeo, `observado_en` con `-03:00`, `motivos` = `[]`, no operable y error), `adaptadores/api` (`200` con el cuerpo de §13.5, `503` con `Retry-After: 30`, `403 SIN_PERMISO` para `sistema_externo`), adaptador de clima (condiciones y nombre del aeropuerto del ejemplo `operable` del contrato) y `formato.test.ts` (ícono según las condiciones y su prioridad): todos pasan (2026-11-20).
- E2E `TestClimaDelAeropuerto` (operador → `200` con SACO, operable y 22,4 °C; sistema externo → `403 SIN_PERMISO`): pasa local con `docker compose` (2026-11-20) y en el CI del PR.
- Manual con capturas en `docs/evidencias/clima-en-la-web/`: operable, no operable (`CLIMA_PREFER=example=not_operable`) y clima caído (`503` con `Retry-After: 60`, la tarjeta dice "Clima no disponible" y el resto de la página sigue). El reintento queda en el log (`WARN "reintento al clima"`) y en `rumbo_reintentos_total{dependencia="clima"}` (2026-11-20).
- Revisión 2026-11-23: las condiciones y el aeropuerto se leen aparte de la operabilidad (una condición ilegible no deja la reserva en `NO_VERIFICADO`), la web no reintenta el `503`, `Retry-After` pasa a 30 s (el breaker), la barra se pone roja al pasar el límite (no al llegar), el reintento no se cuenta si el breaker quedó abierto y se corrigen la antigüedad máxima (unos 10 min) y los pedidos que suma la web. Las capturas de `docs/evidencias/clima-en-la-web/` son del 2026-11-20, con `Retry-After: 60`.
