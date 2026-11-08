package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/rs/cors"

	"github.com/francoroblesss/Rumbo/servicios/comun/observabilidad"
	"github.com/francoroblesss/Rumbo/servicios/comun/problema"
	"github.com/francoroblesss/Rumbo/servicios/gateway/autenticacion"
	"github.com/francoroblesss/Rumbo/servicios/gateway/config"
	"github.com/francoroblesss/Rumbo/servicios/gateway/despertar"
	"github.com/francoroblesss/Rumbo/servicios/gateway/entrada"
	"github.com/francoroblesss/Rumbo/servicios/gateway/limite"
	"github.com/francoroblesss/Rumbo/servicios/gateway/proxy"
)

// API Gateway: punto de entrada único de Rumbo. Rutea cada request al servicio que corresponde,
// asigna el correlation id, valida tokens, propaga la identidad y limita el tráfico (teórico 6).
func main() {
	ctx := context.Background()

	apagar, err := observabilidad.Iniciar(ctx, "gateway")
	if err != nil {
		terminar("no se pudo iniciar la observabilidad", err)
	}
	defer apagar(context.Background())

	cfg := config.Cargar()

	limitador := limite.Nuevo()
	ctxLimpieza, cancelar := context.WithCancel(ctx)
	defer cancelar()
	go limitador.Limpiar(ctxLimpieza)

	// En Render (ADR-026) despierta en paralelo el camino de la capacidad, en segundo plano.
	if cfg.Despertar {
		go despertar.Cadena(ctx, observabilidad.ClienteInterno(3*time.Second),
			[]string{cfg.IdentidadURL, cfg.OperacionesURL, cfg.IntegracionURL}, despertar.Intervalo, despertar.Plazo)
	}

	servidor := &http.Server{
		Addr:              ":" + cfg.Puerto,
		Handler:           armar(cfg, limitador),
		ReadHeaderTimeout: proxy.TiempoMaximo,
	}

	slog.Info("API Gateway escuchando", slog.String("puerto", cfg.Puerto))
	if err := servidor.ListenAndServe(); err != nil {
		terminar("el servidor HTTP se detuvo", err)
	}
}

// armar arma la cadena de middlewares, de afuera hacia adentro:
// salud → observabilidad → CORS → bloqueo de rutas internas → limpieza de identidad →
// autenticación → límite de tráfico → proxy al servicio (con el secreto interno).
func armar(cfg config.Config, limitador *limite.Limitador) http.Handler {
	mux := http.NewServeMux()
	s := cfg.SecretoInterno

	// Contrato público "Reservar traslado" (ADR-021), sin versión en la ruta (ADR-019): token,
	// disponibilidad y traslados.
	mux.Handle("/auth/", proxy.Nuevo(cfg.IdentidadURL, "", s))
	mux.Handle("/disponibilidad", proxy.Nuevo(cfg.IntegracionURL, "", s))
	mux.Handle("/traslados", proxy.Nuevo(cfg.IntegracionURL, "", s))
	mux.Handle("/traslados/", proxy.Nuevo(cfg.IntegracionURL, "", s))

	// API interna para la aplicación web: /api/<servicio>/... se reenvía al servicio sin el
	// prefijo (ej. /api/recursos/choferes → /choferes en Recursos). Sin versión en la ruta (ADR-019).
	mux.Handle("/api/identidad/", proxy.Nuevo(cfg.IdentidadURL, "/api/identidad", s))
	mux.Handle("/api/operaciones/", proxy.Nuevo(cfg.OperacionesURL, "/api/operaciones", s))
	mux.Handle("/api/recursos/", proxy.Nuevo(cfg.RecursosURL, "/api/recursos", s))
	mux.Handle("/api/notificaciones/", proxy.Nuevo(cfg.NotificacionesURL, "/api/notificaciones", s))
	// Stream de avisos (SSE, ADR-027): la única ruta sin el deadline de 5 s. La autenticación y
	// el límite de tráfico se aplican igual, al abrirlo (docs/interfaces-internas.md §11.4).
	mux.Handle("GET /api/notificaciones/avisos/stream",
		proxy.NuevoStream(cfg.NotificacionesURL, "/api/notificaciones", s))

	// Cualquier otra ruta: 404 con el mismo formato de error que el resto.
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		problema.EscribirHTTP(w, http.StatusNotFound, "NO_ENCONTRADA", "No encontrada", "La ruta no existe.")
	})

	var h http.Handler = mux
	h = limite.Middleware(limitador)(h)
	h = autenticacion.Middleware(cfg.TokenSecreto)(h)
	h = entrada.LimpiarIdentidad(h)
	h = entrada.BloquearInternas(h)
	// CORS antes de la autenticación: el preflight (OPTIONS) no lleva token. CORS no autentica
	// (teórico 6): solo le dice al navegador qué origen puede leer las respuestas.
	h = cors.New(cors.Options{
		AllowedOrigins:   []string{cfg.WebOrigen},
		AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete},
		AllowedHeaders:   []string{"Authorization", "Content-Type", "Idempotency-Key", "Last-Event-ID"},
		ExposedHeaders:   []string{observabilidad.HeaderCorrelacion},
		AllowCredentials: false, // el token va en Authorization, no en una cookie
	}).Handler(h)
	return salud(observabilidad.InstrumentarHTTP(h, "gateway"))
}

// salud responde GET y HEAD /salud sin token, sin límite de tráfico y sin medirlo (§11.3): lo
// usan los healthchecks de Compose y Render, y los monitores externos que usan HEAD (ADR-026).
// Solo dice que el proceso está vivo.
func salud(siguiente http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if (r.Method == http.MethodGet || r.Method == http.MethodHead) && r.URL.Path == "/salud" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"estado":"ok","servicio":"gateway"}`))
			return
		}
		siguiente.ServeHTTP(w, r)
	})
}

// terminar registra el error y corta el proceso (Docker lo reinicia).
func terminar(mensaje string, err error) {
	slog.Error(mensaje, slog.String("error", err.Error()))
	os.Exit(1)
}
