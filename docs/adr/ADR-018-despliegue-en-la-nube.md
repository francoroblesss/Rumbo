# ADR-018 — Despliegue en la nube: Vercel para la web y Railway para el backend

- **Fecha:** 2026-10-16 (acordado por el grupo el 2026-10-15)
- **Estado:** aceptada (se implementa para la Presentación Grupal)

## Contexto
- La consigna (§4.1) pide que la primera versión operativa de la capacidad publicada esté **en un entorno accesible** y se mantenga operativa hasta el final de la evaluación. Para la Presentación Grupal (§6) pide la capacidad propia **desplegada en la nube y accesible mediante una URL pública**.
- En la clase de presentación del proyecto (minuto 00:13, `insumos-arquitectura.md`) se pidió usar **opciones simples y gratuitas, no AWS**: GitHub Pages, Vercel, Netlify, Render o Railway.
- El sistema ya corre entero en contenedores (`compose.yaml`), con una imagen por servicio construida desde `servicios/<servicio>/Dockerfile`.
- El backend necesita procesos **siempre prendidos**: consumidores de RabbitMQ, relays del outbox y la tarea de vencimiento. También necesita MySQL, MongoDB, RabbitMQ y Memcached.
- [ADR-015](ADR-015-capacidad-y-costos.md) ya estimó el costo con los precios de Railway: ≈ USD 24 por mes con el uso medido.

## Decisión
- **Aplicación web → Vercel** (plan Hobby, USD 0). Es una aplicación estática de Vite (`npm run build`), y Vercel la publica desde el repositorio en cada push a `main`. Llama al gateway por su URL pública (`VITE_API_URL`).
- **Backend → Railway**, un proyecto con un servicio por contenedor de `compose.yaml`:
  - **Despliegue:** cada servicio Go se construye desde su `Dockerfile` del repositorio, con el mismo contexto de build que en Compose. MySQL, MongoDB, RabbitMQ, Memcached y NGINX usan las mismas imágenes que en local.
  - **Red:** los servicios se hablan por la red privada de Railway (`<servicio>.railway.internal`). **Solo el API Gateway tiene dominio público**, y es la URL que se publica en `docs/contracts/README.md` y en el `README.md`. Así se mantiene el punto de entrada único (ADR-001).
  - **Secretos:** se cargan como variables de Railway y de Vercel, los mismos nombres que `.env.example`. Nunca van al repositorio (consigna §2.12).
  - **Balanceo:** se mantienen NGINX y las dos réplicas de Integración ([ADR-009](ADR-009-balanceo.md)), porque la distribución tiene que poder verse en la observabilidad. El `resolver` de NGINX pasa a ser una variable: en Docker es `127.0.0.11` ([ADR-017](ADR-017-reconexion-broker-y-balanceador.md)); en Railway, el DNS de su red privada.
- **No se despliegan** `clima-mock` (en la nube se usa la URL real del grupo del aeropuerto, ADR-012) ni, por ahora, `grafana/otel-lgtm`. LGTM es para desarrollo (ARCHITECTURE §10). Cómo se observa el sistema en la nube se decide al desplegar, y si cambia algo de ADR-014, va en un ADR nuevo.

## Alternativas consideradas
- **AWS, GCP o Azure.** Descartadas: en clase se pidió explícitamente no usar AWS, y su configuración (VPC, IAM, balanceadores) es mucho más de lo que el problema necesita.
- **Vercel o Netlify para el backend.** Descartadas: ejecutan funciones *serverless*, no contenedores siempre prendidos. Los consumidores de RabbitMQ y los relays del outbox no pueden vivir ahí.
- **Supabase.** Descartada: ofrece PostgreSQL, y el motor relacional es MySQL ([ADR-005](ADR-005-mysql-como-motor-relacional.md)).
- **Render.** Viable, pero su plan gratuito duerme los servicios web después de un rato sin tráfico, y los *background workers* no son gratuitos. Railway corre las mismas imágenes de Docker, cobra por uso y es el proveedor con el que ya se estimaron los costos (ADR-015).
- **Las réplicas nativas de Railway en lugar de NGINX.** Descartadas por ahora: el balanceo interno de Railway no deja ver en nuestros logs qué réplica atendió cada pedido, y eso lo exige la consigna (§2.3).

## Consecuencias
- **Ganamos:**
  - la misma topología que en local (mismas imágenes, mismos nombres de variables), así que lo que se probó con Compose es lo que corre en la nube;
  - una sola URL pública, la del gateway;
  - despliegue automático desde `main`.
- **Pagamos:**
  - ≈ USD 24 por mes con el uso medido (ADR-015). No entra en el crédito del plan Hobby (USD 5): hace falta el plan Pro o pagar el excedente. Para bajar el costo:
    - no desplegar Recursos ni Notificaciones hasta que tengan funcionalidad;
    - espaciar los healthchecks de MongoDB y RabbitMQ, que son la mayor parte de la CPU promedio (ADR-015);
  - el `resolver` de NGINX y las URL internas cambian entre local y nube (por variables de entorno);
  - las bases en contenedores de Railway no tienen copias de seguridad administradas. Se acepta, porque es un entorno de evaluación.

## Cómo se verifica (Presentación)
- `POST /v2/traslados` contra la URL pública del gateway con un token del sistema externo devuelve `202`, y la solicitud aparece en Operaciones.
- Ningún servicio, salvo el gateway, responde desde Internet.
- La web publicada en Vercel completa el flujo principal contra el gateway desplegado.
- La URL pública figura en `README.md` y en `docs/contracts/README.md`, y el grupo consumidor la usa.
