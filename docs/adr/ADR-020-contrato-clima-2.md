# ADR-020 — Contrato de clima 2.0.1 del proveedor (reemplaza parcialmente a ADR-012)

- **Fecha:** 2026-10-23
- **Estado:** aceptada
- **Reemplaza a:**
  - la ruta de la viñeta "Llamada" de [ADR-012](ADR-012-consumo-clima.md): `GET {CLIMA_URL}/v1/airports/{CLIMA_ICAO}/conditions` pasa a `GET {CLIMA_URL}/airports/{CLIMA_ICAO}/conditions`;
  - la copia de la viñeta "Contrato fijado" de ADR-012: `docs/contracts/consumidos/clima-v1.yaml` (1.0.0) pasa a `docs/contracts/consumidos/clima.yaml` (2.0.1).
  - **El resto de ADR-012 sigue vigente:** uso en el negocio, fail-open, timeout, reintento, breaker, caché y métricas.

## Contexto
El grupo del sistema de aeropuerto (`MaxiBarella/Proyecto-arquitectura`), que nos provee el clima (D9), cambió su contrato entre el 18 y el 20/10. Publicó la **2.0.0** (rompe compatibilidad: saca la versión de la ruta) y la **2.0.1** (solo una aclaración). También publicó su URL real en Render, que se duerme sin uso.

| | Lo que teníamos fijado | Lo vigente del proveedor |
| --- | --- | --- |
| Versión | 1.0.0 (`clima-v1.yaml`, commit `d9113be`) | **2.0.1** (`clima.yaml`, commit `70ad2d6`) |
| Ruta | `GET /v1/airports/{icao}/conditions` | **`GET /airports/{icao}/conditions`** (sin versión; ellos lo marcan como cambio que rompe) |
| Versionado | versión mayor en la ruta | versión solo en el documento (`info.version`), igual que nuestro [ADR-019](ADR-019-contrato-sin-version-en-la-ruta.md) |
| Dirección real | no había | `https://clima-aeropuerto.onrender.com`. Plan gratuito: se duerme sin uso y el primer pedido tarda cerca de un minuto |
| `503 WEATHER_SOURCE_UNAVAILABLE` | "no se pudo obtener el clima de la fuente" | "el último dato tiene más de 30 minutos y no se pudo renovar" |
| `precipitation_mm` | última hora | últimos 15 minutos (2.0.1, solo aclaración) |

El cuerpo de la respuesta, los códigos de error, la autenticación (`X-API-Key`) y el límite (60 por minuto) no cambiaron.

- Es exactamente el escenario de la **consigna §4.1**: trabajar con un sistema que no controlamos, a partir de su contrato publicado.
- **Teórico 9 (contratos y evolución compatible):** el número de versión identifica el contrato pero **no vuelve compatible un cambio**. Ante un cambio incompatible hay **migración coordinada** o **versiones que conviven**. El proveedor **no** mantuvo `/v1`, así que nos toca migrar.
- Nuestro adaptador llamaba a `/v1/...`. Con el mock local andaba, pero contra el proveedor real daba `404`, y todo traslado al aeropuerto habría quedado "clima no verificado".
- Lo que detectó el cambio fue el **test de contrato** al actualizar la copia fijada. Con la copia 2.0.1 y el adaptador sin cambiar (`go test ./adaptadores/clima/ -v`):
  ```
  === RUN   TestContratoEjemplosDeRespuesta/operable
      contrato_test.go:104: el pedido no cumple el contrato: no matching operation was found
  --- FAIL: TestContratoEjemplosDeRespuesta (0.03s)
  --- FAIL: TestContratoErrores (5.96s)
  --- FAIL: TestBreakerSeAbreConCincoFallas (0.55s)
  FAIL	github.com/francoroblesss/Rumbo/servicios/operaciones/adaptadores/clima	9.503s
  ```
  `kin-openapi` no encuentra en el contrato nuevo ninguna operación para el pedido `/v1/...` que arma el adaptador. Falla también el test del breaker, porque su servidor de prueba valida cada pedido contra el mismo contrato.

## Decisión
1. **Migrar a la ruta nueva** y fijar la copia 2.0.1 (commit `70ad2d6`).
   - Por la arquitectura hexagonal de Operaciones ([ADR-010](ADR-010-estilos-internos.md), teórico 8), el cambio queda **solo en el adaptador** `adaptadores/clima`.
   - El puerto `ConsultaClima`, el dominio y el caso de uso no cambian.
2. **La URL del proveedor es configuración** (`CLIMA_URL`, por defecto el mock): en local se usa el mock y en la nube ([ADR-018](ADR-018-despliegue-en-la-nube.md)) la URL real.
3. **No se cambian el timeout (1 s por intento) ni el reintento (1)**, aunque el proveedor sugiere un timeout de 60 s para el primer pedido después de dormir.
   - La consulta está dentro de la aceptación del operador, que es síncrona. Esperar 60 s la bloquearía por un dato **informativo**.
   - El deadline es una decisión de producto, y no se copian umbrales de otro (teórico 7, anti-patrón "copiar umbrales").
   - Con el proveedor dormido aplica el **fail-open de ADR-012**: la aceptación sigue y queda "clima no verificado".
   - Esos pedidos fallidos igual despiertan el servicio, que en el minuto siguiente vuelve a responder.
   - Si hay 5 fallas seguidas se abre el breaker 30 s, como ya está decidido.
