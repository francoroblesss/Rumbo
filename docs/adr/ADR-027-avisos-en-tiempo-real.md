# ADR-027 — Avisos en tiempo real con Server-Sent Events (bonus)

- **Fecha:** 2026-11-06
- **Estado:** aceptada
- **Relacionado:** bonus "Actualizaciones en tiempo real" anunciado en la Entrega 2 (`SPEC.md` §7); [ADR-001](ADR-001-limites-de-los-servicios.md) (Notificaciones es dueño de los avisos); [ADR-024](ADR-024-alcance-de-la-presentacion.md) (eventos y roles de esta etapa).

## Contexto
- **El bonus anunciado:** el operador y el cliente ven en la web los cambios que les importan apenas ocurren, sin recargar. Criterio (CA-41): el aviso aparece en la pantalla abierta en **menos de 2 s**.
- **Lo que ya existe:** Operaciones publica en `rumbo.operaciones` los eventos de las reservas (aceptada, cancelación, cambio de estado, alerta climática), y Notificaciones es el dueño de la bandeja (ADR-001, dominio §13).
- **Destinatarios** (ADR-024): avisos dirigidos a un **rol** (`operador`) o a una **organización**. Ningún aviso es para un usuario puntual, y Notificaciones no necesita conocer la lista de usuarios.
- **Teórico 9:**
  - **SSE** es el flujo servidor → cliente, para "actualizaciones unidireccionales frecuentes";
  - **WebSockets** es bidireccional, y **"no usar WebSockets para actualizar una pantalla si el cliente no responde por ese canal"**;
  - el **polling** sirve con cambios poco frecuentes, pero hace consultas sin novedades;
  - SSE **no es una cola durable**: el estado se persiste aparte y el cliente se reconcilia consultando;
  - `EventSource` **no permite el header `Authorization`**: hay que usar `fetch` leyendo el stream, y no poner tokens en la URL;
  - los proxies tienen que dejar pasar el flujo, sin buffer y sin un timeout corto.
- El gateway corta toda request a los 5 s (ADR-004). El stream tiene que quedar fuera de ese deadline (lo deja así la base: `docs/interfaces-internas.md` §11.4).

## Decisión
1. **Los avisos se persisten primero.** El consumidor de Notificaciones (`notificaciones.avisos`) aplica la tabla de reglas de `docs/interfaces-internas.md` §14 y guarda cada aviso en MongoDB. Es idempotente: el `_id` del aviso deriva del `id` del mensaje. La bandeja (`GET /avisos`) es la fuente de verdad.
2. **El stream (`GET /avisos/stream`, `text/event-stream`) se arma consultando MongoDB**, sin un hub en memoria:
   - cada conexión abierta busca **cada 1 s** los avisos visibles para quien llama (sus roles y su organización) más nuevos que el último enviado, con un índice por destinatario y fecha;
   - los manda como `event: aviso`, con `id: <id del aviso>`;
   - un comentario `: ping` cada 15 s mantiene viva la conexión a través del gateway;
   - con `Last-Event-ID`, primero reenvía lo posterior a ese aviso; si el id no existe, manda los del último minuto;
   - la conexión termina cuando el cliente se va (`ctx.Done()`).
3. **`leido` es compartido por destinatario** (lo marca uno y queda marcado para todos los de ese rol u organización), porque el aviso es para el rol o la organización y no para una persona.
4. **La web** abre el stream con `fetch` y `Authorization: Bearer`, y lee el cuerpo con un lector de streams. Ante cada aviso muestra una notificación y vuelve a pedir los datos de la pantalla (la fuente de verdad sigue siendo la API). Si se corta, reconecta con `Last-Event-ID` y backoff ([ADR-029](ADR-029-aplicacion-web.md)).
5. **El stream no se cuenta en la latencia HTTP:** se registra fuera del middleware de métricas (dura minutos y arruinaría el p95 del servicio).

## Alternativas consideradas
- **WebSockets.** Descartada: el cliente no manda nada por ese canal, y el teórico 9 lo señala como un uso equivocado. Suma handshake, reconexión y autenticación propias sin necesidad.
- **Polling desde la web.** Descartada para el bonus: para cumplir "menos de 2 s", cada pantalla abierta consultaría la API cada segundo aunque no haya novedades. Queda igual como respaldo: la bandeja funciona sin el stream.
- **Hub en memoria** (el consumidor empuja a las conexiones abiertas). Descartada: un reinicio pierde lo que estaba en vuelo, y con más de una réplica un aviso consumido por una no llegaría a los usuarios conectados a la otra. Consultar MongoDB hace que cualquier réplica sirva cualquier stream.
- **Change streams de MongoDB.** Descartada: requieren un replica set, y el Compose usa un nodo único.
- **Avisos por usuario, con lectura individual.** Descartada (ADR-024): Notificaciones tendría que conocer qué usuarios tiene cada rol y cada organización (una llamada a Identidad o una réplica por eventos), sin una necesidad concreta en esta etapa.
- **Token en la URL para usar `EventSource`.** Descartada: queda en los logs de acceso (teórico 9).

## Consecuencias
- **Ganamos:**
  - avisos en la pantalla en ~1 s, sin recargar;
  - stateless: escala con réplicas y sobrevive a reinicios;
  - si el stream falla, la bandeja sigue funcionando.
- **Pagamos:**
  - cada conexión abierta hace una consulta indexada por segundo a MongoDB. Está bien para decenas de usuarios; para muchos más habría que pasar a un hub con un broker de difusión (limitación conocida);
  - conexiones largas a través del gateway, que necesita una excepción a su deadline;
  - el aviso llega con hasta 1 s de demora, más la del evento (relay del outbox y consumo);
  - un operador que lee un aviso lo marca como leído para los demás operadores.

## Cómo se verifica
- **Test del handler SSE** (`httptest`): manda los avisos nuevos con `id` y `event`, respeta `Last-Event-ID` y corta al cancelar el contexto.
- **Test de integración con MongoDB real:** un evento repetido no duplica el aviso, y un cliente no ve los avisos de otra organización.
- **Con el sistema levantado** (CA-41): con la pantalla del operador abierta, una reserva por la API pública hace aparecer "Reserva nueva" en **menos de 2 s**. Se mide con el log `aviso enviado por stream`, que tiene la demora desde `creado_en`, y con el `ocurrido_en` del evento.
