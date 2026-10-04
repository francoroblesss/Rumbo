# Práctico — Clase 7: Alta disponibilidad, resiliencia y balanceadores de carga (NGINX)

Fuente: repo del docente, `CLASE_7/` (`PRESENTACION/Presentacion.pdf` 11 págs., `DEMO/`, `ACTIVIDADES/1`, `ACTIVIDADES_RESUELTAS/1`).

## Teoría de la presentación
- **Alta disponibilidad:** hacer todo lo posible para que el sistema nunca se caiga, con **redundancia** (si un equipo falla, otro toma el control). Métrica: **uptime**; objetivo clásico "cinco nueves" (99,999 %).
- **Resiliencia:** aceptar que las cosas se rompen y diseñar para **recuperarse rápido** sin que el usuario lo note demasiado; no evita fallos, minimiza su impacto y acelera la recuperación.
- **Escalabilidad / elasticidad** (ej. Hot Sale): **vertical** (más RAM/CPU; límite físico) · **horizontal** (muchas máquinas chicas en equipo; más flexible y económico a escala) · **auto scaling** (ej.: CPU al 70 % → crea 5 copias más; al bajar el tráfico las destruye).
- **Patrones de resiliencia en el código** ("agregar servidores no sirve si el código no maneja errores de red"):
  1. **Retry con backoff exponencial** — esperar 1 s, 2 s, 4 s para no saturar mientras el servicio se recupera.
  2. **Circuit breaker** — como la llave térmica: si un servicio está caído "salta" y corta el tráfico; periódicamente prueba si se recuperó.
  3. **Fallback (plan B)** — alternativa en lugar de error: si las recomendaciones con IA están caídas, mostrar "Los 10 productos más vendidos". "El sistema se degradó, pero no falló."
- **Balanceador de carga** ("director de tránsito"): recibe todas las peticiones y las reparte. **HAProxy** (tráfico puro, velocidad y estabilidad) · **Envoy** (moderno, nube, base de muchos sistemas de microservicios) · **NGINX** (servidor, proxy y balanceador; estándar de la industria; **el del laboratorio**).
- **Algoritmos:** **Round-robin** (cíclico; asume servidores iguales) · **Weighted round-robin** (más peso al servidor potente, ej. 3:1) · **Least connections** (al de menos conexiones activas; ideal si hay tareas mucho más largas) · **Hashing consistente** (anillo: si un servidor se apaga, solo se mueven los usuarios de esa zona).
- **Stateless** (regla de oro de la Clase 1 de DSW): no guardar información del usuario en la RAM local. ❌ Stateful (sesión/carrito en memoria) obliga a **IP Hash** y si el servidor cae el usuario pierde la sesión. ✅ Stateless: "la aplicación nace en la petición y muere en la respuesta"; cualquier servidor libre atiende.
- **Configurando NGINX:**
  - **Health checks:** detectar un servidor Go "muerto" y sacarlo de la rotación; tolerancias, por ejemplo **3 errores** antes de considerarlo muerto y **fuera por 30 segundos**.
  - **Keepalive:** mantener, por ejemplo, **32 conexiones** abiertas entre NGINX y Go para no abrir/cerrar miles por segundo.
  - **502 Bad Gateway:** el servidor Go explotó o está mal configurado; NGINX no pudo conectarse. **504 Gateway Timeout:** NGINX se conectó, pero (p. ej.) la base fue tan lenta que NGINX cortó la llamada.
- **Laboratorio:** crear API Go → empaquetar en Docker → 3 réplicas con Compose → configurar NGINX.
- **Próxima clase — Monitoreo:** **logs** ("diario íntimo" del sistema: qué pasó y a qué hora; investigar incidentes) · **métricas** (números en tiempo real, ej. 85 % de RAM, 5.000 req/s; alertan antes del colapso) · **trazas** (camino exacto de una petición por muchos servidores, midiendo tiempo en cada parada).

