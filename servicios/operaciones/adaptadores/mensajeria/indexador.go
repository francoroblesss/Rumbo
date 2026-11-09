package mensajeria

import (
	"context"
	"errors"
	"fmt"

	"github.com/francoroblesss/Rumbo/servicios/comun/mensajeria"
	"github.com/francoroblesss/Rumbo/servicios/operaciones/dominio"
	"github.com/francoroblesss/Rumbo/servicios/operaciones/puertos"
)

// ColaIndexador es la cola del indexador de la búsqueda (§16).
const ColaIndexador = "operaciones.indexador"

// ClavesIndexador son los eventos que cambian algo de lo que se indexa (§5.4, §16).
var ClavesIndexador = []string{"operaciones.solicitud.*", "operaciones.traslado.*", "operaciones.cancelacion.*"}

// NuevoManejadorIndexador es el adaptador de entrada del consumidor operaciones.indexador: lee la
// vista del evento y la aplica al índice. La versión es la agregado_version del sobre y la demora
// se mide desde su ocurrido_en. Traduce los errores a la política de comun/mensajeria (ADR-004):
//   - datos ilegibles o incompletos: no se arreglan reintentando → DLQ;
//   - Solr caído: transitorio → reintento con backoff; cuando vuelve, el índice se pone al día.
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
		datos.Solicitud.Version = s.AgregadoVersion
		err := casos.IndexarEvento(ctx, datos.OrganizacionID, datos.Solicitud, s.OcurridoEn)
		if errors.Is(err, dominio.ErrDatosInvalidos) {
			return mensajeria.Permanente(err)
		}
		return err
	}
}
