package messaging

import (
	"context"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/francoroblesss/Rumbo/servicios/comun/mensajeria"
)

// ConfigEstados es la suscripción a los cambios de estado que publica Operaciones
// (interfaces-internas §5.2). Las dos réplicas consumen la misma cola y compiten por los
// mensajes; el control por agregado_version hace que el resultado no dependa de cuál procese.
var ConfigEstados = mensajeria.ConfigConsumidor{
	Cola:     "integracion.estados",
	Exchange: "rumbo.operaciones",
	Claves:   []string{"operaciones.solicitud.*", "operaciones.traslado.*", "operaciones.cancelacion.*"},
}

// ConsumirEstados procesa los eventos con el manejador (SolicitudesService.AplicarEvento). Los
// reintentos con backoff, la DLQ y el ACK después del efecto los resuelve comun/mensajeria.
func ConsumirEstados(ctx context.Context, conexion *amqp.Connection, manejar mensajeria.Manejador) error {
	return mensajeria.Consumir(ctx, conexion, ConfigEstados, manejar)
}
