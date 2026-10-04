package main

import (
	"context"
	"log"

	"github.com/gin-gonic/gin"

	"github.com/francoroblesss/Rumbo/servicios/operaciones/adaptadores/mensajeria"
	"github.com/francoroblesss/Rumbo/servicios/operaciones/adaptadores/postgres"
	"github.com/francoroblesss/Rumbo/servicios/operaciones/config"
)

// Operaciones: núcleo de Rumbo (solicitudes, planes, traslados, recorridos, asignaciones,
// incidentes). Arquitectura interna hexagonal: dominio y aplicación en el centro, puertos como
// contratos y adaptadores (HTTP, PostgreSQL, RabbitMQ, Recursos, Identidad, clima) afuera.
// main es el único lugar que conoce a todos y los conecta.
func main() {
	cfg := config.Cargar()

	// 1. Infraestructura: base propia en PostgreSQL y broker.
	pool, err := postgres.Conectar(context.Background(), cfg.PostgresURI)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	conexionRabbit, err := mensajeria.ConectarConReintentos(cfg.RabbitURI, 5)
	if err != nil {
		log.Fatal(err)
	}
	defer conexionRabbit.Close()

	// 2. Cableado (DI manual): adaptadores de salida → casos de uso → adaptador HTTP. Pendiente.

	// 3. Servidor HTTP (adaptador de entrada).
	router := gin.Default()

	log.Printf("Operaciones escuchando en :%s", cfg.Puerto)
	if err := router.Run(":" + cfg.Puerto); err != nil {
		log.Fatal(err)
	}
}
