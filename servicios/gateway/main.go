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

	// Contrato público "Solicitar traslado" v2 (ADR-006): token y traslados.
	mux.Handle("/v2/auth/", proxy.Nuevo(cfg.IdentidadURL, ""))
	mux.Handle("/v2/traslados", proxy.Nuevo(cfg.IntegracionURL, ""))
	mux.Handle("/v2/traslados/", proxy.Nuevo(cfg.IntegracionURL, ""))

	// API interna para la aplicación web, versionada: /api/v1/<servicio>/... se reenvía
	// al servicio sin el prefijo (ej. /api/v1/recursos/choferes → /choferes en Recursos).
	mux.Handle("/api/v1/identidad/", proxy.Nuevo(cfg.IdentidadURL, "/api/v1/identidad"))
	mux.Handle("/api/v1/operaciones/", proxy.Nuevo(cfg.OperacionesURL, "/api/v1/operaciones"))
	mux.Handle("/api/v1/recursos/", proxy.Nuevo(cfg.RecursosURL, "/api/v1/recursos"))
	mux.Handle("/api/v1/notificaciones/", proxy.Nuevo(cfg.NotificacionesURL, "/api/v1/notificaciones"))

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
