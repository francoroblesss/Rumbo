package main

import (
	"context"
	"log"

	"github.com/gin-gonic/gin"

	"github.com/francoroblesss/Rumbo/servicios/identidad/config"
	"github.com/francoroblesss/Rumbo/servicios/identidad/database"
)

// Identidad y clientes: usuarios, roles, login y tokens; organizaciones cliente; padrón de personas.
// Arquitectura interna en capas: controllers → services → repositories (estilo del práctico).
func main() {
	cfg := config.Cargar()

	// 1. Infraestructura: base propia en MongoDB.
	mongoCliente, err := database.Conectar(context.Background(), cfg.MongoURI)
	if err != nil {
		log.Fatal(err)
	}
	defer mongoCliente.Disconnect(context.Background())

	// 2. Cableado (DI manual): repositories → services → controllers. Pendiente de implementar.

	// 3. Servidor HTTP.
	router := gin.Default()

	log.Printf("Identidad y clientes escuchando en :%s", cfg.Puerto)
	if err := router.Run(":" + cfg.Puerto); err != nil {
		log.Fatal(err)
	}
}
