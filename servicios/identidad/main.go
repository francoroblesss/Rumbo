package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/gin-gonic/gin"

	"github.com/francoroblesss/Rumbo/servicios/comun/observabilidad"
	"github.com/francoroblesss/Rumbo/servicios/identidad/config"
	"github.com/francoroblesss/Rumbo/servicios/identidad/database"
)

// Identidad y clientes: usuarios, roles, login y tokens; organizaciones cliente; padrón de personas.
// Arquitectura interna en capas: controllers → services → repositories (estilo del práctico).
func main() {
	ctx := context.Background()

	// 0. Observabilidad (D11): logs JSON, métricas y trazas con OpenTelemetry.
	apagar, err := observabilidad.Iniciar(ctx, "identidad")
	if err != nil {
		terminar("no se pudo iniciar la observabilidad", err)
	}
	defer apagar(context.Background())

	cfg := config.Cargar()

	// 1. Infraestructura: base propia en MongoDB.
	mongoCliente, err := database.Conectar(ctx, cfg.MongoURI)
	if err != nil {
		terminar("no se pudo conectar a MongoDB", err)
	}
	defer mongoCliente.Disconnect(context.Background())

	// 2. Cableado (DI manual): repositories → services → controllers. Pendiente de implementar.

	// 3. Servidor HTTP instrumentado (traza, correlation id, métrica y log por request).
	router := gin.New()
	router.Use(gin.Recovery())
	observabilidad.Instrumentar(router, "identidad")

	slog.Info("servicio escuchando", slog.String("puerto", cfg.Puerto))
	if err := router.Run(":" + cfg.Puerto); err != nil {
		terminar("el servidor HTTP se detuvo", err)
	}
}

// terminar registra el error y corta el proceso (Docker lo reinicia).
func terminar(mensaje string, err error) {
	slog.Error(mensaje, slog.String("error", err.Error()))
	os.Exit(1)
}
