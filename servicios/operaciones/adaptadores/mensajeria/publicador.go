package mensajeria

import (
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/francoroblesss/Rumbo/servicios/comun/mensajeria"
	"github.com/francoroblesss/Rumbo/servicios/operaciones/aplicacion"
)

// NuevoPublicador abre el publicador con confirmaciones y declara el exchange de eventos de
// Operaciones (rumbo.operaciones, topic). Lo usa el relay del outbox.
func NuevoPublicador(conexion *amqp.Connection) (*mensajeria.Publicador, error) {
	publicador, err := mensajeria.NuevoPublicador(conexion)
	if err != nil {
		return nil, err
	}
	if err := publicador.DeclararExchangeTopic(aplicacion.ExchangeEventos); err != nil {
		publicador.Cerrar()
		return nil, fmt.Errorf("declarar exchange %s: %w", aplicacion.ExchangeEventos, err)
	}
	return publicador, nil
}
