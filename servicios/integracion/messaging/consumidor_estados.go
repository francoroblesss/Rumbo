package messaging

import (
	"context"

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
// reintentos con backoff, la DLQ, el ACK después del efecto y la reconexión al broker los
// resuelve comun/mensajeria. Termina cuando se cancela ctx.
func ConsumirEstados(ctx context.Context, conexion *mensajeria.Conexion, manejar mensajeria.Manejador) {
	mensajeria.Consumir(ctx, conexion, ConfigEstados, manejar)
}
