# ADR-030 — Monitor externo que mantiene despierta la capacidad en Render

- **Fecha:** 2026-11-08
- **Estado:** aceptada
- **Reemplaza parcialmente a:** [ADR-026](ADR-026-despliegue-en-render.md), en el punto 4 ("Arranque en frío") y en la alternativa descartada "Mantener los servicios despiertos con un pinger externo". El resto de ADR-026 sigue vigente.

## Contexto
- ADR-026 resolvía el arranque en frío así: el primer pedido despierta al gateway, y el gateway, al arrancar, le pide `GET /salud` a Identidad, Integración y Operaciones para que despierten todos a la vez.
- **Medido el 2026-11-08 contra Render, eso no funciona** (evidencia en los logs de Render del gateway):
  - a las 19:32 UTC, después de 17 minutos sin tráfico, el gateway despertó en 11,7 s con el primer pedido;
  - durante los 120 s de su plazo, el gateway le pidió `/salud` a los otros tres cada 5 s. Render le respondió **`502` al instante** y no los despertó: Integración y Operaciones quedaron dormidos (`"servicio no respondió al despertar"`, 120 000 ms), e Identidad despertó recién cuando se le pegó directo desde internet;
  - mientras tanto, `POST /auth/token` por el gateway devolvía `502`. Un pedido desde internet a la URL de cada servicio, en cambio, los despertó en ~12 s.
- O sea: **en el plan gratuito, un servicio dormido solo despierta con tráfico que llega desde internet, no con el de otro servicio de Render.** Con ADR-026 tal como estaba, después de cada siesta la capacidad quedaba caída hasta que alguien despertara cada servicio a mano. La consigna pide que esté "operativa hasta finalizar la evaluación" y que el consumidor se integre "sin depender de explicaciones privadas".
- ADR-026 había descartado un pinger externo porque 4 servicios × 720 h superan las 750 h gratuitas del mes. Eso vale si se los mantiene despiertos **todo el mes**, pero la ventana que hace falta cubrir es más corta: desde hoy hasta la Presentación (mié 11 o vie 13/11).
- No reinventar: un monitor de disponibilidad es una herramienta que ya existe, gratuita y con alertas.

## Decisión
- **UptimeRobot (plan gratuito)** pide `/salud` a los cuatro servicios desplegados (gateway, Identidad, Integración y Operaciones) **cada 5 minutos**, desde internet. Como Render duerme un servicio a los 15 minutos sin tráfico, ninguno llega a dormirse.
- **Solo hasta el fin de la Presentación** (vie 13/11 a la noche, hora de Argentina). Después se pausan los monitores y vuelve el comportamiento de ADR-026 (se duerme y se despierta con `infra/render/despertar.sh`).
  - Cuenta de horas: del 08/11 a las 19 UTC al 14/11 a las 03 UTC son 128 h × 4 servicios = **512 h**, más las que ya se usaron hoy (~10 h). Quedan unas 230 h de margen sobre las 750 del mes.
  - Si la evaluación sigue después del 13/11 (por ejemplo, para la Defensa Individual), se vuelve a hacer la cuenta: en noviembre no alcanzan las horas para todo el mes, y en diciembre empieza un cupo nuevo.
- **`/salud` responde también `HEAD`** (PR #26): es lo que manda UptimeRobot gratuito, que no deja elegir el método. Sigue siendo una sonda de "vivo", no de "listo" (teórico 6): no mira las bases ni el broker.
- **El monitor también es una alarma:** si un servicio deja de responder, UptimeRobot manda un mail. Es detección externa, desde afuera de Render, que complementa las alertas de Grafana (que solo existen en local).
- **Se mantiene de ADR-026:** el deadline de 5 s del gateway **no se agranda** (teórico 7: el deadline es una decisión de producto); el reintento con la misma `Idempotency-Key` sigue siendo seguro; el despertar del gateway al arrancar queda (no molesta y sirve si algún día los servicios se hablan por otro camino), y `infra/render/despertar.sh` sirve para despertar todo a mano después de la Presentación.

## Alternativas consideradas
- **Dejarlo como en ADR-026 y despertar a mano antes de cada uso.** Descartada: el grupo consumidor no podría integrarse solo, y la capacidad estaría caída casi todo el tiempo.
- **Agrandar el deadline del gateway para esperar a que despierten.** Descartada: no resuelve nada, porque Render no despierta a los servicios por pedidos del gateway; y aunque los despertara, un deadline de un minuto es una mala decisión de producto (teórico 7).
- **Un workflow programado de GitHub Actions como pinger.** Descartada: GitHub no garantiza la hora de las ejecuciones programadas y suele atrasarlas 10 a 15 minutos, justo el margen antes de que Render duerma un servicio.
- **Plan pago de Render (Starter, ~USD 7 por mes por servicio).** Descartada por el grupo: ADR-026 eligió costo cero.
- **Mantenerlos despiertos todo el mes.** Descartada: 4 × 720 h superan las 750 h y Render suspendería todo hasta el mes siguiente (el argumento de ADR-026 sigue valiendo para el mes completo).

## Consecuencias
- **Ganamos:**
  - la capacidad responde en el momento durante la evaluación, sin arranque en frío para el consumidor;
  - una alarma por mail si algo se cae en la nube, y un historial de disponibilidad que sirve de evidencia.
- **Pagamos:**
  - una cuenta más (UptimeRobot) y acordarse de pausar los monitores después de la Presentación;
  - ~520 de las 750 horas del mes. Si se olvida pausarlos, la cuota se agota alrededor del 15/11 y Render suspende los cuatro servicios hasta diciembre;
  - si el monitor falla, vuelve el problema de ADR-026: hay que despertar los servicios con `infra/render/despertar.sh`.

## Cómo se verifica
- Los cuatro monitores de UptimeRobot en "Up".
- Después de más de 15 minutos sin tráfico propio, los logs de Render no muestran arranques nuevos de ningún servicio, y `GET /salud` responde en menos de 1 s.
- En el panel de Render (Billing → Free usage), las horas del mes quedan por debajo de 750 el 13/11.
