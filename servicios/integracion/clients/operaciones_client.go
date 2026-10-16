package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/sony/gobreaker/v2"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/francoroblesss/Rumbo/servicios/comun/identidad"
	"github.com/francoroblesss/Rumbo/servicios/comun/observabilidad"
	"github.com/francoroblesss/Rumbo/servicios/integracion/models"
)

// Errores del cliente: el servicio los traduce a los códigos del contrato público.
var (
	ErrNoEncontrada   = errors.New("Operaciones no tiene la solicitud")
	ErrDatosInvalidos = errors.New("Operaciones rechazó los datos")
	ErrNoDisponible   = errors.New("Operaciones no está disponible")
)

// ErrConflicto es un 409 de Operaciones, con su código (ESTADO_NO_PERMITE_OPERACION,
// CONTRAPROPUESTA_VENCIDA…).
type ErrConflicto struct{ Codigo string }

func (e ErrConflicto) Error() string { return "Operaciones respondió conflicto: " + e.Codigo }

// TimeoutOperaciones es el timeout de cada llamada (interfaces-internas §4.1, ADR-004).
const TimeoutOperaciones = 2 * time.Second

// errFallaDependencia marca una respuesta 5xx para que el breaker la cuente como falla.
var errFallaDependencia = errors.New("Operaciones respondió con error de servidor")

// OperacionesClient llama a los endpoints internos de Operaciones (interfaces-internas §4.1).
type OperacionesClient interface {
	ResponderContrapropuesta(ctx context.Context, org, claveIdem, id string, acepta bool) (models.VistaSolicitud, error)
	Cancelar(ctx context.Context, org, claveIdem, id, motivo string) (models.VistaSolicitud, error)
}

type operacionesHTTP struct {
	baseURL string
	http    *http.Client
	breaker *gobreaker.CircuitBreaker[*http.Response]
}

// NewOperacionesClient crea el cliente con timeout de 2 s y circuit breaker (ADR-008). No
// reintenta: el reintento es del consumidor, con su Idempotency-Key (un solo dueño del reintento).
func NewOperacionesClient(baseURL string) OperacionesClient {
	estado, _ := observabilidad.Medidor().Int64Gauge("rumbo_circuito_estado",
		metric.WithDescription("Estado del circuit breaker: 0 cerrado, 1 semiabierto, 2 abierto"))
	atributos := metric.WithAttributes(attribute.String("dependencia", "operaciones"))
	estado.Record(context.Background(), int64(gobreaker.StateClosed), atributos)

	breaker := gobreaker.NewCircuitBreaker[*http.Response](gobreaker.Settings{
		Name:        "operaciones",
		MaxRequests: 1,                // en semiabierto deja pasar una llamada de prueba
		Interval:    60 * time.Second, // ventana en la que se cuentan las fallas
		Timeout:     30 * time.Second, // tiempo abierto antes de pasar a semiabierto
		ReadyToTrip: func(c gobreaker.Counts) bool {
			return c.ConsecutiveFailures >= 5 || (c.Requests >= 10 && c.TotalFailures*2 >= c.Requests)
		},
		OnStateChange: func(nombre string, desde, hacia gobreaker.State) {
			// Los valores de gobreaker.State coinciden con los del gauge: 0, 1 y 2.
			estado.Record(context.Background(), int64(hacia), atributos)
			slog.Warn("cambió el estado del circuit breaker", slog.String("dependencia", nombre),
				slog.String("desde", desde.String()), slog.String("hacia", hacia.String()))
		},
	})
	return &operacionesHTTP{baseURL: baseURL, http: observabilidad.ClienteHTTP(TimeoutOperaciones), breaker: breaker}
}

func (c *operacionesHTTP) ResponderContrapropuesta(ctx context.Context, org, claveIdem, id string, acepta bool) (models.VistaSolicitud, error) {
	return c.llamar(ctx, org, claveIdem, "/internas/solicitudes/"+url.PathEscape(id)+"/respuesta-contrapropuesta",
		map[string]bool{"acepta": acepta})
}

func (c *operacionesHTTP) Cancelar(ctx context.Context, org, claveIdem, id, motivo string) (models.VistaSolicitud, error) {
	return c.llamar(ctx, org, claveIdem, "/internas/solicitudes/"+url.PathEscape(id)+"/cancelacion",
		map[string]string{"motivo": motivo})
}

func (c *operacionesHTTP) llamar(ctx context.Context, org, claveIdem, ruta string, cuerpo any) (models.VistaSolicitud, error) {
	crudo, err := json.Marshal(cuerpo)
	if err != nil {
		return models.VistaSolicitud{}, fmt.Errorf("serializar pedido a Operaciones: %w", err)
	}
	// Solo cuentan como falla del breaker los errores de red, los timeouts y los 5xx. Un 4xx es
	// una respuesta válida del negocio, no un problema de la dependencia (teórico 7).
	resp, err := c.breaker.Execute(func() (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+ruta, bytes.NewReader(crudo))
		if err != nil {
			return nil, err
		}
		req.Header.Set(identidad.HeaderOrganizacion, org)
		req.Header.Set("Idempotency-Key", claveIdem)
		req.Header.Set("Content-Type", "application/json")
		resp, err := c.http.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode >= http.StatusInternalServerError {
			resp.Body.Close()
			return nil, fmt.Errorf("%w: %d", errFallaDependencia, resp.StatusCode)
		}
		return resp, nil
	})
	if err != nil {
		// Incluye el breaker abierto (gobreaker.ErrOpenState / ErrTooManyRequests).
		slog.WarnContext(ctx, "no se pudo llamar a Operaciones", slog.String("ruta", ruta), slog.String("error", err.Error()))
		return models.VistaSolicitud{}, fmt.Errorf("%w: %v", ErrNoDisponible, err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		var vista models.VistaSolicitud
		if err := json.NewDecoder(resp.Body).Decode(&vista); err != nil {
			return models.VistaSolicitud{}, fmt.Errorf("decodificar la vista de Operaciones: %w", err)
		}
		return vista, nil
	case http.StatusNotFound:
		return models.VistaSolicitud{}, ErrNoEncontrada
	case http.StatusConflict:
		var p struct {
			Codigo string `json:"codigo"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&p)
		return models.VistaSolicitud{}, ErrConflicto{Codigo: p.Codigo}
	case http.StatusUnprocessableEntity:
		return models.VistaSolicitud{}, ErrDatosInvalidos
	default:
		return models.VistaSolicitud{}, fmt.Errorf("respuesta inesperada de Operaciones: %d", resp.StatusCode)
	}
}
