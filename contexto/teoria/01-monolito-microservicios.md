# Teórico 1 — Monolito → Microservicios + estrategias de migración

Fuente (PDF de la cátedra, ya no se guarda local): `clase01-monolito-y-microservicios.pdf`

## Definiciones
- **Monolito:** una única unidad de despliegue y versionado; todos los módulos en el mismo proceso. *Es una característica de despliegue, no una condena de calidad.*
- **Monolito modular:** límites explícitos entre módulos (interfaces internas) dentro del mismo proceso. Separación **lógica** (depende de la disciplina del equipo). Arquitectura legítima y subestimada.
- **Monolito distribuido (anti-patrón, práctico):** servicios separados pero tan acoplados que hay que desplegarlos juntos.
- **Microservicios:** servicios autónomos alrededor de **capacidades de negocio**, comunicados por red. "Micro" = alcance de responsabilidad, no tamaño de código. Separación **física** (proceso + memoria propios; solo red).

## Principios de microservicios
1. **Límites de dominio** — cada servicio encapsula su capacidad **y sus datos**; nadie toca la DB de otro.
2. **Autonomía** — deploy y escalado independiente. El aislamiento de fallos **se diseña** (timeouts, límites, degradación); no es automático.
3. **Interfaces explícitas** — solo contratos (REST, eventos, mensajes).
4. **Deploy independiente** — pipeline CI/CD propio.

## Ventajas del monolito
Simplicidad operacional, desarrollo inicial rápido, debugging lineal, sin latencia de red (ns vs ms), transacciones ACID simples, consistencia de versiones.

## Límites del monolito a escala
Acoplamiento que crece · deploys todo-o-nada · **fault propagation** (OOM de un módulo mata todo) · escalado horizontal indiferenciado · barreras tecnológicas (mismo lenguaje/runtime).

## Costos de microservicios
Overhead operacional (N servicios = N deploys/monitoreos) · latencia de red y más puntos de fallo · **consistencia distribuida** (se pierde ACID → Saga) · debugging distribuido (**Correlation ID** + distributed tracing).

**Condiciones mínimas antes de distribuir:** CI/CD reversible, observabilidad centralizada, gestión de config/secretos, contratos versionados.

## Cuándo migrar (y cuándo no)
- Monolito es correcto para: equipos chicos (<8-10 backend), dominio aún no claro, consistencia transaccional crítica, poca madurez operativa.
- **Señales para migrar:** deploy bloquea productividad · un módulo necesita escalar distinto · conflictos de equipo frecuentes · un área necesita otra tecnología · fallos se propagan entre módulos sin relación.
- *"Don't start a new project with microservices"* (Fowler, Monolith First). Paso intermedio: **mejorar la modularidad**.

## Estrategias de migración (se combinan)
| Estrategia | Ámbito | Mecanismo | Riesgo |
| --- | --- | --- | --- |
| **Strangler Fig** | Extraer módulos a servicios | Proxy/router de borde redirige rutas gradualmente | Variable; datos requieren plan propio |
| **Feature Flags** | Rollout de cualquier cambio | Flag evaluado en runtime (entorno/usuario/%/región) | Bajo; no revierte efectos ya hechos. Flags viejos = deuda técnica |
| **Branch by Abstraction** | Cambiar implementación interna con muchos consumidores | Interfaz → nueva impl → redirigir → eliminar vieja | Bajo |
| **Parallel Run / Shadow** | Validar antes del cutover | Procesa duplicado, compara, no entrega | Bajo; **cuidado con efectos secundarios** (no duplicar compras) |
| **Expand and Contract** | Cambios de API/esquema | Expand (agregar) → Migrate → Contract (borrar) | Bajo; compatibilidad hacia atrás |
| **Big Bang Rewrite** | Reemplazo total | Corte único | Alto (Netscape, eBay Denali) |

Secuencia típica: delimitar capacidad + ownership de datos → Expand/Contract → Branch by Abstraction → Parallel Run (solo lecturas) → Feature flag + canary para escrituras → Strangler Fig y retiro.

**Desafío clave del Strangler: los datos.** FKs entre dominios → referencias lógicas (API o proyección por eventos). Un único dueño por dato.

## Puntos clave para defender
- Monolito ≠ malo. Microservicios ≠ objetivo en sí.
- Los beneficios de microservicios solo aparecen con diseño cuidadoso.
- La arquitectura correcta resuelve el problema real *en su etapa actual*.
