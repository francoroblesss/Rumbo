package main

import (
	"context"
	"log"

	"github.com/gin-gonic/gin"

	"github.com/francoroblesss/Rumbo/servicios/integracion/config"
	"github.com/francoroblesss/Rumbo/servicios/integracion/database"
	"github.com/francoroblesss/Rumbo/servicios/integracion/messaging"
)

// Integración de carga: atiende la capacidad publicada "Solicitar traslado" (contrato v2)
// y la traduce al modelo interno de Operaciones (ADR-001, ADR-002).
// Arquitectura interna en capas: controllers → services → repositories / clients.
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

	// 2. Cableado (DI manual): repositories, clients → services → controllers. Pendiente de implementar.

	// 3. Servidor HTTP.
	router := gin.Default()

	log.Printf("Integración de carga escuchando en :%s", cfg.Puerto)
	if err := router.Run(":" + cfg.Puerto); err != nil {
		log.Fatal(err)
	}
}
