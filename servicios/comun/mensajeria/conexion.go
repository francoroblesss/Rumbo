package mensajeria

import (
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel/propagation"
)

// ConectarConReintentos se conecta a RabbitMQ reintentando con backoff exponencial y jitter
// (teórico 7): 1 s, 2 s, 4 s... más una parte aleatoria, para que varias réplicas que arrancan
// juntas no golpeen al broker al mismo tiempo.
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
		slog.Warn("RabbitMQ no responde, se reintenta",
			slog.Int("intento", intento), slog.Int("intentos", intentos), slog.Duration("espera", espera))
		time.Sleep(espera)
	}
	return nil, fmt.Errorf("no se pudo conectar a RabbitMQ")
}

// encabezados adapta los headers AMQP al propagador de OpenTelemetry, para que la traza viaje
// dentro del mensaje (traceparent / tracestate).
type encabezados amqp.Table

var _ propagation.TextMapCarrier = encabezados{}

func (e encabezados) Get(clave string) string {
	if v, ok := e[clave].(string); ok {
		return v
	}
	return ""
}

func (e encabezados) Set(clave, valor string) { e[clave] = valor }

func (e encabezados) Keys() []string {
	claves := make([]string, 0, len(e))
	for k := range e {
		claves = append(claves, k)
	}
	return claves
}
