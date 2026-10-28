# Proyecto — Arquitectura de Software (UCC, ARQ 2026)

Repositorio del **proyecto grupal** de la materia Arquitectura de Software (Ingeniería en Informática, UCC).
La **consigna del proyecto final** está transcripta en `contexto/proyecto/consigna.md` (fuente: `Enunciado TP Final.pdf`, que queda solo local y no se sube); leer el .md, no el PDF.
El sistema a construir es **Rumbo**: su dominio está en `contexto/proyecto/dominio.md` (manda sobre cualquier otra fuente). Las decisiones de arquitectura se registran como ADR en `docs/adr/ADR-NNN-*.md` (índice y formato en `docs/adr/README.md`). La documentación entregable (consigna §3) vive en la raíz: `README.md`, `SPEC.md` y `docs/`; hay que mantenerla actualizada durante el desarrollo.

## Cómo trabajamos

- **Idioma:** todo en español (código con identificadores en español como hace la cátedra: `pedidos`, `clientes`, `ItemsService`, etc.; mezclar con inglés solo donde la cátedra lo hace — `controllers/`, `services/`, `repositories/`).
- **Rol de Claude:** implementa el código completo **y explica cada decisión** en términos de la teoría de la materia, para que el grupo pueda defenderla en la corrección/coloquio. Cada decisión de arquitectura debe poder justificarse con un trade-off visto en clase (ver `contexto/teoria/`).
- **Trabajo grupal:** el repo se comparte con el equipo vía GitHub. No asumir que quien pregunta escribió el código existente.
- **Estilo de código:** seguir `contexto/convenciones-codigo.md` (derivado del código de las demos/soluciones del docente). Ante la duda, imitar las soluciones resueltas resumidas en `contexto/practica/`.
- **Si el teórico y el práctico se contradicen en una decisión de código o arquitectura, GANA EL TEÓRICO** (`contexto/teoria/`). El práctico (`contexto/practica/`) es referencia de estilo, librerías y comandos, y suele simplificar a propósito (ej.: ignorar errores, sin idempotencia ni jitter, `context.TODO()`). Cuando se aparte del práctico por esta regla, decirlo explícitamente y citar el teórico.
- **No reinventar la rueda (indicación del profesor en clase, confirmada con el audio):** caché, búsqueda, mensajería, validación de tokens, rate limiting, etc. se resuelven con **librerías o herramientas existentes**, preferentemente las vistas en clase. Nunca implementar desde cero lo que ya existe; solo se escribe código propio para lógica de negocio y pegamento mínimo.
- **Antes de cada decisión de arquitectura**, releer los teóricos relevantes de `contexto/teoria/` y citarlos en el ADR.
- **Historial de decisiones visible (consigna §5 y profesor):** un ADR o una regla de negocio que ya fue commiteado **no se edita**: se marca como reemplazado y se crea uno nuevo que diga qué reemplaza y por qué. Solo lo que todavía no se commiteó puede corregirse en el mismo archivo con una línea "Revisión <fecha>: motivo".
- **Complejidad mínima que resuelva el problema.** La cátedra penaliza sobre-ingeniería ("no agregar un gateway/NoSQL/caché porque es lo que se usa"). Cada componente tiene que responder a un problema concreto.

## Mapa del contexto

| Archivo | Contenido |
| --- | --- |
| `contexto/materia.md` | Cursada, docentes, cronograma, stack, caso EventOS, repo de la cátedra |
| `contexto/teoria/*.md` | Resumen de cada apunte teórico (conceptos, reglas, trade-offs, anti-patrones) |
| `contexto/practica/` | Un resumen por clase práctica (1–8) + TP integrador 1-4; `README.md` = índice + comandos Docker |
| `contexto/convenciones-codigo.md` | Estructura de proyecto Go, capas, DI, interfaces/mocks, librerías y versiones |
| `contexto/checklist-arquitectura.md` | Preguntas que la cátedra exige responder antes de agregar cada pieza |
| `contexto/proyecto/dominio.md` | **Dominio y alcance de Rumbo** (el sistema a construir): actores, conceptos, reglas, estados, fuera de alcance |
| `contexto/proyecto/insumos-arquitectura.md` | Ideas técnicas a evaluar (servicios sugeridos por el profe, mapeo dominio → requisitos, casos para validar) y **lo dicho en la clase de presentación del proyecto**, con minutos |
| `contexto/Transcripcion - Universidad Catolica de Cordoba.txt` | Transcripción automática del audio de la clase de presentación del proyecto (baja confianza en los tramos `[revisar]`). **Solo local** (en `.gitignore`) |
| `contexto/proyecto/` | Consigna, dominio, insumos, equipo y estado del proyecto final |

**Código:** `servicios/<servicio>/` (un módulo Go cada uno, agrupados con `go.work`; Operaciones en hexagonal, el resto en capas). **`servicios/comun/`** tiene lo compartido (observabilidad, mensajería con reintentos y DLQ, identidad del gateway, Problem Details): usarlo, no reimplementarlo. **El contrato interno entre servicios está en `docs/interfaces-internas.md`**. `infra/` (init de MySQL, MongoDB y el MySQL de logs; NGINX; Grafana) y `compose.yaml` en la raíz. La consola web de la Entrega 2 se eliminó (ADR-021): la nueva web (React + Vite + TS) se arma para la Presentación y no expone la capacidad publicada. Puertos: gateway 8080, identidad 8081, operaciones 8082, recursos 8083, notificaciones 8084, integracion 8085.

## Material fuente

Los PDFs de clase (teórico y práctico) **no se guardan en el repo**: su contenido está resumido en `contexto/teoria/` y `contexto/practica/`, que son la fuente a consultar.

- Repo del docente de práctica (`github.com/TomiCassanelli/arquitectura-de-software`): **no se guarda clonado**; todo su contenido está resumido en `contexto/practica/`. Si suben clases nuevas: clonarlo temporalmente, resumir lo nuevo en `contexto/practica/` y borrar el clon.
- Cuando lleguen presentaciones nuevas: leerlas con `pdftotext -enc UTF-8 archivo.pdf -` (poppler está instalado), resumirlas en `contexto/teoria/` o `contexto/practica/`, actualizar este mapa y borrar el PDF.

## Entorno

Windows 11, Go 1.26 (`C:\Program Files\Go`; los módulos declaran `go 1.26.0` porque lo exige OpenTelemetry; con una versión anterior instalada, Go descarga solo la 1.26 gracias a `GOTOOLCHAIN=auto`), Docker Desktop, Node.js 22, Python 3.14, gh CLI. Shell: PowerShell / Git Bash.

**Git:** GitHub Flow (ver README → Forma de trabajo): ramas `feat/` `fix/` `docs/` `adr/`, PR a `main` con CI (`.github/workflows/ci.yml`), commits en español con prefijo de tipo, tags por entrega.

Para probar el sistema: `cp .env.example .env` (una vez) y `docker compose up --build`. Si Docker Desktop está cerrado, hay que iniciarlo primero. Grafana LGTM es opcional (perfil `observabilidad`, ver `.env.example`); para ahorrar memoria se puede levantar un subconjunto (`docker compose up --build <servicio>`).
