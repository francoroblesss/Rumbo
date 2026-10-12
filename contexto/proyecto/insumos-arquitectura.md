# Rumbo — Insumos para la arquitectura (no son decisiones)

> Ideas técnicas que venían en `Rumbo.docx` y `Rumbo - Ampliaciones del dominio.docx` (ya borrados) y no entran en `dominio.md`. Son **puntos de partida para evaluar** en los ADR, no decisiones tomadas. Las reglas del negocio están en `dominio.md`.

## 1. Servicios sugeridos por el profesor — RESUELTO en `docs/adr/ADR-001-limites-de-los-servicios.md`
Según el grupo, el profesor sugirió y aprobó preliminarmente **tres servicios**, en una conversación breve y antes de conocer las precisiones del dominio:
- **Personas**
- **Insumos y gestión de los vehículos**
- **Notificaciones**

Se pueden usar, ajustar o reemplazar. La consigna exige un mínimo de 3 microservicios + API Gateway, y se admiten más si se justifican. Antes de elegir, hay que responder cuatro preguntas:
1. **¿Qué incluye Personas?** Usuarios, clientes, pasajeros y choferes tienen responsabilidades distintas.
2. **¿Qué significa Insumos?** Registrar la carga transportada no es administrar el inventario del cliente (que está fuera de alcance).
3. **¿Dónde van solicitudes, planes, traslados, recorridos y asignaciones?** Concentran el comportamiento principal y no aparecen en las tres etiquetas.
4. **¿Qué avisos concretos justifican Notificaciones?** (ver `dominio.md` §13)

La elección tiene que explicar responsabilidades, relaciones y propiedad de los datos (D1). **Los roles y los paneles no determinan por sí solos la cantidad de servicios.** El documento original descartaba la idea de "siete microservicios" de un PDF anterior: no había razones para que fueran siete.

**Indicación posterior del profesor (en clase, 00:12:22 – 00:12:56):** la funcionalidad publicada para el otro grupo conviene separarla en otro microservicio, simple; mínimo 3 servicios, pueden ser más. Se aplicó en ADR-001: servicio **Integración de carga**.

## 2. Cómo el dominio genera los requisitos de la consigna
| Requisito | Dónde aparece en Rumbo |
| --- | --- |
| Concurrencia / consistencia (§2.8, D4) | Dos operadores asignan a la vez el mismo chofer o vehículo. No se pueden confirmar dos asignaciones incompatibles (`dominio.md` §8). |
| Mensajería / evento de dominio (§2.5) | Cambios de estado de solicitudes, traslados y recorridos. Los avisos al cliente, al chofer y al operador. |
| Búsqueda (§2.6, D6) | Miles de traslados, filtrables por cliente, fecha, estado, chofer, tipo de servicio y origen/destino. |
| Caché (§2.7, D7) | Información muy consultada y relativamente estable, por ejemplo disponibilidad/calendario o los recorridos del día. **No se elige todavía**: la consigna exige una necesidad observable y medir el impacto. |
| Integración: capacidad propia (§4, D8) | "Solicitar traslado" genérico, v2 (`dominio.md` §15, ADR-006). |
| Integración: consumo (§4, D9) | Clima del sistema de aeropuerto; el flujo se define al conocer su contrato. |
| Resiliencia (§2.9, D10) | ¿Qué hace Rumbo si el sistema del aeropuerto está caído o lento? Se demuestra con una caída provocada. |
| Bonus (§7) | Seguimiento en tiempo real (GPS) quedó fuera de alcance y es candidato a desafío adicional. |

## 3. Contrato "Solicitar traslado de carga" (D8) — RESUELTO en `docs/adr/ADR-002-contrato-propio.md`
- Qué se informa al **recibir** la solicitud (ej. identificador y estado `RECIBIDA`).
- Qué se confirma **después de la revisión del operador** (aceptada, contrapropuesta, rechazada) y cómo responde el consumidor a una contrapropuesta.
- **Cómo se entera el sistema externo** de cada cambio de estado (consulta o aviso).
- **Propuesta para la Entrega 1:** preparar una simulación (mock) de las respuestas del contrato para que el otro grupo pueda empezar a probar. La consigna menciona el mock en el título de la entrega pero no detalla sus condiciones, así que esto es una propuesta y no un requisito explícito.

## 4. Validación del dominio con casos completos
Antes de cerrar la arquitectura, narrar de punta a punta:
1. Un paciente recurrente con ida y vuelta.
2. Un recorrido escolar.
3. Un pedido de stock que llega desde el otro grupo.
4. Una cancelación (con 24 h o más, y tardía).
5. Una avería, con el alcance acotado (registrar + reasignar antes de salir; si ocurre en viaje, interrumpido).

