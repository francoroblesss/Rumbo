package puertos

import (
	"context"
	"time"

	"github.com/francoroblesss/Rumbo/servicios/operaciones/dominio"
)

// Solicitudes son los casos de uso que ofrece Operaciones sobre las solicitudes (puerto de
// entrada). Los implementa aplicacion y los invocan los adaptadores de entrada (HTTP, RabbitMQ y
// la tarea programada).
type Solicitudes interface {
	RegistrarExterna(ctx context.Context, cmd ComandoSolicitudExterna) error // idempotente
	Aceptar(ctx context.Context, org string, id string, extremo *dominio.Extremo) (Vista, error)
	Contraproponer(ctx context.Context, id string, hora time.Time) (Vista, error)
	Rechazar(ctx context.Context, id, motivo string) (Vista, error)
	// ResponderContrapropuesta y Cancelar los usa Integración (endpoints internos): con idem, la
	// clave de idempotencia se verifica y se guarda en la misma transacción que el cambio.
	ResponderContrapropuesta(ctx context.Context, org, id string, acepta bool, idem *Idempotencia) (Vista, error)
	Cancelar(ctx context.Context, org, id, motivo string, idem *Idempotencia) (Vista, error)
	ResolverCancelacionTardia(ctx context.Context, id string, aprobar bool) (Vista, error)
	VencerContrapropuestas(ctx context.Context) (int, error)
	Obtener(ctx context.Context, org, id string) (Vista, error) // org "" = personal de Rumbo (ve todas)
	Listar(ctx context.Context, estado string, pagina, tamanio int) (Pagina, error)
}

// ComandoSolicitudExterna es el comando integracion.solicitud_carga.recibida ya traducido
// (docs/interfaces-internas.md §5.3).
type ComandoSolicitudExterna struct {
	MensajeID      string // id del sobre: es la marca de idempotencia del consumidor
	SolicitudID    string
	OrganizacionID string
	Datos          dominio.DatosSolicitud
}

// Idempotencia identifica un pedido de un endpoint interno: la misma clave con el mismo cuerpo
// devuelve la misma respuesta; con otro cuerpo, ErrClaveReutilizada.
type Idempotencia struct {
	Endpoint string
	Clave    string
	Hash     string // SHA-256 del cuerpo, en hexadecimal
}
