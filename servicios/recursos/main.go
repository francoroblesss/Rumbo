package main

import (
	"context"
	"log"

	"github.com/gin-gonic/gin"

	"github.com/francoroblesss/Rumbo/servicios/recursos/config"
	"github.com/francoroblesss/Rumbo/servicios/recursos/database"
	"github.com/francoroblesss/Rumbo/servicios/recursos/messaging"
)

// Recursos: choferes, vehículos, actividad, tarifas y preliquidación.
// Arquitectura interna en capas: controllers → services → repositories.
func main() {
	cfg := config.Cargar()

	// 1. Infraestructura: base propia en MySQL y broker.
	db, err := database.Conectar(context.Background(), cfg.MySQLDSN)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	conexionRabbit, err := messaging.ConectarConReintentos(cfg.RabbitURI, 5)
	if err != nil {
		log.Fatal(err)
	}
	defer conexionRabbit.Close()

	// 2. Cableado (DI manual), outbox y consumidor de recorrido.finalizado. Pendiente de implementar.

	// 3. Servidor HTTP.
	router := gin.Default()

	log.Printf("Recursos escuchando en :%s", cfg.Puerto)
	if err := router.Run(":" + cfg.Puerto); err != nil {
		log.Fatal(err)
	}
}
