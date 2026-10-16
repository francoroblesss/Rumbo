package mensajeria

import (
	"github.com/francoroblesss/Rumbo/servicios/comun/mensajeria"
	"github.com/francoroblesss/Rumbo/servicios/operaciones/aplicacion"
)

// NuevoPublicador arma el publicador con confirmaciones y registra el exchange de eventos de
// Operaciones (rumbo.operaciones, topic), que se declara en cada canal que se abra. Lo usa el
// relay del outbox. No necesita que el broker esté disponible.
func NuevoPublicador(conexion *mensajeria.Conexion) *mensajeria.Publicador {
	publicador := mensajeria.NuevoPublicador(conexion)
	// Sin conexión no hay error: el exchange se declara al abrir el canal.
	_ = publicador.DeclararExchangeTopic(aplicacion.ExchangeEventos)
	return publicador
}
