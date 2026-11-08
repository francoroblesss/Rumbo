package messaging

import (
	"context"

	"github.com/francoroblesss/Rumbo/servicios/comun/mensajeria"
	"github.com/francoroblesss/Rumbo/servicios/notificaciones/models"
)

// ConfigAvisos es la suscripción a los eventos de Operaciones que generan avisos
// (docs/interfaces-internas.md §14). Es una cola propia (pub/sub, teórico 4): Operaciones no
// sabe que Notificaciones existe.
var ConfigAvisos = mensajeria.ConfigConsumidor{
	Cola:     "notificaciones.avisos",
	Exchange: "rumbo.operaciones",
	Claves: []string{models.EventoSolicitudAceptada, models.EventoCancelacionRegistrada,
		models.EventoEstadoCambiado, models.EventoAlertaClimatica},
}

// ConsumirAvisos procesa los eventos con el manejador (AvisosService.ProcesarEvento). Los
// reintentos con backoff, la DLQ notificaciones.avisos.dlq, el ACK después del efecto y la
// reconexión al broker los resuelve comun/mensajeria. Termina cuando se cancela ctx.
func ConsumirAvisos(ctx context.Context, conexion *mensajeria.Conexion, manejar mensajeria.Manejador) {
	mensajeria.Consumir(ctx, conexion, ConfigAvisos, manejar)
}
