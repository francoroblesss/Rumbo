package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	_ "time/tzdata" // la imagen Alpine no trae la base de zonas horarias (America/Argentina/Cordoba)

	"github.com/francoroblesss/Rumbo/servicios/comun/mensajeria"
	"github.com/francoroblesss/Rumbo/servicios/comun/observabilidad"
	"github.com/francoroblesss/Rumbo/servicios/integracion/clients"
	"github.com/francoroblesss/Rumbo/servicios/integracion/config"
	"github.com/francoroblesss/Rumbo/servicios/integracion/controllers"
	"github.com/francoroblesss/Rumbo/servicios/integracion/database"
	"github.com/francoroblesss/Rumbo/servicios/integracion/messaging"
	"github.com/francoroblesss/Rumbo/servicios/integracion/repositories"
	"github.com/francoroblesss/Rumbo/servicios/integracion/services"
)

// Integración de carga: atiende la capacidad publicada "Reservar traslado" (ADR-021) y la
// traduce al modelo interno de Operaciones (ADR-001).
// Arquitectura interna en capas: controllers → services → repositories / clients.
func main() {
	// El contexto se cancela al recibir la señal de apagado: detiene el consumidor.
	ctx, detener := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer detener()

	// 0. Observabilidad (D11): logs JSON, métricas y trazas con OpenTelemetry.
	apagar, err := observabilidad.Iniciar(ctx, "integracion")
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

	// El broker se conecta en segundo plano y se reconecta solo: el servicio atiende HTTP aunque
	// RabbitMQ no esté; los cambios de estado llegan cuando vuelve (ADR-017).
	conexionRabbit := mensajeria.Conectar(ctx, cfg.RabbitURI)
	defer conexionRabbit.Cerrar()

	// 2. Cableado (DI manual): repositorio → caché → cliente → servicio → controlador y consumidor.
	repoMongo := repositories.NewSolicitudesMongo(mongoCliente.Database("integracion"))
	if err := repoMongo.AsegurarIndices(ctx); err != nil {
		terminar("no se pudieron crear los índices", err)
	}
	// Decorador cache-aside con Memcached (ADR-007): compartido por las dos réplicas.
	repo := repositories.NewSolicitudesCache(repoMongo, repositories.NewMemcached(cfg.MemcachedAddr))
	// Cliente hacia Operaciones con timeout de 2 s y circuit breaker (ADR-008).
	operaciones := clients.NewOperacionesClient(cfg.OperacionesURL)
	servicio := services.NewSolicitudesService(repo, operaciones, time.Now)
	controlador := controllers.NewTrasladosController(servicio)

	go messaging.ConsumirEstados(ctx, conexionRabbit, servicio.AplicarEvento)

	// 3. Servidor HTTP instrumentado (traza, correlation id, métrica y log por request).
	router := gin.New()
	router.Use(gin.Recovery())
	observabilidad.Instrumentar(router, "integracion")
	controlador.Registrar(router)

	servidor := &http.Server{Addr: ":" + cfg.Puerto, Handler: router}
	go func() {
		<-ctx.Done()
		cierre, cancelar := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelar()
		_ = servidor.Shutdown(cierre)
	}()

	slog.Info("servicio escuchando", slog.String("puerto", cfg.Puerto))
	if err := servidor.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		terminar("el servidor HTTP se detuvo", err)
	}
}

// terminar registra el error y corta el proceso (Docker lo reinicia).
func terminar(mensaje string, err error) {
	slog.Error(mensaje, slog.String("error", err.Error()))
	os.Exit(1)
}
