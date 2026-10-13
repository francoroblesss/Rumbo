package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/gin-gonic/gin"

	"github.com/francoroblesss/Rumbo/servicios/comun/mensajeria"
	"github.com/francoroblesss/Rumbo/servicios/comun/observabilidad"
	"github.com/francoroblesss/Rumbo/servicios/operaciones/adaptadores/mysql"
	"github.com/francoroblesss/Rumbo/servicios/operaciones/config"
)

// Operaciones: núcleo de Rumbo (solicitudes, planes, traslados, recorridos, asignaciones,
// incidentes). Arquitectura interna hexagonal: dominio y aplicación en el centro, puertos como
// contratos y adaptadores (HTTP, MySQL, RabbitMQ, Recursos, Identidad, clima) afuera.
// main es el único lugar que conoce a todos y los conecta.
func main() {
	ctx := context.Background()

	// 0. Observabilidad (D11): logs JSON, métricas y trazas con OpenTelemetry.
	apagar, err := observabilidad.Iniciar(ctx, "operaciones")
	if err != nil {
		terminar("no se pudo iniciar la observabilidad", err)
	}
	defer apagar(context.Background())

	cfg := config.Cargar()

	// 1. Infraestructura: base propia en MySQL y broker.
	db, err := mysql.Conectar(ctx, cfg.MySQLDSN)
	if err != nil {
		terminar("no se pudo conectar a MySQL", err)
	}
	defer db.Close()

	conexionRabbit, err := mensajeria.ConectarConReintentos(cfg.RabbitURI, 5)
	if err != nil {
		terminar("no se pudo conectar a RabbitMQ", err)
	}
	defer conexionRabbit.Close()

	// 2. Cableado (DI manual): adaptadores de salida → casos de uso → adaptador HTTP. Pendiente.

	// 3. Servidor HTTP (adaptador de entrada), instrumentado.
	router := gin.New()
	router.Use(gin.Recovery())
	observabilidad.Instrumentar(router, "operaciones")

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