4. **El `503` nuevo** (dato de más de 30 min) se sigue tratando como "no disponible", así que el traslado queda "clima no verificado": un dato tan viejo no sirve para decidir.
   - No se reintenta salvo `Retry-After` ≤ 1 s, como ya decía ADR-012.
   - El dato puede tener hasta 30 min del proveedor más 5 min de nuestra caché (máximo 35 min). Se acepta porque la alerta es para el operador, que decide.
5. **`precipitation_mm` no se usa** (solo leemos `observed_at` y `operability`): la aclaración 2.0.1 no nos afecta. Seguimos ignorando los campos desconocidos (lector tolerante), como pide la política de compatibilidad del proveedor.
6. **Antes de una demo, despertar el servicio** con el procedimiento de [`docs/contracts/consumidos/README.md`](../contracts/consumidos/README.md).

## Alternativas consideradas
- **Timeout de 60 s, como sugiere el proveedor.** Descartada: bloquea al operador por un dato informativo (decisión 3).
- **Hacer pedidos periódicos para que no se duerma (keep-alive).** Descartada: gasta el límite de 60 pedidos por minuto y el plan gratuito del proveedor, y agrega un componente que no resuelve un problema de nuestro negocio (complejidad mínima).
- **Consultar el clima en segundo plano, después de aceptar.** Descartada: cambia el flujo de R-41 y suma complejidad. No hace falta mientras el fail-open alcance.
- **Mantener `/v1`.** Descartada: ya no existe del lado del proveedor.

## Consecuencias
- **Ganamos:**
  - volvemos a cumplir el contrato vigente;
  - el cambio quedó contenido en un adaptador;
  - queda probado que el test de contrato detecta un cambio incompatible del proveedor.
- **Pagamos:**
  - el primer pedido después de que el proveedor se duerme termina en "clima no verificado";
  - el dato puede tener hasta 35 min;
  - seguimos atados a que el proveedor avise sus cambios en su historial.

## Cómo se verifica
- **Test de contrato:** en rojo con la copia nueva y el adaptador viejo (fragmento en Contexto). En verde después de corregir la ruta del adaptador: `TestContratoEjemplosDeRespuesta`, `TestContratoErrores`, `TestValidadorDetectaPedidosInvalidos` y `TestBreakerSeAbreConCincoFallas` pasan, y `go test ./...` de Operaciones también.
- **Mock (`clima-mock`), 2026-10-23, con `docker compose up --build -d`:** desde el contenedor de Operaciones, `GET /airports/SACO/conditions` devolvió `200` con el ejemplo `operable`, y `GET /v1/airports/SACO/conditions` devolvió `404` (Prism: `NO_PATH_MATCHED_ERROR`).
- **Aceptación al aeropuerto** (`aeropuerto_extremo: DESTINO`, hora pedida dentro de 2 h), el mismo día:
  - con el mock arriba: `200`, solicitud `ACEPTADA` y traslado sin alerta (aeropuerto operable). El mock registró el pedido del adaptador en `/airports/SACO/conditions`. En los logs de Operaciones no hubo ningún `el servicio de clima rechazó el pedido`;
  - con `clima-mock` detenido (y la caché vacía, reiniciando Operaciones): `200` en 2,7 s, solicitud `ACEPTADA` y `alerta_climatica` `NO_VERIFICADO`, con el log `clima no verificado al aceptar` (`context deadline exceeded` en `/airports/SACO/conditions`). Fail-open sin cambios.
  - Mientras dura la caché de 5 min, una aceptación con el mock recién detenido usa el último dato y no consulta al proveedor: es lo que decide ADR-012.
- **Contra el proveedor real:** pendiente, falta la clave. Cuando el grupo la tenga, se mide el arranque en frío y el p50/p99 en caliente, y se repite la medición de [ADR-016](ADR-016-validacion-d5.md).

## Fuentes
- Consigna §4.1 (consumir una capacidad de otro grupo desde su contrato publicado, con test de contrato).
- Teórico 9, "Contratos y evolución compatible": la versión no vuelve compatible un cambio; migración coordinada o versiones que conviven.
- Teórico 7: timeouts y deadlines (el deadline es una pregunta de producto); fail-open vs. fail-closed; anti-patrones ("copiar umbrales").
- Teórico 8: arquitectura hexagonal, puertos y adaptadores.
- [ADR-010](ADR-010-estilos-internos.md), [ADR-012](ADR-012-consumo-clima.md), [ADR-018](ADR-018-despliegue-en-la-nube.md), [ADR-019](ADR-019-contrato-sin-version-en-la-ruta.md).
- Guía del proveedor en el commit `70ad2d6`: [README de su contrato](https://github.com/MaxiBarella/Proyecto-arquitectura/blob/70ad2d6896da850a3a5789e97c139c3dca7f960a/docs/contracts/README.md).
