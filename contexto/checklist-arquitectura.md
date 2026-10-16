# Checklist de decisiones (lo que la cátedra espera poder justificar)

Usar al diseñar cada parte del proyecto y registrar las respuestas en los ADR del proyecto (la consigna pide `docs/adr/ADR-XXX.md`, uno por decisión; ver `proyecto/consigna.md` §5).

## Qué sección alimenta cada ADR obligatorio de la consigna
| ADR | Decisión | Secciones de este checklist |
| --- | --- | --- |
| _Estado al 4/10/2026: D1 → ADR-001, D8 → ADR-002, D3 → ADR-003, D5 → ADR-004 (iniciales). El resto, pendiente._ | | |
| D1 | Límites de los servicios | Descomposición |
| D2 | Arquitectura interna de los servicios | Estilos internos |
| D3 | Persistencia | Datos |
| D4 | Consistencia y concurrencia | Datos, Resiliencia (idempotencia) |
| D5 | Comunicación entre servicios | Mensajería, Resiliencia (timeouts, retries) |
| D6 | Búsqueda | Búsqueda |
| D7 | Caché | Caché |
| D8 | Contrato propio | Borde (versionado de API, teórico 6) |
| D9 | Consumo del proveedor | Resiliencia (timeouts, fallback honesto) |
| D10 | Resiliencia | Resiliencia |
| D11 | Observabilidad | _(sin teórico todavía: anunciado "Producción y despliegue" / Monitoreo)_ |
| D12 | Balanceo de carga | Borde |
| D13 | Capacidad y costos | _(sin teórico todavía)_ |

## Descomposición (teórico 1)
- [ ] Cada servicio = una **capacidad de negocio** con responsabilidad nombrable en una oración.
- [ ] Ningún servicio accede a la DB de otro; se comunican por API o eventos.
- [ ] ¿Hay algún par de servicios que siempre haya que desplegar juntos? → monolito distribuido, revisar límites.
- [ ] Lo que debe ser atómico vive en un mismo servicio.
- [ ] Cada servicio arranca solo, en su puerto, y el sistema funciona con varios puertos a la vez.

## Datos (teórico 2)
- [ ] Dueño único de cada dato; referencias entre servicios por ID lógico.
- [ ] Elección de motor justificada por estructura, queries, acceso, escala, consistencia y experiencia del equipo (no "porque escala").
- [ ] MongoDB: embedding vs reference decidido por patrón de acceso; índices definidos; esquema documentado.
- [ ] Operaciones multi-servicio: ¿Saga (coreografía/orquestación)? ¿compensaciones definidas?
- [ ] Guardar + publicar evento: ¿dual write aceptado conscientemente o Outbox?

## Caché (teórico 3)
- [ ] Las 7 preguntas respondidas (operación costosa, frecuencia, frescura, invalidación, patrón, fallos, métricas).
- [ ] Clave incluye todo lo que cambia la respuesta; TTL definido; invalidación en escrituras.
- [ ] Nada crítico (stock/pago/permisos) se decide con la caché.
- [ ] Caída de caché → degradación, no 500.

## Mensajería (teórico 4)
- [ ] ¿Por qué este efecto puede ser asíncrono?
- [ ] Evento vs comando; contrato versionado con `id`, `tipo`, `version`, `correlation_id`.
- [ ] Cola de trabajo vs fanout/topic justificado.
- [ ] ACK después del efecto; consumidor idempotente (cómo).
- [ ] Qué pasa con mensajes que fallan (reintentos con backoff, DLQ con dueño).

## Búsqueda (teórico 5)
- [ ] ¿Por qué hace falta un motor y no alcanza con la DB?
- [ ] Documento indexado diseñado para la lectura (campos texto / filtro / ranking).
- [ ] Sincronización fuente → índice (eventos, upsert idempotente, versión) y reindexación posible.
- [ ] El servicio dueño valida antes de cualquier efecto de negocio.

## Borde (teórico 6)
- [ ] ¿Gateway, solo LB, o nada? Justificado.
- [ ] Autenticación en el borde, autorización en cada servicio.
- [ ] Servicios stateless; réplicas detrás de NGINX con health checks.
- [ ] Rate limiting (si aplica): dimensión, algoritmo, respuesta 429.

## Estilos internos (teórico 8)
- [ ] La consigna exige **al menos dos servicios con patrones de arquitectura diferentes**, aplicados de forma reconocible: ¿cuáles y por qué cada uno se ajusta a SU problema?
- [ ] Para cada estilo, ¿qué pregunta responde? DDD (reglas/lenguaje/límites) · Hexagonal/Clean (dependencias hacia el núcleo) · CQRS (lectura ≠ escritura) · Event Sourcing (historial como fuente de verdad) · Reactivo (mucho I/O concurrente). Capas/MVC sigue siendo válido si alcanza.
- [ ] Hexagonal/Clean: el núcleo no importa tipos de Gin/ORM/SDK; structs de dominio sin tags `json`/`bson`; puertos (interfaces) solo donde hay variación real.
- [ ] Agregados: un agregado por transacción; invariantes validadas en la raíz; coordinación entre agregados por eventos + consistencia eventual.
- [ ] CQRS: ¿lectura y escritura difieren de verdad (forma, carga, permisos)? ¿misma base o separada? Si hay proyecciones async, consistencia eventual explicitada en el contrato/UI.
- [ ] Explicitar qué NO se optimiza y qué costo se acepta (contratos, mapeos, duplicación).

## Comunicación entre servicios (teórico 9)
- [ ] Para cada interacción: ¿quién inicia, hace falta respuesta inmediata, en qué dirección van los datos? → petición/respuesta (REST, gRPC, GraphQL), canal (SSE, WebSockets) o webhook.
- [ ] Contrato explícito (OpenAPI) con errores y semántica; política de evolución: qué es compatible (campos opcionales que se ignoran) y qué no (quitar, renombrar, **agregar valores a un enum**); versiones que conviven y cuándo se retira la vieja.
- [ ] POST con clave de idempotencia; ante timeout el resultado es **incierto**: retry con la misma clave o reconciliar consultando.
- [ ] Colecciones con paginación y filtros en el contrato.
- [ ] Tiempo real: SSE si el cliente solo recibe (persistir primero, reconsultar al reconectar); WebSockets solo si es bidireccional. Autenticación sin `Authorization` nativo y sin tokens largos en la URL.
- [ ] Webhooks recibidos: validar firma y frescura, deduplicar, decidir con el estado vigente.
- [ ] Proveedor externo detrás de puerto + adaptador, sin prometer sustitución transparente; mandar solo los datos necesarios.

## Resiliencia (teórico 7)
- [ ] Toda llamada remota con timeout/deadline (`context`).
- [ ] Retries solo en operaciones seguras, con backoff+jitter y un único dueño.
- [ ] Mutaciones repetibles con idempotency key.
- [ ] Fallbacks honestos; estados de negocio incluyendo "desconocido/verificando".
- [ ] Las 4 preguntas antes de cada patrón (modo de falla, qué protege, nueva falla, cómo se verifica).
