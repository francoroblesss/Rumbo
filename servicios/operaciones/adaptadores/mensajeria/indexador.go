package mensajeria

import (
	"context"
	"errors"
	"fmt"

	"github.com/francoroblesss/Rumbo/servicios/comun/mensajeria"
	"github.com/francoroblesss/Rumbo/servicios/operaciones/puertos"
)

// ClavesIndexador son los eventos que actualizan el índice de búsqueda (§16): todos llevan la
// vista completa de la solicitud (§5.4).
var ClavesIndexador = []string{"operaciones.solicitud.*", "operaciones.traslado.*", "operaciones.cancelacion.*"}

// NuevoManejadorIndexador es el adaptador de entrada del consumidor operaciones.indexador: lee el
// sobre y le pasa la vista al caso de uso con su versión. Errores (ADR-004):
//   - datos ilegibles o sin solicitud: no se arreglan reintentando → DLQ;
//   - Solr caído: transitorio → reintento con backoff; agotados, DLQ (lo recupera la reindexación).
func NuevoManejadorIndexador(casos puertos.Busqueda) mensajeria.Manejador {
	return func(ctx context.Context, s mensajeria.Sobre) error {
		var datos struct {
			OrganizacionID string        `json:"organizacion_id"`
			Solicitud      puertos.Vista `json:"solicitud"`
		}
		if err := s.LeerDatos(&datos); err != nil {
			return err // ya viene marcado como permanente
		}
		if datos.Solicitud.ID == "" || datos.OrganizacionID == "" {
			return mensajeria.Permanente(fmt.Errorf("%s sin solicitud.id u organizacion_id", s.Tipo))
		}
		// La versión que manda es la del sobre (agregado_version, §5.1): es la que se compara.
		datos.Solicitud.Version = s.AgregadoVersion
		err := casos.IndexarEvento(ctx, datos.OrganizacionID, datos.Solicitud, s.OcurridoEn)
		if err != nil && !errors.Is(err, puertos.ErrBusquedaNoDisponible) {
			return mensajeria.Permanente(err)
		}
		return err
	}
}