Estos casos siguen pendientes: conviene narrarlos antes de implementar Operaciones (sirven también como base de los criterios de aceptación de `SPEC.md`).

## 5. Lo dicho en la clase de presentación del proyecto
Fuente: `contexto/Transcripcion - Universidad Catolica de Cordoba.txt` (transcripción automática; los minutos sirven para buscar en el audio).

**Confirmado por el grupo con el audio:**
- **00:00:01 – 00:00:13:** "dos tipos de base de datos; para tres microservicios, tres bases de datos; no una base de datos compartida" → una base por microservicio (aplicado en ADR-003).
- **00:00:18 – 00:01:22:** "no crear caché local por código desde 0 en Go; usen lo que ya existe; vale para caché, búsqueda, etc." → siempre librerías o herramientas existentes (regla en `../convenciones-codigo.md`).
- **00:19:52 – 00:20:34:** la historia de las decisiones tiene que verse ("tal fecha 3 servicios, tal fecha 4; no borro lo anterior"). Antes del primer commit se corrige en el mismo ADR; después, un ADR nuevo reemplaza al anterior.
- **00:21:14 – 00:21:25:** agregar a los docentes como colaboradores del repositorio (lo hace el grupo).

**Sin confirmar con el audio (para decisiones futuras):**
| Minuto | Qué se dijo | Para qué decisión |
| --- | --- | --- |
| 00:01:44 – 00:02:47 | Al actualizar datos, mantener sincronizados el motor de búsqueda y la caché con eventos; definir cuánta demora se acepta y cómo se asegura la consistencia, al menos en la operación más importante | D6, D7 |
| 00:03:06 – 00:04:05 | Elegir una función importante y documentar cómo se reacciona si se cae; en ese archivo tienen que estar **los logs** y la explicación | D10, `POSTMORTEM.md` |
| 00:04:41 – 00:05:34 | Logs bien estructurados y legibles; métricas con un lugar para verlas (ej. cuántos 404 hubo la semana pasada) | D11 (retención de al menos una semana) |
| 00:05:44 – 00:07:12 | Tests unitarios y de integración como en el TP anterior, más test de carga ante picos; escalones del tipo 1, 2, 5, 10… 100/s, 1000 cada 10 s; ver el efecto del balanceo y la caché | D13 (comparar con y sin caché o balanceo) |
| 00:07:27 – 00:07:55 | No exigen Git Flow, pero sí un método de trabajo con Git, explicado; "proyectos por WhatsApp no sirven" | Estrategia de ramas (§2.12) |
| 00:08:00 – 00:08:49 | README con las partes del proyecto y los pasos para ponerlo en marcha; que corra fácil en cualquier computadora (Docker) | README |
| 00:13:01 – 00:14:00 | Desplegar con opciones simples y gratuitas, no AWS: GitHub Pages, Vercel, Netlify, Render, Railway y otras que se ven en el teórico | Despliegue de Integración de carga |
| 00:14:47 – 00:16:07 | Se puede usar IA, pero con documentación tipo *specs* y alguna metodología: reglas de negocio, decisiones, arquitectura de **alto y bajo nivel** y organización interna de los servicios | `ARCHITECTURE.md` (sección de bajo nivel), README |
| 00:25:11 – 00:25:40 | Los bonus tienen que funcionar y no salvan un proyecto flojo | Bonus (§7) |

## 6. Devolución de la Entrega 1 (profesor, 2026-10-09)
| Comentario | Qué se hizo |
| --- | --- |
| "Traslado = función con 3 parámetros: cosa, inicio y destino. Para que gestión de almacenamiento nos consuma tiene que ser más abstracto que cerrado (cambiar persona por cosa en los parámetros)" | Contrato genérico v2: `cosa`, `origen` y `destino` obligatorios, el resto opcional (ADR-006) |
| "Fijarse los endpoints v1, hay que actualizar versiones (que no quede desfasada la documentación con el código)" | Nueva versión mayor `/v2`; v1 marcada como reemplazada; API interna versionada `/api/v1/…`; documentación y gateway alineados (ADR-006) |
| "Cambiar Postgre a MySQL (más simple)" | MySQL 8.4 con transacción + `SELECT … FOR UPDATE` por recurso (ADR-005) |
| "No jugársela con las comunicaciones, tiempos, etc. (está bien, pero prestar atención)" | Sin cambios por ahora. En la Entrega 2, al **validar D5**, ajustar los timeouts (800 ms, 2 s, 5 s) con **latencias medidas** y documentar de dónde sale cada número |
| "Tiene bastante contexto, bien" · "el profe Tomi banca nuestro proyecto" | Seguir con la misma modalidad: dominio y decisiones documentados antes del código |
