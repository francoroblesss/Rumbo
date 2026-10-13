package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/gin-gonic/gin"

	"github.com/francoroblesss/Rumbo/servicios/comun/mensajeria"
	"github.com/francoroblesss/Rumbo/servicios/comun/observabilidad"
	"github.com/francoroblesss/Rumbo/servicios/notificaciones/config"
	"github.com/francoroblesss/Rumbo/servicios/notificaciones/database"
)

// Notificaciones: bandeja de avisos de cada usuario, alimentada por eventos de RabbitMQ.
// Arquitectura interna en capas: controllers → services → repositories.
func main() {
	ctx := context.Background()

	// 0. Observabilidad (D11): logs JSON, métricas y trazas con OpenTelemetry.
	apagar, err := observabilidad.Iniciar(ctx, "notificaciones")
	if err != nil {
		terminar("no se pudo iniciar la observabilidad", err)
	}
	defer apagar(context.Background())

	cfg := config.Cargar()

	// 1. Infraestructura: base propia en MongoDB y broker.
	mongoCliente, err := database.Conectar(ctx, cfg.MongoURI)
	if err != nil {
		terminar("no se pudo conectar a MongoDB", err)
	}
	defer mongoCliente.Disconnect(context.Background())

	conexionRabbit, err := mensajeria.ConectarConReintentos(cfg.RabbitURI, 5)
	if err != nil {
		terminar("no se pudo conectar a RabbitMQ", err)
	}
	defer conexionRabbit.Close()

	// 2. Cableado (DI manual): repositories → services → controllers y consumidores de eventos. Pendiente de implementar.

	// 3. Servidor HTTP instrumentado (traza, correlation id, métrica y log por request).
	router := gin.New()
	router.Use(gin.Recovery())
	observabilidad.Instrumentar(router, "notificaciones")

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
