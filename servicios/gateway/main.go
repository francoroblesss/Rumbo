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

	// Contrato público "Solicitar traslado" (ADR-006), sin versión en la ruta (ADR-019): token y traslados.
	mux.Handle("/auth/", proxy.Nuevo(cfg.IdentidadURL, ""))
	mux.Handle("/traslados", proxy.Nuevo(cfg.IntegracionURL, ""))
	mux.Handle("/traslados/", proxy.Nuevo(cfg.IntegracionURL, ""))

	// API interna para la aplicación web, versionada: /api/v1/<servicio>/... se reenvía
	// al servicio sin el prefijo (ej. /api/v1/recursos/choferes → /choferes en Recursos).
	mux.Handle("/api/v1/identidad/", proxy.Nuevo(cfg.IdentidadURL, "/api/v1/identidad"))
	mux.Handle("/api/v1/operaciones/", proxy.Nuevo(cfg.OperacionesURL, "/api/v1/operaciones"))
	mux.Handle("/api/v1/recursos/", proxy.Nuevo(cfg.RecursosURL, "/api/v1/recursos"))
	mux.Handle("/api/v1/notificaciones/", proxy.Nuevo(cfg.NotificacionesURL, "/api/v1/notificaciones"))

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
