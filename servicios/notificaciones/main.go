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

	"github.com/francoroblesss/Rumbo/servicios/comun/identidad"
	"github.com/francoroblesss/Rumbo/servicios/comun/mensajeria"
	"github.com/francoroblesss/Rumbo/servicios/comun/observabilidad"
	"github.com/francoroblesss/Rumbo/servicios/notificaciones/config"
	"github.com/francoroblesss/Rumbo/servicios/notificaciones/controllers"
	"github.com/francoroblesss/Rumbo/servicios/notificaciones/database"
	"github.com/francoroblesss/Rumbo/servicios/notificaciones/messaging"
	"github.com/francoroblesss/Rumbo/servicios/notificaciones/repositories"
	"github.com/francoroblesss/Rumbo/servicios/notificaciones/services"
)

// Notificaciones: bandeja de avisos por rol y por organización, alimentada por los eventos de
// Operaciones, y stream de avisos en tiempo real (ADR-027).
// Arquitectura interna en capas: controllers → services → repositories.
func main() {
	// El contexto se cancela al recibir la señal de apagado: detiene el consumidor.
	ctx, detener := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer detener()

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

	// Conexión al broker en segundo plano, con reconexión (comun/mensajeria): si RabbitMQ no está,
	// la bandeja atiende igual y los avisos llegan cuando vuelve (ADR-017).
	conexionRabbit := mensajeria.Conectar(ctx, cfg.RabbitURI)
	defer conexionRabbit.Cerrar()

	// 2. Cableado (DI manual): repositorio → servicio → controlador y consumidor.
	repo := repositories.NewAvisosMongo(mongoCliente.Database("notificaciones"))
	if err := repo.AsegurarIndices(ctx); err != nil {
		terminar("no se pudieron crear los índices", err)
	}
	servicio := services.NewAvisosService(repo, time.Now)
	controlador := controllers.NewAvisosController(servicio)

	go messaging.ConsumirAvisos(ctx, conexionRabbit, servicio.ProcesarEvento)

	// 3. Servidor HTTP instrumentado (traza, correlation id, métrica y log por request).
	router := gin.New()
	router.Use(gin.Recovery())
	secreto := identidad.ExigirSecretoInterno()
	// /salud antes de Instrumentar y del secreto: las sondas no se miden ni lo piden (§11.3).
	observabilidad.Salud(router, "notificaciones")
	// El stream también va antes de Instrumentar: una conexión SSE dura minutos y, medida como
	// una request, arruinaría el p95 de rumbo_http_duracion_segundos (ADR-027). En Gin, router.Use
	// solo alcanza a las rutas registradas después, así que esta queda fuera de las métricas. El
	// secreto interno SÍ se le aplica, en su propio grupo: sin él, cualquiera que llegue directo al
	// servicio podría mandar X-Rumbo-Roles: operador y leer los avisos (§11.2).
	controlador.RegistrarStream(router.Group("", secreto))
	observabilidad.Instrumentar(router, "notificaciones")
	// Después de Instrumentar, para que un 401 por falta del secreto se mida y se loguee (§11.2).
	router.Use(secreto)
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
