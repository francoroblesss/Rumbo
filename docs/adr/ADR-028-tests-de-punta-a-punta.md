# ADR-028 — Tests de punta a punta en el CI (bonus)

- **Fecha:** 2026-11-06
- **Estado:** aceptada
- **Relacionado:** bonus "Tests de punta a punta (e2e)" anunciado en la Entrega 2 (`SPEC.md` §7); [ADR-024](ADR-024-alcance-de-la-presentacion.md) (flujo de la Presentación).

## Contexto
- El bonus anunciado: tests automatizados del flujo principal a través del sistema levantado, que corren en el CI.
- Hoy el CI (`.github/workflows/ci.yml`) compila y testea cada módulo Go por separado: tests unitarios y de integración contra una dependencia real (testcontainers). Ninguno verifica que los servicios **juntos** cumplan el flujo, ni que los contratos internos entre servicios cierren de punta a punta.
- El flujo de la Presentación (ADR-024) cruza el gateway, Identidad, Integración, Operaciones, RabbitMQ y Notificaciones, con pasos asíncronos (outbox, consumidores).
- Teórico 7: las pruebas e2e verifican el comportamiento ante fallas ("sin duplicados" ante un reintento). Teórico 2: lo asíncrono es eventualmente consistente, así que un test no puede esperar el resultado en el mismo instante.

## Decisión
1. **Módulo Go `pruebas/e2e`** (en `go.work`), con `net/http` y `testing` de la librería estándar: **sin herramientas nuevas**. Los tests hablan **solo con el gateway** (`E2E_GATEWAY_URL`, por defecto `http://localhost:8080`), como un cliente real.
2. **El sistema se levanta con el mismo procedimiento único** que usa cualquiera: `cp .env.example .env` y `docker compose up --build -d --wait`, sin el perfil de observabilidad.
3. **Lo asíncrono se espera con sondeo y plazo:** una función `esperarHasta(t, plazo, condición)` consulta cada 1 s hasta que se cumple o vence el plazo (15 s), y falla con el último valor visto. Nunca un `time.Sleep` fijo. Los tests pasan por el gateway con su límite de tráfico (1 pedido por segundo por token, ráfaga de 10): ante un `429`, esperan lo que dice `Retry-After` y repiten el mismo pedido, como haría un consumidor real (teórico 9).
4. **Escenario principal** (el flujo de ADR-024): token del sistema externo → disponibilidad → reserva → el operador lo ve en franjas y en el listado → `EN_CURSO` → `COMPLETADO` → el sistema externo lo ve `COMPLETADO` por la API pública → el cliente de la organización tiene los avisos en su bandeja.
5. **Escenarios cortos:**
   - la misma reserva repetida con la misma `Idempotency-Key` devuelve la misma respuesta y ocupa un solo lugar;
   - franja llena → `409 SIN_DISPONIBILIDAD`;
   - cancelación del cliente desde la web → la franja recupera el lugar;
   - sin token → `401`.

   El aislamiento entre organizaciones (CA-40) no entra en el e2e: el único sistema externo sembrado es de la misma organización que el usuario cliente, así que por la API no se pueden crear datos de otra. Lo cubren los tests de la API de Operaciones.
6. **Job `e2e` en el CI**, que corre en cada PR después de los tests de cada módulo. Si falla, imprime `docker compose ps` y `docker compose logs` antes de bajar todo.

## Alternativas consideradas
- **Postman/Newman, Playwright o un framework de BDD.** Descartadas: otra herramienta y otro lenguaje para algo que `net/http` y `testing` resuelven. Además, el grupo ya escribe y lee tests en Go.
- **Tests e2e de la web en el navegador (Playwright).** Descartada por plazo: la web se verifica a mano en su PR y los e2e cubren los contratos entre servicios, que es lo que más se rompe al integrar.
- **Correrlos contra la nube (Render).** Descartada: gastaría las horas del plan gratuito (ADR-026) y dependería del arranque en frío.
- **`time.Sleep` para esperar lo asíncrono.** Descartada: es lento o es frágil (teórico 2: consistencia eventual con un plazo definido).

## Consecuencias
- **Ganamos:**
  - el CI detecta cuándo un cambio en un servicio rompe el flujo completo o el contrato con otro;
  - el escenario principal es también la demo de la Presentación, escrita y reproducible.
- **Pagamos:**
  - el job tarda varios minutos (compilar las imágenes y levantar el sistema);
  - el sistema completo tiene que entrar en un runner de GitHub (7 GB de RAM): se corre sin observabilidad;
  - los plazos de espera pueden volver inestables los tests si el runner está lento. Por eso el plazo es amplio (15 s) frente al retraso esperado (~1 s);
  - el límite de tráfico del gateway alarga el job (esperas por `429`).

## Cómo se verifica
- El job `e2e` está en verde en el PR que lo agrega y en los siguientes.
- Romper a propósito, en una rama de prueba que no se mergea, el consumidor de Notificaciones o el avance de estado hace fallar el escenario principal.
