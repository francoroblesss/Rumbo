package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"github.com/francoroblesss/Rumbo/servicios/comun/observabilidad"
	"github.com/francoroblesss/Rumbo/servicios/comun/problema"
	"github.com/francoroblesss/Rumbo/servicios/gateway/autenticacion"
	"github.com/francoroblesss/Rumbo/servicios/gateway/config"
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
// observabilidad → bloqueo de rutas internas → limpieza de identidad → autenticación →
// límite de tráfico → proxy al servicio.
func armar(cfg config.Config, limitador *limite.Limitador) http.Handler {
	mux := http.NewServeMux()

	// Contrato público "Reservar traslado" (ADR-021), sin versión en la ruta (ADR-019): token,
	// disponibilidad y traslados.
	mux.Handle("/auth/", proxy.Nuevo(cfg.IdentidadURL, ""))
	mux.Handle("/disponibilidad", proxy.Nuevo(cfg.IntegracionURL, ""))
	mux.Handle("/traslados", proxy.Nuevo(cfg.IntegracionURL, ""))
	mux.Handle("/traslados/", proxy.Nuevo(cfg.IntegracionURL, ""))

	// API interna para la aplicación web: /api/<servicio>/... se reenvía al servicio sin el
	// prefijo (ej. /api/recursos/choferes → /choferes en Recursos). Sin versión en la ruta (ADR-019).
	mux.Handle("/api/identidad/", proxy.Nuevo(cfg.IdentidadURL, "/api/identidad"))
	mux.Handle("/api/operaciones/", proxy.Nuevo(cfg.OperacionesURL, "/api/operaciones"))
	mux.Handle("/api/recursos/", proxy.Nuevo(cfg.RecursosURL, "/api/recursos"))
	mux.Handle("/api/notificaciones/", proxy.Nuevo(cfg.NotificacionesURL, "/api/notificaciones"))

	// Cualquier otra ruta: 404 con el mismo formato de error que el resto.
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		problema.EscribirHTTP(w, http.StatusNotFound, "NO_ENCONTRADA", "No encontrada", "La ruta no existe.")
	})

	var h http.Handler = mux
	h = limite.Middleware(limitador)(h)
	h = autenticacion.Middleware(cfg.TokenSecreto)(h)
	h = entrada.LimpiarIdentidad(h)
	h = entrada.BloquearInternas(h)
	return observabilidad.InstrumentarHTTP(h, "gateway")
}

// terminar registra el error y corta el proceso (Docker lo reinicia).
func terminar(mensaje string, err error) {
	slog.Error(mensaje, slog.String("error", err.Error()))
	os.Exit(1)
}
