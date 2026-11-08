package mensajeria

import (
	"context"
	"errors"
	"fmt"

	"github.com/francoroblesss/Rumbo/servicios/comun/mensajeria"
	"github.com/francoroblesss/Rumbo/servicios/operaciones/puertos"
)

// NuevoManejadorClima es el adaptador de entrada del consumidor operaciones.clima (§13.2): lee el
// sobre de operaciones.solicitud.aceptada y llama al caso de uso. Traduce los errores a la
// política de comun/mensajeria (ADR-004):
//   - datos ilegibles o una solicitud que no existe: no se arreglan reintentando → DLQ;
//   - error de MySQL: transitorio → reintento con backoff;
//   - clima caído: el caso de uso no lo devuelve como error (fail-open) → ACK.
func NuevoManejadorClima(casos puertos.Solicitudes) mensajeria.Manejador {
	return func(ctx context.Context, s mensajeria.Sobre) error {
		var datos struct {
			Solicitud struct {
				ID string `json:"id"`
			} `json:"solicitud"`
		}
		if err := s.LeerDatos(&datos); err != nil {
			return err // ya viene marcado como permanente
		}
		if datos.Solicitud.ID == "" {
			return mensajeria.Permanente(fmt.Errorf("%s sin solicitud.id", s.Tipo))
		}
		err := casos.RevisarClimaDeReserva(ctx, datos.Solicitud.ID)
		if errors.Is(err, puertos.ErrNoEncontrada) {
			return mensajeria.Permanente(err)
		}
		return err
	}
}
