package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/gin-gonic/gin"

	"github.com/francoroblesss/Rumbo/servicios/comun/identidad"
	"github.com/francoroblesss/Rumbo/servicios/comun/mensajeria"
	"github.com/francoroblesss/Rumbo/servicios/comun/observabilidad"
	"github.com/francoroblesss/Rumbo/servicios/recursos/config"
	"github.com/francoroblesss/Rumbo/servicios/recursos/database"
)

// Recursos: choferes, vehículos, actividad, tarifas y preliquidación.
// Arquitectura interna en capas: controllers → services → repositories.
func main() {
	ctx := context.Background()

	// 0. Observabilidad (D11): logs JSON, métricas y trazas con OpenTelemetry.
	apagar, err := observabilidad.Iniciar(ctx, "recursos")
	if err != nil {
		terminar("no se pudo iniciar la observabilidad", err)
	}
	defer apagar(context.Background())

	cfg := config.Cargar()

	// 1. Infraestructura: base propia en MySQL y broker.
	db, err := database.Conectar(ctx, cfg.MySQLDSN)
	if err != nil {
		terminar("no se pudo conectar a MySQL", err)
	}
	defer db.Close()

	// Conexión al broker en segundo plano, con reconexión (comun/mensajeria).
	conexionRabbit := mensajeria.Conectar(ctx, cfg.RabbitURI)
	defer conexionRabbit.Cerrar()

	// 2. Cableado (DI manual): repositories → services → controllers, outbox y consumidor de recorrido.finalizado. Pendiente de implementar.

	// 3. Servidor HTTP instrumentado (traza, correlation id, métrica y log por request).
	router := gin.New()
	router.Use(gin.Recovery())
	// /salud antes de Instrumentar y del secreto: las sondas no se miden ni lo piden (§11.3).
	observabilidad.Salud(router, "recursos")
	observabilidad.Instrumentar(router, "recursos")
	// Después de Instrumentar, para que un 401 por falta del secreto se mida y se loguee (§11.2).
	router.Use(identidad.ExigirSecretoInterno())

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
