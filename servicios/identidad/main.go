package main

import (
	"context"
	"log/slog"
	"os"
	"time"
	_ "time/tzdata" // zona horaria America/Argentina/Cordoba disponible en la imagen Alpine

	"github.com/gin-gonic/gin"

	"github.com/francoroblesss/Rumbo/servicios/comun/observabilidad"
	"github.com/francoroblesss/Rumbo/servicios/identidad/config"
	"github.com/francoroblesss/Rumbo/servicios/identidad/controllers"
	"github.com/francoroblesss/Rumbo/servicios/identidad/database"
	"github.com/francoroblesss/Rumbo/servicios/identidad/repositories"
	"github.com/francoroblesss/Rumbo/servicios/identidad/services"
)

// nombreBase es la base propia del servicio en MongoDB (ADR-003).
const nombreBase = "identidad"

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
	base := mongoCliente.Database(nombreBase)

	// 2. Cableado (DI manual): repositories (con sus índices únicos) → service → controllers.
	ctxInicio, cancelar := context.WithTimeout(ctx, 30*time.Second)
	usuarios, err := repositories.NuevoUsuariosMongo(ctxInicio, base)
	if err != nil {
		terminar("no se pudo preparar la colección de usuarios", err)
	}
	organizaciones, err := repositories.NuevoOrganizacionesMongo(ctxInicio, base)
	if err != nil {
		terminar("no se pudo preparar la colección de organizaciones", err)
	}

	autenticacion := services.NuevoAuthService(usuarios, organizaciones, cfg.TokenSecreto, services.Siembra{
		AdminEmail:          cfg.AdminEmail,
		AdminPassword:       cfg.AdminPassword,
		ExternoClientID:     cfg.ExternoClientID,
		ExternoClientSecret: cfg.ExternoClientSecret,
	})
	if err := autenticacion.Sembrar(ctxInicio); err != nil {
		terminar("no se pudo sembrar los datos iniciales", err)
	}
	cancelar()

	// 3. Servidor HTTP instrumentado (traza, correlation id, métrica y log por request).
	router := gin.New()
	router.Use(gin.Recovery())
	observabilidad.Instrumentar(router, "identidad")
	controllers.NuevoAuthController(autenticacion).Registrar(router)

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
