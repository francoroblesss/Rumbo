package messaging

import (
	"fmt"
	"log"
	"math/rand/v2"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// ConectarConReintentos se conecta a RabbitMQ reintentando con backoff exponencial
// y jitter (teórico 7): 1 s, 2 s, 4 s... más una parte aleatoria, para que varias
// réplicas que arrancan juntas no golpeen al broker al mismo tiempo.
func ConectarConReintentos(uri string, intentos int) (*amqp.Connection, error) {
	for intento := 1; intento <= intentos; intento++ {
		conexion, err := amqp.Dial(uri)
		if err == nil {
			return conexion, nil
		}
		if intento == intentos {
			return nil, fmt.Errorf("RabbitMQ no respondió después de %d intentos: %w", intentos, err)
		}

		espera := time.Second * time.Duration(1<<(intento-1))
		espera += time.Duration(rand.Int64N(int64(espera / 2)))
		log.Printf("[mensajeria] RabbitMQ no responde (intento %d/%d). Reintento en %v", intento, intentos, espera)
		time.Sleep(espera)
	}
	return nil, fmt.Errorf("no se pudo conectar a RabbitMQ")
}
