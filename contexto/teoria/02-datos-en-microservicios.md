# Teórico 2 — Datos en microservicios

Fuente (PDF de la cátedra, ya no se guarda local): `datos-en-microservicios.pdf`

## Shared Database = anti-patrón (si es permanente)
Separar código pero compartir DB camufla el monolito: acoplamiento estructural, dependencia de deploy por cambios de esquema, no se escala datos por separado, lock-in de motor. Válido solo como **etapa transitoria** de migración.

## Database per Service
Cada servicio es **único dueño** de sus datos. Acceso externo solo por **(1) su API** o **(2) sus eventos**.
Variantes: tabla separada (lógico) · schema separado (lógico fuerte) · instancia separada (físico). No hace falta una instancia física por servicio.
- Beneficios: desacople real, libertad tecnológica, escalado y deploy independiente.
- Costos: se pierden JOINs y FKs entre servicios (IDs lógicos; la validación es del dueño), respuestas compuestas requieren varias llamadas.

## Componer datos entre servicios
- **API Composition:** una capa llama a varios servicios y ensambla. Simple, suficiente para la mayoría.
- **CQRS:** separar escritura de lectura; vistas materializadas desnormalizadas actualizadas por eventos → **consistencia eventual** en lectura.

## Transferir ownership (migración)
Fase 1 Expand (copiar, leer del nuevo) → Fase 2 Coexistencia controlada (**un escritor autorizado por dato**) → Fase 3 Corte de ownership (momento explícito) → Fase 4 Contract (limpiar sincronizadores).

## Consistencia
- Se pierde ACID global. 2PC/XA existe pero agrega acoplamiento, latencia y problemas de disponibilidad → no es default.
- **Fuerte vs eventual:** el desafío es decidir qué operaciones necesitan cada una.
- **Saga:** secuencia de transacciones locales + **transacciones compensatorias** (no son rollbacks: son nuevas operaciones de negocio — devolver stock, anular cobro).
  - **Coreografía:** cada servicio reacciona a eventos; sin coordinador; más difícil de razonar/depurar.
  - **Orquestación:** orquestador dirige pasos y compensaciones; más observable y modificable.
- **Dual write:** guardar en DB + publicar evento no es atómico.
- **Transactional Outbox:** insertar entidad + fila `outbox` en la **misma transacción local**; un **relay** publica. Garantiza no perder la intención; entrega **at-least-once** (puede duplicar).
- **Idempotencia obligatoria en consumidores:** tabla `mensajes_procesados(mensaje_id PK)` insertada **en la misma transacción** que el efecto (`ON CONFLICT DO NOTHING`).
- Regla de diseño: lo que debe ser atómico, va **en el mismo servicio**.

## SQL vs NoSQL
- **SQL** (PostgreSQL/MySQL): relaciones complejas, JOINs, ACID estricto (finanzas, reservas), auditoría. No descartarlo prematuramente (pooling + réplicas + caché aguanta mucho).
- **Documentales** (MongoDB): jerárquico/semiestructurado, entidad leída como unidad, esquema variable.
- **Clave-valor** (Redis, Memcached): caché, sesiones, contadores.
- **Columnares wide-column** (Cassandra): escritura masiva, patrones predecibles. ≠ analíticos (BigQuery, ClickHouse).
- **Grafos** (Neo4j), **series de tiempo** (InfluxDB, Prometheus).
- Criterios: estructura del dato, tipo de queries, patrón de acceso, escala, consistencia requerida, **madurez del equipo**. Nunca "porque escala".

## MongoDB
- Documento BSON, colecciones sin esquema impuesto, `_id` ObjectId.
- **Embedding** si el subdocumento no tiene vida propia, se lee con el padre y no crece sin límite. **References** si tiene ciclo de vida independiente, se comparte o crece sin límite.
- Pros: modelo natural jerárquico (1 lectura vs JOIN de 7 tablas), mapeo directo a structs Go, sharding, atomicidad por documento.
- Riesgos: `$lookup` frecuente = mal modelado · lecturas de secundarios desactualizadas (read preference / read concern `local` vs `majority`) · **esquema flexible ≠ sin diseño** (usar JSON Schema validation, campo discriminador) · transacciones multi-documento caras · duplicación al desnormalizar (decisión explícita) · **índices según patrones de consulta** (sin índice = collection scan) · aggregation pipeline difícil para analítica.

## Puntos clave para defender
Shared DB es transitoria · IDs lógicos sin FK · Saga ≠ rollback · Outbox + consumidor idempotente · elegir DB por características, no moda.
