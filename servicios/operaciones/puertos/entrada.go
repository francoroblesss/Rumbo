package puertos

import (
	"context"
	"time"

	"github.com/francoroblesss/Rumbo/servicios/operaciones/dominio"
)

// Solicitudes son los casos de uso que ofrece Operaciones sobre las solicitudes (puerto de
// entrada). Los implementa aplicacion y los invocan los adaptadores de entrada (HTTP y RabbitMQ).
type Solicitudes interface {
	// Reservar y Disponibilidad son la capacidad publicada (ADR-021), a través de Integración.
	// Reservar toma un lugar de la franja y crea la solicitud aceptada en una sola transacción;
	// con idem, un reintento devuelve la misma reserva sin tomar otro lugar.
	Reservar(ctx context.Context, org string, d dominio.DatosSolicitud, franjaInicio time.Time, idem *Idempotencia) (Vista, error)
	Disponibilidad(ctx context.Context, fecha string, pesoKg float64) (Disponibilidad, error) // fecha AAAA-MM-DD
	// Cancelar lo usan Integración (endpoint interno) y la web: con idem, la clave de
	// idempotencia se verifica y se guarda en la misma transacción.
	Cancelar(ctx context.Context, org, id, motivo string, idem *Idempotencia) (Vista, error)
	// CancelarDesdeWeb es el mismo caso de uso para la web (§12): org "" = personal de Rumbo,
	// que cancela cualquier solicitud; la clave se guarda con la organización de la solicitud.
	CancelarDesdeWeb(ctx context.Context, org, id, motivo string, idem *Idempotencia) (Vista, error)
	Obtener(ctx context.Context, org, id string) (Vista, error) // org "" = personal de Rumbo (ve todas)
	Listar(ctx context.Context, f FiltroListado) (Pagina, error)
	// Franjas y AvanzarEstado son de la web del operador (§12, ADR-024).
	Franjas(ctx context.Context, fecha string) (FranjasDelDia, error) // fecha AAAA-MM-DD
	AvanzarEstado(ctx context.Context, org, id string, destino dominio.EstadoTraslado) (Vista, error)
	// RevisarClimaDeReserva es el consumidor de operaciones.solicitud.aceptada (§13.2, ADR-025).
	RevisarClimaDeReserva(ctx context.Context, solicitudID string) error
}

// FiltroListado es el filtro del listado de la web (§12). Organizacion vacía = todas (operador y
// administrador); para el cliente es siempre la suya.
type FiltroListado struct {
	Organizacion   string
	Fecha          string // AAAA-MM-DD, día de inicio de la franja en hora de Argentina; "" = todas
	EstadoTraslado string // "" = todos
	Pagina         int
	Tamanio        int
}

// Idempotencia identifica un pedido de un endpoint: la misma clave con el mismo cuerpo devuelve
// la misma respuesta; con otro cuerpo, ErrClaveReutilizada.
type Idempotencia struct {
	Endpoint string
	Clave    string
	Hash     string // SHA-256 del cuerpo, en hexadecimal
}
