package mensajeria

import (
	"context"
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/francoroblesss/Rumbo/servicios/comun/mensajeria"
	"github.com/francoroblesss/Rumbo/servicios/operaciones/dominio"
	"github.com/francoroblesss/Rumbo/servicios/operaciones/puertos"
)

// ColaSolicitudesExternas es la cola del comando de Integración (docs/interfaces-internas.md §5.2).
const ColaSolicitudesExternas = "operaciones.solicitudes-externas"

// TipoSolicitudExterna es el tipo del comando (§5.3).
const TipoSolicitudExterna = "integracion.solicitud_carga.recibida"

// datosComando es la forma de "datos" del comando (§5.3).
type datosComando struct {
	SolicitudID       string        `json:"solicitud_id"`
	OrganizacionID    string        `json:"organizacion_id"`
	ReferenciaExterna *string       `json:"referencia_externa"`
	Cosa              cosaDatos     `json:"cosa"`
	Origen            ubicacionDato `json:"origen"`
	Destino           ubicacionDato `json:"destino"`
	HoraLlegada       *time.Time    `json:"hora_llegada_solicitada"`
	Tolerancia        *int          `json:"tolerancia_minutos"`
	Observaciones     *string       `json:"observaciones"`
}

type cosaDatos struct {
	Descripcion string   `json:"descripcion"`
	Cantidad    int      `json:"cantidad"`
	PesoKg      *float64 `json:"peso_kg"`
	VolumenM3   *float64 `json:"volumen_m3"`
}

type ubicacionDato struct {
	Nombre    string  `json:"nombre"`
	Direccion string  `json:"direccion"`
	Contacto  *string `json:"contacto"`
}

// ManejadorSolicitudesExternas traduce el sobre del comando al caso de uso. El ACK lo manda
// comun/mensajeria recién cuando esto termina sin error (ACK después del efecto, ADR-004).
func ManejadorSolicitudesExternas(casos puertos.Solicitudes) mensajeria.Manejador {
	return func(ctx context.Context, s mensajeria.Sobre) error {
		if s.Tipo != TipoSolicitudExterna || s.Version != 1 {
			return mensajeria.Permanente(fmt.Errorf("mensaje no soportado: %s v%d", s.Tipo, s.Version))
		}
		var d datosComando
		if err := s.LeerDatos(&d); err != nil {
			return err
		}
		cmd := puertos.ComandoSolicitudExterna{
			MensajeID:      s.ID,
			SolicitudID:    d.SolicitudID,
			OrganizacionID: d.OrganizacionID,
			Datos: dominio.DatosSolicitud{
				ReferenciaExterna: d.ReferenciaExterna,
				Cosa: dominio.Cosa{Descripcion: d.Cosa.Descripcion, Cantidad: d.Cosa.Cantidad,
					PesoKg: d.Cosa.PesoKg, VolumenM3: d.Cosa.VolumenM3},
				Desde:                 dominio.Ubicacion{Nombre: d.Origen.Nombre, Direccion: d.Origen.Direccion, Contacto: d.Origen.Contacto},
				Hasta:                 dominio.Ubicacion{Nombre: d.Destino.Nombre, Direccion: d.Destino.Direccion, Contacto: d.Destino.Contacto},
				HoraLlegadaSolicitada: d.HoraLlegada,
				Observaciones:         d.Observaciones,
			},
		}
		if d.HoraLlegada != nil {
			hora := d.HoraLlegada.UTC()
			cmd.Datos.HoraLlegadaSolicitada = &hora
		}
		if d.Tolerancia != nil {
			cmd.Datos.ToleranciaMinutos = *d.Tolerancia
		}
		return casos.RegistrarExterna(ctx, cmd)
	}
}

// ConsumirSolicitudesExternas procesa la cola del comando hasta que se cancele ctx. Usa la cola
// con reintentos y DLQ de comun/mensajeria (sin exchange: es un comando punto a punto).
func ConsumirSolicitudesExternas(ctx context.Context, conexion *amqp.Connection, casos puertos.Solicitudes) error {
	return mensajeria.Consumir(ctx, conexion, mensajeria.ConfigConsumidor{Cola: ColaSolicitudesExternas},
		ManejadorSolicitudesExternas(casos))
}
