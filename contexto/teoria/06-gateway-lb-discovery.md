# Teórico 6 — API Gateway, Load Balancing y Service Discovery

Fuente (PDF de la cátedra, ya no se guarda local): `api-gateway-load-balancing-y-service-discovery.pdf`
Caso: tienda online con Catálogo, Búsqueda, Carrito, Órdenes, Pagos.

## Tres preguntas distintas
- **Load Balancing:** ¿a qué **réplica sana** envío esta request?
- **API Gateway:** ¿qué API expongo y **bajo qué política** la dejo pasar?
- **Service Discovery:** ¿qué instancias existen **ahora mismo**?
Objetivo: **entrada pública estable** mientras la topología interna cambia.

## Load Balancing
- Réplicas por: capacidad, tolerancia a caída de instancias, deploy gradual.
- Algoritmos: **round-robin** (capacidad y costo similares) · **weighted RR** · **least connections** (cuidado con réplica nueva → **slow start**) · **least response time** · (práctico) **hashing consistente**.
- **Health checks:** vivo ≠ listo ≠ dependencia degradada. Readiness demasiado amplia también daña. Varias verificaciones consecutivas para evitar **flapping**. (K8s: liveness / readiness / startup probes.)
- **Sticky sessions** = estado en memoria → preferir **servicios stateless** con sesión externa (Redis).
- Ubicaciones: frente al borde público, interno por servicio, o en librería cliente.
- Un LB no define autenticación, routing de negocio, cuotas ni versionado.

## API Gateway
- Punto de entrada de clientes externos; **routing semántico** (host, path, método, versión, audiencia).
- Estabilidad del contrato; versionado: primero **evolución compatible**; `/v2` no arregla incompatibilidades.
- Composición/adaptación **con límites**: no reglas de dominio, no transacciones.
- Reverse proxy (reenvía) ≠ Gateway (gobierna API) ≠ **BFF** (adapta a una audiencia).
- **Anti-patrones:** gateway como monolito de negocio · agregación sin límite · punto único de falla (sin réplicas) · bypass informal.

## Identidad en el borde
- TLS + token firmado; el gateway valida firma, vigencia, emisor, audiencia → 401.
- **Autenticación centralizada, autorización distribuida**: el servicio dueño decide qué puede hacer esa identidad sobre SU recurso.
- Propagar identidad por **canal confiable diseñado**; el gateway **borra headers de identidad del cliente** (`X-User-Id` fabricado). Servicios no accesibles sin pasar por el gateway.
- Límites de tamaño, **CORS explícito** (no autentica), separación pública/privada/interna. Auditoría con correlation id, sin tokens ni datos sensibles.

## Rate limiting
- Rate limit (req/tiempo) ≠ cuota (presupuesto largo) ≠ concurrencia (trabajo simultáneo).
- Dimensión: identidad, app, API key, tenant, endpoint; **IP solo auxiliar** (NAT).
- **Token bucket** (burst acotado + recarga) vs ventana fija (efecto borde).
- Exceso → **429** pronto (+ `Retry-After` si hay estimación). No encolar sin límite ni fingir éxito.
- **Distribuido:** 4 réplicas × 100 tokens locales = 400 efectivos → estado compartido (precisión, +dependencia) o presupuestos locales (aproximación).

## Service Discovery
- Registro: nombre lógico → instancias + salud. **Auto-registro** (heartbeats) o **registro por la plataforma**. Riesgos: info atrasada, instancias huérfanas.
- Lista local cacheada (disponibilidad vs destino viejo).
- **Client-side** (cliente elige; ej. Eureka + Ribbon) vs **server-side** (gateway/LB resuelve). DNS, registros dedicados, orquestadores.
- Discovery dice *qué instancias son elegibles*; el LB elige *cuál atiende esta request*.

## Diseño de referencia
Clientes → **LB público** → **Gateway (réplicas)** → [401 / 429] → routing semántico → **LB interno** por servicio (consulta Discovery) → réplicas. Dos niveles de balanceo con preguntas distintas. Rutas de admin no publicadas por defecto.
Casos: Netflix Zuul (borde) + Eureka/Ribbon (discovery+LB cliente), Uber (gateway que evoluciona), Kubernetes probes.

## Trade-offs
Sin gateway si API cohesionada y simple · gateway si múltiples capacidades/clientes/políticas repetidas · solo LB si réplicas homogéneas sin routing semántico · BFF si audiencias realmente distintas · discovery dinámico si instancias cambian seguido.
Reglas transversales: stateless, gateway replicado y monitoreado, least privilege, negocio y autorización en cada servicio.
