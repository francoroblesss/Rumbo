package config

import (
	"log"
	"os"
)

// Config reúne la configuración del servicio. Se lee de variables de entorno para que
// funcione igual en Docker Compose (host = nombre del servicio) y en local (localhost).
type Config struct {
	Puerto    string // puerto HTTP
	MongoURI  string // base propia del servicio
	RabbitURI string // broker de mensajes
}

// Cargar lee la configuración del entorno. Las URIs con credenciales no tienen valor por
// defecto: los secretos nunca se escriben en el código (consigna §2.12) y salen de .env.
func Cargar() Config {
	return Config{
		Puerto:    valor("PUERTO", "8084"),
		MongoURI:  obligatorio("MONGO_URI"),
		RabbitURI: obligatorio("RABBITMQ_URI"),
	}
}

// valor devuelve la variable de entorno o, si no está definida, el valor por defecto.
func valor(nombre, porDefecto string) string {
	if v := os.Getenv(nombre); v != "" {
		return v
	}
	return porDefecto
}

// obligatorio devuelve la variable de entorno o termina el servicio si no está definida.
func obligatorio(nombre string) string {
	v := os.Getenv(nombre)
	if v == "" {
		log.Fatalf("falta la variable de entorno %s (ver .env.example)", nombre)
	}
	return v
}
