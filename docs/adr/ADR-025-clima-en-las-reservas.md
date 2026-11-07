# ADR-025 — Clima del aeropuerto en las reservas (D9)

- **Fecha:** 2026-11-06
- **Estado:** aceptada
- **Reemplaza parcialmente a:** [ADR-012](ADR-012-consumo-clima.md) y [ADR-020](ADR-020-contrato-clima-2.md) en **cuándo** se consulta el clima y **cómo** se sabe que un traslado va al aeropuerto. Se mantiene todo lo demás de ADR-012 y ADR-020: contrato 2.0.1 del proveedor, timeout de 1 s, un reintento solo ante errores transitorios, circuit breaker, caché de 5 min, fail-open y test de contrato.

## Contexto
- La consigna (§4.1) pide que la capacidad del proveedor esté en **un flujo importante** del sistema consumidor. Una llamada aislada no vale.
- ADR-012 consultaba el clima **cuando el operador aceptaba una solicitud**, y era el operador quien marcaba qué extremo era el aeropuerto. Desde [ADR-021](ADR-021-capacidad-de-autoservicio.md) las reservas entran confirmadas, y [ADR-024](ADR-024-alcance-de-la-presentacion.md) saca el flujo de aceptación. Así, **el clima ya no se consulta nunca**.
- El flujo principal de la Presentación es la reserva de franjas (ADR-024). Tiene dos momentos donde el clima importa:
  - **después de reservar**, si el traslado es para dentro de pocas horas;
  - **cuando sale** (`EN_CURSO`): es el "al iniciar" de R-41 en el dominio.
- La reserva es síncrona, con un timeout de 2 s desde Integración (ADR-021). El clima tiene un timeout de 1 s más un reintento, y el servicio del proveedor se duerme (ADR-020).
- Teórico 4: lo que el negocio no necesita saber para responder (avisos, verificaciones) va asíncrono, separando la aceptación del procesamiento. Teórico 7: fail-open o fail-closed es una decisión explícita, y una degradación tiene que ser honesta.

## Decisión
1. **El aeropuerto se detecta solo:** al reservar, Operaciones marca `aeropuerto_extremo` si el nombre o la dirección del origen (o, si no, del destino), normalizados, contienen alguna palabra de `AEROPUERTO_PALABRAS` (por defecto `aeropuerto`, `saco` y `pajas blancas`). Es una regla pura del dominio, sin llamadas externas, dentro de la misma transacción de la reserva.
2. **Después de reservar, asíncrono:** un consumidor de Operaciones (`operaciones.clima`) escucha su propio evento `operaciones.solicitud.aceptada`. Si el traslado va al aeropuerto y su hora comprometida cae dentro de las próximas 6 h:
   - consulta el clima **fuera** de la transacción;
   - registra la alerta en una transacción corta, solo si cambió;
   - publica `operaciones.traslado.alerta_climatica`.

   La reserva no espera al clima.
3. **Al pasar a `EN_CURSO`, síncrono:** el operador avanza el traslado; si va al aeropuerto y está dentro de las 6 h, se consulta el clima antes de la transacción y la alerta se guarda con el cambio de estado.
4. **Fail-open en los dos momentos**, como en ADR-012: si el clima no responde, la alerta queda `NO_VERIFICADO` y la operación sigue. En el consumidor, un clima caído no es un error del mensaje: no se reintenta ni va a la DLQ, porque el adaptador ya reintentó. Un error de MySQL sí se reintenta.
5. **Idempotencia:** si el traslado ya tiene la misma alerta (mismo estado y mismos motivos), no se escribe ni se publica. Un evento reentregado deja todo igual.
6. **Métrica:** `rumbo_clima_consultas_total` suma el atributo `momento` (`reserva` o `inicio`).

Detalle del contrato en `docs/interfaces-internas.md` §13.

## Alternativas consideradas
- **Consultar el clima dentro de la reserva (síncrono).** Descartada: suma hasta ~2 s a una operación con 2 s de timeout desde Integración, y un proveedor dormido haría fallar reservas que no dependen del clima.
- **Solo al pasar a `EN_CURSO`.** Descartada: una reserva para dentro de 3 h con el aeropuerto cerrado se enteraría recién al salir. Avisar enseguida le da al operador y a la organización tiempo para reaccionar.
- **Solo después de reservar.** Descartada: la reserva se hace con 2 h a 30 días de anticipación, y la ventana del clima es de 6 h. Una reserva hecha con días de anticipación nunca se revisaría.
- **Que el sistema externo indique el aeropuerto con un campo nuevo del contrato.** Descartada: es un cambio de contrato para el consumidor (versión nueva) y pondría en el otro grupo una regla que es nuestra.
- **Que el operador marque el aeropuerto desde la web.** Descartada: vuelve a poner un paso manual en una reserva de autoservicio y llegaría tarde para el aviso inmediato.
- **Un consumidor en otro servicio.** Descartada: la alerta es un dato del traslado, que es de Operaciones (ADR-001). Consumir su propio evento separa el trabajo lento de la respuesta sin mover la propiedad de los datos.

## Consecuencias
- **Ganamos:**
  - la capacidad del proveedor entra en el flujo principal en los dos momentos donde importa, sin tocar la latencia de la reserva;
  - el operador y la organización reciben el aviso (Notificaciones consume `alerta_climatica`).
- **Pagamos:**
  - la alerta de una reserva llega unos segundos después del `201` (consistencia eventual);
  - la detección por palabras puede fallar: un nombre como "Hotel cerca del aeropuerto" se marca. Se acepta, porque el resultado es una alerta para que el operador mire, no un bloqueo;
  - una cola y un consumidor más en Operaciones.

## Cómo se verifica
- **Test de dominio** de la detección: "Aeropuerto Internacional de Córdoba", "Av. La Voz del Interior 8500 (SACO)", "AEROPUERTO" con tildes o mayúsculas, y un destino común sin alerta.
- **Test de aplicación** del consumidor, con un clima falso no operable, caído y operable; con un evento reentregado; y con un traslado fuera de la ventana.
- **Test de aplicación** del paso a `EN_CURSO` con el clima caído: el estado cambia y queda `NO_VERIFICADO` (CA-36).
- **Con el sistema levantado** y el mock de clima respondiendo "no operable" (CA-35): reservar una franja que termine dentro de las 6 h, con destino "Aeropuerto Internacional de Córdoba". El `201` llega en el tiempo de siempre y segundos después la vista tiene `alerta_climatica`.
