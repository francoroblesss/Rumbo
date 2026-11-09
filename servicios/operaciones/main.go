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
	"github.com/francoroblesss/Rumbo/servicios/operaciones/adaptadores/solr"
	"github.com/francoroblesss/Rumbo/servicios/operaciones/aplicacion"
	"github.com/francoroblesss/Rumbo/servicios/operaciones/config"
	"github.com/francoroblesss/Rumbo/servicios/operaciones/puertos"
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
	reloj := func() time.Time { return time.Now().UTC() }
	solicitudes := aplicacion.NuevasSolicitudes(unidadDeTrabajo, consultaClima, cfg.ClimaICAO, cfg.AeropuertoPalabras,
		calendario, reloj, observabilidad.Instancia())

	// Búsqueda de traslados (ADR-011): Solr es opcional. Sin SOLR_URL (Render, ADR-026) solo la
	// búsqueda responde 503. El esquema se aplica en segundo plano: no frena el arranque.
	var buscador puertos.BuscadorTraslados
	if cfg.SolrURL != "" {
		clienteSolr := solr.NuevoCliente(cfg.SolrURL, observabilidad.ClienteHTTP(2*time.Second))
		buscador = clienteSolr
		go asegurarEsquema(ctx, clienteSolr)
	} else {
		slog.InfoContext(ctx, "búsqueda deshabilitada: falta SOLR_URL")
	}
	busqueda := aplicacion.NuevaBusqueda(buscador, unidadDeTrabajo, reloj)

	router := gin.New()
	router.Use(gin.Recovery())
	// /salud antes de Instrumentar y del secreto: las sondas no se miden ni lo piden (§11.3).
	observabilidad.Salud(router, "operaciones")
	observabilidad.Instrumentar(router, "operaciones")
	// Después de Instrumentar, para que un 401 por falta del secreto se mida y se loguee (§11.2).
	router.Use(identidad.ExigirSecretoInterno())
	api.NuevoHandler(solicitudes).Registrar(router)
	api.NuevoHandlerBusqueda(busqueda).Registrar(router) // sin motor responde 503

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

	// Indexador de la búsqueda (ADR-011, §16): el índice es una vista derivada que se actualiza con
	// los eventos que Operaciones ya publica. Si Solr cae, los eventos esperan en la cola
	// (reintentos y DLQ de comun) y la operación sigue (teórico 7).
	if buscador != nil {
		go mensajeria.Consumir(ctx, conexionRabbit, mensajeria.ConfigConsumidor{
			Cola:     adaptadormensajeria.ColaIndexador,
			Exchange: aplicacion.ExchangeEventos,
			Claves:   adaptadormensajeria.ClavesIndexador,
		}, adaptadormensajeria.NuevoManejadorIndexador(busqueda))
	}

	// 3. Servidor HTTP (adaptador de entrada), instrumentado.
	slog.InfoContext(ctx, "servicio escuchando", slog.String("puerto", cfg.Puerto))
	if err := router.Run(":" + cfg.Puerto); err != nil {
		terminar("el servidor HTTP se detuvo", err)
	}
}

// asegurarEsquema aplica el esquema de Solr con reintentos (1 s, 2 s, 4 s… hasta 30 s): Solr
// puede tardar en arrancar y el primer cambio de esquema recarga el core, que puede pasarse del
// timeout del cliente. Mientras tanto, buscar e indexar lo vuelven a intentar por su cuenta.
func asegurarEsquema(ctx context.Context, c *solr.Cliente) {
	espera := time.Second
	for {
		err := c.AsegurarEsquema(ctx)
		if err == nil {
			slog.InfoContext(ctx, "esquema de búsqueda listo")
			return
		}
		slog.WarnContext(ctx, "no se pudo aplicar el esquema de búsqueda, se reintenta",
			slog.String("error", err.Error()), slog.Duration("espera", espera))
		select {
		case <-ctx.Done():
			return
		case <-time.After(espera):
		}
		espera = min(espera*2, 30*time.Second)
	}
}

// terminar registra el error y corta el proceso (Docker lo reinicia).
func terminar(mensaje string, err error) {
	slog.Error(mensaje, slog.String("error", err.Error()))
	os.Exit(1)
}
