package main

import (
	"log"
	"net/http"

	"github.com/francoroblesss/Rumbo/servicios/gateway/config"
	"github.com/francoroblesss/Rumbo/servicios/gateway/correlacion"
	"github.com/francoroblesss/Rumbo/servicios/gateway/proxy"
)

// API Gateway: punto de entrada único de Rumbo. Rutea cada request al servicio que corresponde,
// asigna el correlation id y (próximamente) valida tokens y limita el tráfico (teórico 6).
func main() {
	cfg := config.Cargar()

	mux := http.NewServeMux()

	// Contrato público (ADR-002): token y solicitudes de carga.
	mux.Handle("/v1/auth/", proxy.Nuevo(cfg.IdentidadURL, ""))
	mux.Handle("/v1/solicitudes-carga", proxy.Nuevo(cfg.IntegracionURL, ""))
	mux.Handle("/v1/solicitudes-carga/", proxy.Nuevo(cfg.IntegracionURL, ""))

	// API interna para la aplicación web: /api/<servicio>/... se reenvía sin el prefijo.
	mux.Handle("/api/identidad/", proxy.Nuevo(cfg.IdentidadURL, "/api/identidad"))
	mux.Handle("/api/operaciones/", proxy.Nuevo(cfg.OperacionesURL, "/api/operaciones"))
	mux.Handle("/api/recursos/", proxy.Nuevo(cfg.RecursosURL, "/api/recursos"))
	mux.Handle("/api/notificaciones/", proxy.Nuevo(cfg.NotificacionesURL, "/api/notificaciones"))

	servidor := &http.Server{
		Addr:              ":" + cfg.Puerto,
		Handler:           correlacion.Middleware(mux),
		ReadHeaderTimeout: proxy.TiempoMaximo,
	}

	log.Printf("API Gateway escuchando en :%s", cfg.Puerto)
	if err := servidor.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
