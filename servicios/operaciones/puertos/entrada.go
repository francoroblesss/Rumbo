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
	// Reservar y Disponibilidad son la capacidad publicada (ADR-021), a través de Integración.
	// Reservar toma un lugar de la franja y crea la solicitud aceptada en una sola transacción;
	// con idem, un reintento devuelve la misma reserva sin tomar otro lugar.
	Reservar(ctx context.Context, org string, d dominio.DatosSolicitud, franjaInicio time.Time, idem *Idempotencia) (Vista, error)
	Disponibilidad(ctx context.Context, fecha string, pesoKg float64) (Disponibilidad, error) // fecha AAAA-MM-DD
	Aceptar(ctx context.Context, org string, id string, extremo *dominio.Extremo) (Vista, error)
	Contraproponer(ctx context.Context, id string, hora time.Time) (Vista, error)
	Rechazar(ctx context.Context, id, motivo string) (Vista, error)
	// ResponderContrapropuesta es del cliente web. Cancelar lo usa Integración (endpoint interno):
	// con idem, la clave de idempotencia se verifica y se guarda en la misma transacción.
	ResponderContrapropuesta(ctx context.Context, org, id string, acepta bool, idem *Idempotencia) (Vista, error)
	Cancelar(ctx context.Context, org, id, motivo string, idem *Idempotencia) (Vista, error)
	ResolverCancelacionTardia(ctx context.Context, id string, aprobar bool) (Vista, error)
	VencerContrapropuestas(ctx context.Context) (int, error)
	Obtener(ctx context.Context, org, id string) (Vista, error) // org "" = personal de Rumbo (ve todas)
	Listar(ctx context.Context, estado string, pagina, tamanio int) (Pagina, error)
}

// Idempotencia identifica un pedido de un endpoint interno: la misma clave con el mismo cuerpo
// devuelve la misma respuesta; con otro cuerpo, ErrClaveReutilizada.
type Idempotencia struct {
	Endpoint string
	Clave    string
	Hash     string // SHA-256 del cuerpo, en hexadecimal
}
