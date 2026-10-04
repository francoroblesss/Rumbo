# Teórico 3 — Caché

Fuente (PDF de la cátedra, ya no se guarda local): `cache.pdf`

## Ideas base
- Caché = **copia temporal**; **no es fuente de verdad**. Puede estar ausente, vencida o desactualizada.
- `hit rate = hits / (hits + misses)`. Alto hit rate no alcanza como meta; medir junto con latencia y carga de la fuente. Errores/timeouts de la caché se miden aparte.
- Un miss paga el costo completo (fuente + poblar caché).
- **Buenos candidatos:** se leen mucho, compartidos, caros de reconstruir, toleran desfase, clave estable.
- **Malos candidatos:** decisiones irreversibles — stock final al comprar, saldo, permisos revocados, estado de pago. La UI puede mostrar aproximación; **la operación crítica valida contra la fuente**.

## Frescura = requisito de negocio
Inmutables (TTL largo) · cambio infrecuente (TTL moderado + invalidación) · altamente dinámicos (no cachear).
Pregunta correcta: *"¿cuánto desfase puede ver un usuario y qué daño causa?"*

| Concepto | Disparador |
| --- | --- |
| Expiración | Se cumplió el TTL |
| Invalidación | Cambio conocido en la fuente → borrar/actualizar clave |
| Expulsión (eviction) | Presión de memoria (LRU, LFU, FIFO) |
Combinar: invalidación al escribir + TTL como red de seguridad.

## Claves
Incluir **todo lo que cambia la respuesta** (idioma, moneda, rol) o se filtran datos entre usuarios.
Convención: `eventos:{id}:publico:v1`, `usuarios:{id}:perfil:v2` (práctico usa `ITEM:<id>`, `PRODUCTO:12`). Sufijo de versión para cambiar formato.

## Dónde vive
- **Local** (in-process, ej. ccache): mínima latencia; cada réplica tiene su copia (divergen), se vacía al reiniciar.
- **Distribuida** (Redis/Memcached): compartida entre réplicas; paga red, serialización y es otra dependencia que puede fallar → timeouts y degradación.
- **Dos niveles (L1+L2):** menor latencia pero invalidar L2 no borra L1 de cada réplica. Solo si se justifica.

## Patrones
| Patrón | Quién carga | Riesgo dominante |
| --- | --- | --- |
| **Cache-aside** (lazy) | App ante miss | Miss inicial, invalidación. **Default recomendado** |
| Read-through | Capa de caché | Acopla infra a la fuente |
| Write-through | Escritura síncrona en fuente + caché | Fallos parciales, latencia de escritura |
| Refresh-ahead | Proceso anticipado | Trabajo inútil |
| Write-behind | Caché primero, fuente después | **Pérdida de datos**; no para compras/pagos |
Escritura en cache-aside: **actualizar la fuente → al confirmar, `DEL` de la clave** (más seguro que actualizar ambas).

## Fallos
- **Cache stampede / thundering herd:** clave popular vence y todos van a la DB. Defensas: lock por clave, **jitter en TTL**, refresh-ahead, stale-while-revalidate, límites.
- **Cold start:** caché vacía tras deploy/reinicio. Calentamiento selectivo, carga progresiva.
- **La caché puede caer:** timeout acotado; fallback a la fuente **solo si la fuente aguanta** (si no, cascada). A veces degradar es mejor.
- **Stale-while-revalidate:** servir vencido mientras se refresca; definir límite fresco y límite stale.

## Anti-patrones
Cachear todo · claves incompletas · sin TTL ni capacidad · decidir algo crítico con la caché · fallback ilimitado a la DB · ignorar métricas de eviction/miss.

## 7 preguntas antes de cachear (usar en el proyecto)
1. ¿Qué operación es costosa y cuál es su fuente de verdad? 2. ¿Quién la necesita y cuánto se repite? 3. ¿Cuánto puede estar desactualizada? 4. ¿Qué evento invalida cada clave derivada? 5. ¿Qué patrón corresponde? 6. ¿Qué pasa ante miss, expiración, evicción y caída? 7. ¿Qué métricas demostrarán que funciona?
