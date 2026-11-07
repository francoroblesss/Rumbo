package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	// Base de zonas horarias embebida: la imagen alpine no la trae y las respuestas van en
	// America/Argentina/Cordoba (-03:00).
	_ "time/tzdata"

	"github.com/gin-gonic/gin"

	"github.com/francoroblesss/Rumbo/servicios/comun/identidad"
	"github.com/francoroblesss/Rumbo/servicios/comun/mensajeria"
	"github.com/francoroblesss/Rumbo/servicios/comun/observabilidad"
	"github.com/francoroblesss/Rumbo/servicios/operaciones/adaptadores/api"
	"github.com/francoroblesss/Rumbo/servicios/operaciones/adaptadores/clima"
	adaptadormensajeria "github.com/francoroblesss/Rumbo/servicios/operaciones/adaptadores/mensajeria"
	"github.com/francoroblesss/Rumbo/servicios/operaciones/adaptadores/mysql"
	"github.com/francoroblesss/Rumbo/servicios/operaciones/aplicacion"
	"github.com/francoroblesss/Rumbo/servicios/operaciones/config"
)

// Operaciones: núcleo de Rumbo (solicitudes, planes, traslados, recorridos, asignaciones,
// incidentes). Arquitectura interna hexagonal (ADR-010): dominio y aplicación en el centro,
// puertos como contratos y adaptadores (HTTP, MySQL, RabbitMQ, clima) afuera.
// main es el único lugar que conoce a todos y los conecta.
func main() {
	// ctx se cancela al recibir SIGTERM (docker compose stop): las goroutines terminan.
	ctx, cancelar := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancelar()

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

	// El broker se conecta en segundo plano y se reconecta solo: el servicio atiende HTTP aunque
	// RabbitMQ no esté, y los eventos esperan en el outbox hasta que vuelva (ADR-004).
	conexionRabbit := mensajeria.Conectar(ctx, cfg.RabbitURI)
	defer conexionRabbit.Cerrar()

	// 2. Cableado (DI manual): adaptadores de salida → casos de uso → adaptadores de entrada.
	if err := mysql.Migrar(ctx, cfg.MySQLDSN); err != nil {
		terminar("no se pudieron aplicar las migraciones", err)
	}
	unidadDeTrabajo := mysql.NuevaUnidadDeTrabajo(db)
	consultaClima := clima.NuevoCliente(cfg.ClimaURL, cfg.ClimaAPIKey, cfg.ClimaPrefer)
	calendario := cfg.Calendario
	calendario.Zona = aplicacion.ZonaRumbo()
	solicitudes := aplicacion.NuevasSolicitudes(unidadDeTrabajo, consultaClima, cfg.ClimaICAO, cfg.AeropuertoPalabras,
		calendario, func() time.Time { return time.Now().UTC() }, observabilidad.Instancia())

	router := gin.New()
	router.Use(gin.Recovery())
	// /salud antes de Instrumentar y del secreto: las sondas no se miden ni lo piden (§11.3).
	observabilidad.Salud(router, "operaciones")
	observabilidad.Instrumentar(router, "operaciones")
	// Después de Instrumentar, para que un 401 por falta del secreto se mida y se loguee (§11.2).
	router.Use(identidad.ExigirSecretoInterno())
	api.NuevoHandler(solicitudes).Registrar(router)

	publicador := adaptadormensajeria.NuevoPublicador(conexionRabbit)
	defer publicador.Cerrar()
	relay, err := mysql.NuevoRelay(db, publicador)
	if err != nil {
		terminar("no se pudo preparar el relay del outbox", err)
	}
	go relay.Ejecutar(ctx)

	// Clima después de reservar (ADR-025, §13.2): Operaciones consume su propio evento. La
	// reserva ya respondió; el clima se revisa acá, sin demorarla. Reintentos y DLQ de comun.
	go mensajeria.Consumir(ctx, conexionRabbit, mensajeria.ConfigConsumidor{
		Cola:     "operaciones.clima",
		Exchange: aplicacion.ExchangeEventos,
		Claves:   []string{"operaciones.solicitud.aceptada"},
	}, adaptadormensajeria.NuevoManejadorClima(solicitudes))

	// 3. Servidor HTTP (adaptador de entrada), instrumentado.
	slog.InfoContext(ctx, "servicio escuchando", slog.String("puerto", cfg.Puerto))
	if err := router.Run(":" + cfg.Puerto); err != nil {
		terminar("el servidor HTTP se detuvo", err)
	}
}

// terminar registra el error y corta el proceso (Docker lo reinicia).
func terminar(mensaje string, err error) {
	slog.Error(mensaje, slog.String("error", err.Error()))
	os.Exit(1)
}
