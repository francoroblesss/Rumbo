# ADR-026 — Despliegue de la capacidad publicada en Render (plan gratuito)

- **Fecha:** 2026-11-06
- **Estado:** aceptada — el arranque en frío (punto 4) queda **reemplazado parcialmente por [ADR-030](ADR-030-monitor-externo-en-render.md)** (monitor externo hasta la Presentación).
- **Reemplaza parcialmente a:** [ADR-018](ADR-018-despliegue-en-la-nube.md). Cambian la plataforma del backend (Railway → Render) y qué se despliega (solo la capacidad, sin la web en Vercel). Se mantienen: una sola URL pública como punto de entrada a la capacidad, secretos como variables de la plataforma y nada de AWS.

## Contexto
- La consigna (§6 y §4.1) pide **la capacidad propia desplegada en la nube y accesible mediante una URL pública**, operativa hasta el final de la evaluación. **No** pide desplegar el sistema entero ni la web: "frontend completo" no dice "desplegado".
- ADR-018 eligió Railway: ≈ USD 24 por mes con el uso medido (ADR-015), mientras que el plan Hobby trae USD 5. El grupo decidió no pagar y usar el **plan gratuito de Render**.
- Condiciones del plan gratuito de Render, verificadas el 2026-11-06 ([render.com/docs/free](https://render.com/docs/free), [render.com/pricing](https://render.com/pricing)):
  - solo hay **web services** gratuitos (512 MB, 0,1 CPU). Los *private services* y los *background workers* son pagos (desde USD 7 por mes cada uno);
  - un servicio gratuito **se duerme a los 15 minutos sin tráfico entrante** y tarda ~1 minuto en despertar;
  - un servicio gratuito **no recibe tráfico de la red privada**: los servicios se hablan por sus URL públicas `onrender.com`;
  - disco efímero: lo que se escribe se pierde al reiniciar;
  - **750 horas gratuitas por mes y por workspace**, que se consumen mientras un servicio está despierto. Si se agotan, Render suspende **todos** los servicios gratuitos hasta el mes siguiente;
  - no hay MySQL ni MongoDB gratuitos (solo PostgreSQL, que vence a los 30 días).
- El camino de la capacidad pasa por cuatro servicios, y cada uno cumple un requisito: el **gateway** (autenticación y límite de tráfico), **Identidad** (emite el token del sistema externo), **Integración** (adapta el contrato y guarda la copia con la idempotencia) y **Operaciones** (la reserva con bloqueo, ADR-022).
- Teórico 6: la identidad se propaga "por un canal confiable diseñado" y los servicios no tienen que ser accesibles sin pasar por el gateway. En Render cada servicio tiene URL pública, así que ese canal hay que construirlo.

## Decisión
**1. Qué va a Render:** cuatro web services gratuitos en **un solo workspace**, construidos desde el `Dockerfile` de cada uno: `gateway`, `identidad`, `integracion` y `operaciones`. **Una sola instancia** de cada uno.
- La URL pública que se publica en el contrato es la del **gateway**.
- Cada servicio escucha en el puerto que le da Render (`PUERTO` = `PORT`).

**2. Bases y broker gestionados gratuitos**, conectados con TLS:

| Qué | Servicio | Base |
| --- | --- | --- |
| MongoDB de Identidad e Integración | MongoDB Atlas M0 (un cluster, dos bases) | `identidad`, `integracion` |
| MySQL de Operaciones | Aiven for MySQL, plan gratuito (1 GB) | `operaciones` |
| RabbitMQ | CloudAMQP, plan Little Lemur (1 M de mensajes por mes, 20 conexiones) | — |

- Atlas acepta conexiones desde cualquier IP (`0.0.0.0/0`), porque Render gratuito no tiene IP fija; la seguridad la dan el usuario, la contraseña y TLS.
- Con Aiven se usa `tls=skip-verify`: el canal va cifrado, pero sin validar la autoridad certificante del proyecto de Aiven (limitación aceptada para un entorno de evaluación).

**3. Secreto interno entre el gateway y los servicios:** header `X-Rumbo-Interno` con `SECRETO_INTERNO` (`docs/interfaces-internas.md` §11.2).
- El gateway lo agrega; los servicios rechazan con `401` lo que no lo trae.
- Las llamadas entre servicios de Rumbo lo mandan (`observabilidad.ClienteInterno`); las llamadas a otro grupo, no.
- Se usa igual en local, para que lo que se prueba con Compose sea lo que corre en la nube.

**4. Arranque en frío:**
- Cada servicio expone `GET /salud` (vivo, no listo: teórico 6).
- Al despertar, el gateway le hace `GET /salud` a cada servicio en segundo plano, para que despierten todos a la vez y no uno por pedido.
- **El deadline de 5 s del gateway no se agranda** para tapar el arranque (teórico 7: el deadline es una decisión de producto). El primer pedido después de un rato de inactividad puede recibir `502` o `504`. Como el `POST` es idempotente con `Idempotency-Key`, reintentar con la misma clave es seguro (teórico 9: un timeout es un resultado incierto que se resuelve reintentando con la misma clave).
- Está documentado en `docs/contracts/README.md`.
- `infra/render/despertar.sh` despierta todo antes de una demo.

**5. Qué no va a la nube** (se demuestra en local con `docker compose up`):

| Pieza | Por qué |
| --- | --- |
| Aplicación web, Notificaciones y Solr | No son parte de la capacidad. Solr es Java y con 0,1 CPU arranca muy lento |
| Memcached | No es HTTP: no corre como web service. Sin caché, Integración lee de MongoDB, que es la degradación diseñada en ADR-007 |
| NGINX y la segunda réplica de Integración | Serían dos servicios más (más horas, el mismo límite de 750). El balanceo se demuestra en local con la evidencia de ADR-009 y ADR-017 |
| Grafana LGTM y el MySQL de logs | En la nube, los logs van a stdout y se ven en los logs de Render |

**6. Las pruebas de carga se corren solo en local:** contra la nube gastarían las horas del workspace y la cuota de CloudAMQP.

## Alternativas consideradas
- **Railway (ADR-018).** Descartada por el grupo: ≈ USD 24 por mes que alguien tiene que pagar.
- **Render con private services y workers pagos.** Descartada: unos 7 servicios a ~USD 7 da ≈ USD 50 por mes, más caro que Railway.
- **Una máquina virtual con crédito estudiantil** (DigitalOcean del GitHub Student Pack o Azure for Students) corriendo el mismo `compose.yaml`. Era la opción más fiel a local, pero el grupo prefirió Render.
- **Repartir los servicios en varios workspaces** (uno por integrante, 750 h cada uno). Descartada por el grupo: cuatro cuentas para administrar.
- **Mantener los servicios despiertos con un pinger externo.** Descartada: con cuatro servicios en un workspace, 4 × 720 h superan las 750 h del mes.
- **Sin secreto interno, confiando en que nadie conoce las URL de los servicios.** Descartada: cualquiera podría mandar `X-Rumbo-Roles: operador` directo a Operaciones (teórico 6).
- **Validar el token en cada servicio.** Descartada: duplica en cuatro servicios lo que hace el gateway (ADR-001, autenticación centralizada). El secreto es más simple y resuelve el problema concreto: que solo el gateway llegue a los servicios.

## Consecuencias
- **Ganamos:**
  - costo USD 0;
  - la capacidad accesible por una URL pública, con la misma imagen que en local.
- **Pagamos:**
  - **arranque en frío:** ~1 minuto, y el primer pedido después de la inactividad puede fallar (reintentable);
  - **riesgo de cuota:** si las 750 h se agotan, la capacidad se cae hasta el mes siguiente. Se mitiga con el sueño de los servicios y sin pruebas de carga en la nube. Se controla el consumo en el panel de Render;
  - mientras un servicio duerme no consume eventos ni publica su outbox: los mensajes esperan en CloudAMQP (cola durable) y se procesan al despertar. La copia de Integración puede atrasarse hasta entonces (consistencia eventual, ya aceptada en ADR-021);
  - la nube no tiene caché, balanceo, observabilidad completa ni búsqueda; todo eso se demuestra en local;
  - los servicios tienen URL pública, protegida por el secreto interno.

## Cómo se verifica
- `POST /traslados` contra la URL pública del gateway, con un token del sistema externo, devuelve `201`, y `GET /traslados/{id}` lo muestra.
- Un pedido directo a la URL de Operaciones con `X-Rumbo-Roles: operador` y sin el secreto devuelve `401`.
- Después de más de 15 minutos sin tráfico, `infra/render/despertar.sh` deja los cuatro servicios respondiendo `/salud`.
- La URL pública del gateway figura en `README.md` ("Parte desplegada") y en `docs/contracts/README.md`, y el grupo consumidor la usa.
