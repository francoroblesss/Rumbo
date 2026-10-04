package main

import (
	"context"
	"log"

	"github.com/gin-gonic/gin"

	"github.com/francoroblesss/Rumbo/servicios/notificaciones/config"
	"github.com/francoroblesss/Rumbo/servicios/notificaciones/database"
	"github.com/francoroblesss/Rumbo/servicios/notificaciones/messaging"
)

// Notificaciones: bandeja de avisos de cada usuario, alimentada por eventos de RabbitMQ.
// Arquitectura interna en capas: controllers → services → repositories.
func main() {
	cfg := config.Cargar()

	// 1. Infraestructura: base propia en MongoDB y broker.
	mongoCliente, err := database.Conectar(context.Background(), cfg.MongoURI)
	if err != nil {
		log.Fatal(err)
	}
	defer mongoCliente.Disconnect(context.Background())

	conexionRabbit, err := messaging.ConectarConReintentos(cfg.RabbitURI, 5)
	if err != nil {
		log.Fatal(err)
	}
	defer conexionRabbit.Close()

	// 2. Cableado (DI manual) y consumidores de eventos. Pendiente de implementar.

	// 3. Servidor HTTP.
	router := gin.Default()

	log.Printf("Notificaciones escuchando en :%s", cfg.Puerto)
	if err := router.Run(":" + cfg.Puerto); err != nil {
		log.Fatal(err)
	}
}
