# Teórico 7 — Resiliencia ante fallos parciales

Fuente (PDF de la cátedra, ya no se guarda local): `resiliencia-ante-fallos-parciales.pdf`

## Conceptos
- **Disponibilidad** (atiende ahora) ≠ **confiabilidad** (comportamiento correcto en el tiempo) ≠ **resiliencia** (contiene la perturbación, conserva capacidades, se recupera).
- **Fault** (condición anómala) → **failure** (se incumple el resultado). Contener faults evita **fallas en cascada**.
- **La lentitud puede ser peor que un error:** retiene conexiones, memoria, goroutines; crece la cola; más retries.
- Idea central: limitar radio de impacto, preservar trabajo útil, **decir la verdad sobre el estado**, facilitar la recuperación.

## Timeouts y deadlines
- Timeout de conexión / lectura / inactividad / **deadline** (presupuesto total). El deadline es una pregunta de producto.
- Presupuesto ilustrativo (2000 ms): gateway 100, validación 150, Pagos con intentos 1300, respuesta 250, margen 200.
- **Propagar el deadline restante** (no reiniciar reloj). En Go: `context.Context` (`WithTimeout`, `http.NewRequestWithContext`).
- **Timeout ≠ fracaso de negocio: puede ser resultado desconocido.** Cancelar la espera no deshace el efecto.

## Retries
- Clasificar: conexión cortada (tal vez; el efecto pudo ocurrir) · sobrecarga con espera (sí, con backoff) · payload inválido / sin autorización / pago rechazado (no) · lectura (usualmente sí) · creación/cobro (**solo con idempotencia**).
- **Idempotency key** (Stripe): clave estable por intención; servidor guarda clave+parámetros+resultado; mismo pedido → mismo resultado; otros parámetros → rechazo. Definir alcance, TTL, concurrencia, estados. Deshabilitar el botón no alcanza.
- **Backoff exponencial con tope + jitter**; **retry budget** (Google SRE). Los retries son "egoístas" (Amazon).
- **Amplificación:** 3 capas × 3 intentos = 27 llamadas. **Un solo dueño del retry** (capa inmediatamente superior).

## Circuit Breaker
Closed → (umbral de fallos/lentitud) → **Open** (falla rápido) → (vence espera) → **Half-open** (pocas pruebas) → Closed/Open.
Política: ventana + volumen mínimo, qué cuenta como fallo (validación ≠ fallo; timeout sí), alcance por operación. No cancela llamadas en curso, no reemplaza timeout, no hace seguro el retry, no reconcilia.

## Bulkhead
Aislar capacidad: límites de concurrencia, pools separados, colas **acotadas**, capacidad reservada para lo crítico. **Bulkhead ≠ rate limiting** (concurrencia vs tasa). Costo: menor utilización conjunta.

## Fallback y degradación
Debe ser **semánticamente honesto**; no convertir errores en `200 OK`.
| Capacidad | Degradación posible | Peligrosa |
| --- | --- | --- |
| Recomendaciones | Omitir / "más vendidos" | Inventar personalizadas |
| Búsqueda | Resultados cacheados con aviso | Confirmar stock desde índice |
| Catálogo | Datos anteriores con timestamp | Presentarlos como precio final |
| Pagos | "Verificando", consultar estado | Aprobar/rechazar sin saber |
**Fail-open vs fail-closed:** decisión explícita (autorización → closed). El fallback también puede fallar: preferir simples (caché local, estático).

## Componer políticas
deadline → bulkhead → circuito → intento con timeout → (fallo repetible + presupuesto) backoff+jitter → degradar/rechazar. El orden de "decoradores" cambia qué se mide.
Reintentar contra todas las réplicas ante una dependencia común saturada multiplica la carga.
**Anti-patrones:** retry en cada capa, mismo timeout para todo, colas sin límite, toda excepción = transitoria, fallback a la misma dependencia caída, circuito compartido entre operaciones distintas, respuestas vacías exitosas, copiar umbrales.

## Extremo a extremo
Estados de negocio: **procesando → confirmada / rechazada / verificando (desconocido)**. Nada de spinner infinito.
Flujo recuperable: id estable de intención → envío único idempotente → si vence, conservar referencia → consultar estado → nueva intención solo si la anterior terminó.
Async: circuit breaker protege sync; **DLQ** para mensajes durables que agotaron política.

## Verificar
Pruebas de componente / integración / e2e (perder la respuesta tras confirmar → "verificando", sin duplicados). Métricas: timeouts, retries por request, circuitos abiertos, rechazos de bulkhead, fallbacks, resultados desconocidos, DLQ. **Chaos Engineering** con hipótesis, radio acotado y condición de detención.

## 4 preguntas antes de introducir un patrón
1. ¿Qué modo de falla concreto reduce? 2. ¿Qué recurso o promesa protege? 3. ¿Qué nueva falla puede crear? 4. ¿Cómo sabremos que funcionó y se recuperó?