## Demo — balanceador simple (`DEMO/`, módulo `balanceador-simple`, go 1.22, Gin v1.10.0)
```
Cliente ──> NGINX ──> Servidor A / Servidor B / Servidor C
```
- `main.go`: `router.GET("/test", handlers.GetTest)`, `router.Run(":3000")`.
- `handlers/servidor_handler.go`: lee env **`NOMBRE_SERVIDOR`** (default "Servidor local") y responde `c.IndentedJSON(200, dto.RespuestaTest{Servidor, Mensaje: "¡Hola Mundo!"})`. `dto/respuesta.go`: `{"servidor","mensaje"}`.
- `Dockerfile`: `FROM golang:1.22-alpine`, `WORKDIR /app`, `COPY . .`, `RUN go build -o api-go .`, `EXPOSE 3000`, `CMD ["./api-go"]` (compila y ejecuta en la misma imagen; "multi-stage se puede ver más adelante").
- `compose.yaml`: `api1/api2/api3` con `build: .` y `NOMBRE_SERVIDOR` "Servidor A/B/C"; `balanceador`: `nginx:alpine`, `ports: "8080:80"`, `./nginx.conf:/etc/nginx/nginx.conf:ro`, `depends_on` las tres.
- `nginx.conf`:
```nginx
events {}
http {
    upstream backends { server api1:3000; server api2:3000; server api3:3000; }
    server { listen 80; location / { proxy_pass http://backends; } }   # round robin por defecto
}
```
- Ejecutar: `docker compose up --build -d`; `for i in {1..9}; do curl -s http://localhost:8080/test; done` (alterna A/B/C; el cliente siempre llama a una sola dirección); `docker compose down`. Sin Docker: `NOMBRE_SERVIDOR="Servidor local" go run .` y `curl http://localhost:3000/test`.
- **No incluye** autoescalado, DNS dinámico ni health checks ("se pueden presentar más adelante").

## Actividad 1 — Migración de una API de clima a tres réplicas (diagnóstico)
- API de clima real (consulta **Open-Meteo**; requiere Internet, Docker Desktop y Compose v2) que se intentó pasar a tres réplicas con NGINX y quedó a medio terminar. **No hay lista de errores**: pueden estar en `main.go`, el service, el handler, Compose o NGINX. No hay que agregar funcionalidad.
- Variante por formulario: bloque `const` de `config/config.go` (Ciudad, Latitud, Longitud, MensajeRespuesta; default Córdoba -31.4167/-64.1833). En el formulario se registra respuesta inicial, diagnóstico y evidencia final.
- Estructura: `handlers/` (Gin/HTTP), `services/` (`OpenMeteoService.ObtenerTemperatura`: `http.Client{Timeout: 5 * time.Second}`, URL `https://api.open-meteo.com/v1/forecast?latitude=..&longitude=..&current=temperature_2m`, errores con `%w`), `dto/` (`RespuestaOpenMeteo{Current{temperature_2m}}` y `RespuestaClima{ciudad, temperatura_c, servidor, mensaje}`), `config/`. Error de Open-Meteo → **502** `{"error": "no se pudo consultar la temperatura"}`. Ruta `GET /clima`.

### Errores sembrados (diff `ACTIVIDADES/1` → `ACTIVIDADES_RESUELTAS/1`)
| Dónde | Error | Corrección |
| --- | --- | --- |
| `nginx.conf` | `upstream api_clima` solo con `api1`; `proxy_pass http://api1:3000` (salta el upstream) | `upstream` con `api1`, `api2`, `api3` y `proxy_pass http://api_clima` |
| `compose.yaml` | Solo `api1` y `api2`; ambas con `ports` (`3001:3000`, `3002:3000`) publicados al host; `api2` sin `NOMBRE_SERVIDOR` | Tres APIs con `NOMBRE_SERVIDOR` "Clima A/B/C" y **`expose: "3000"`** (internos); solo NGINX publica `8080:80`; `depends_on` incluye `api3` |
| `handlers/clima_handler.go` | `nombreServidor := "Clima A"` hardcodeado | `os.Getenv("NOMBRE_SERVIDOR")` con default "Servidor local" |
| `services/open_meteo_service.go` | `if response.StatusCode != http.StatusCreated` (Open-Meteo devuelve 200 → siempre error) | `http.StatusOK` |

- README de la solución: "los dos errores eran" upstream solo con `api1` y backends con `ports` publicados ("los backends deben permanecer internos"). `expose` documenta el puerto interno. Verificación: `for i in {1..9}; do curl -s http://localhost:8080/clima; done` alterna Clima A/B/C; `curl -i http://localhost:3002/clima` **no conecta** (api2 sin puerto publicado).
- El README general de resueltas lista también una "Actividad 2 — Recuperación ante fallo (reintento configurado)" y rutas `actividad-1_balanceador_roto/`, `actividad-2_recuperacion_ante_fallo/` que **no existen en el repo** (solo está la carpeta `1`).
