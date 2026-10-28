package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/sony/gobreaker/v2"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/francoroblesss/Rumbo/servicios/comun/identidad"
	"github.com/francoroblesss/Rumbo/servicios/comun/observabilidad"
	"github.com/francoroblesss/Rumbo/servicios/comun/problema"
	"github.com/francoroblesss/Rumbo/servicios/integracion/models"
)

// Errores del cliente: el servicio los traduce a los códigos del contrato público.
var (
	ErrNoEncontrada = errors.New("Operaciones no tiene la solicitud")
	ErrNoDisponible = errors.New("Operaciones no está disponible")
)

// ErrRechazo es una respuesta 4xx de Operaciones (una regla de negocio o un dato inválido), con
// su Problem Details: el código ya es uno del contrato público, así que se reenvía tal cual.
type ErrRechazo struct {
	Estado  int
	Codigo  string
	Titulo  string
	Detalle string
	Errores []problema.ErrorCampo
}

func (e ErrRechazo) Error() string {
	return fmt.Sprintf("Operaciones rechazó el pedido: %d %s", e.Estado, e.Codigo)
}

// TimeoutOperaciones es el timeout de cada llamada (interfaces-internas §4.1, ADR-004).
const TimeoutOperaciones = 2 * time.Second

// errFallaDependencia marca una respuesta 5xx para que el breaker la cuente como falla.
var errFallaDependencia = errors.New("Operaciones respondió con error de servidor")

// OperacionesClient llama a los endpoints internos de Operaciones (interfaces-internas §4.1).
type OperacionesClient interface {
	Disponibilidad(ctx context.Context, fecha string, pesoKg *float64) (models.Disponibilidad, error)
	Reservar(ctx context.Context, org, claveIdem string, pedido models.PedidoTraslado) (models.VistaSolicitud, error)
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

func (c *operacionesHTTP) Disponibilidad(ctx context.Context, fecha string, pesoKg *float64) (models.Disponibilidad, error) {
	q := url.Values{"fecha": {fecha}}
	if pesoKg != nil {
		q.Set("peso_kg", strconv.FormatFloat(*pesoKg, 'f', -1, 64))
	}
	var d models.Disponibilidad
	err := c.llamar(ctx, http.MethodGet, "/internas/disponibilidad?"+q.Encode(), "", "", nil, &d)
	return d, err
}

func (c *operacionesHTTP) Reservar(ctx context.Context, org, claveIdem string, pedido models.PedidoTraslado) (models.VistaSolicitud, error) {
	var v models.VistaSolicitud
	err := c.llamar(ctx, http.MethodPost, "/internas/reservas", org, claveIdem, pedido, &v)
	return v, err
}

func (c *operacionesHTTP) Cancelar(ctx context.Context, org, claveIdem, id, motivo string) (models.VistaSolicitud, error) {
	var v models.VistaSolicitud
	err := c.llamar(ctx, http.MethodPost, "/internas/solicitudes/"+url.PathEscape(id)+"/cancelacion", org, claveIdem,
		map[string]string{"motivo": motivo}, &v)
	return v, err
}

// llamar hace la request a través del breaker y decodifica la respuesta 2xx en destino.
func (c *operacionesHTTP) llamar(ctx context.Context, metodo, ruta, org, claveIdem string, cuerpo, destino any) error {
	var crudo []byte
	if cuerpo != nil {
		var err error
		if crudo, err = json.Marshal(cuerpo); err != nil {
			return fmt.Errorf("serializar pedido a Operaciones: %w", err)
		}
	}
	// Solo cuentan como falla del breaker los errores de red, los timeouts y los 5xx. Un 4xx es
	// una respuesta válida del negocio, no un problema de la dependencia (teórico 7).
	resp, err := c.breaker.Execute(func() (*http.Response, error) {
		var body io.Reader
		if crudo != nil {
			body = bytes.NewReader(crudo)
		}
		req, err := http.NewRequestWithContext(ctx, metodo, c.baseURL+ruta, body)
		if err != nil {
			return nil, err
		}
		if org != "" {
			req.Header.Set(identidad.HeaderOrganizacion, org)
		}
		if claveIdem != "" {
			req.Header.Set("Idempotency-Key", claveIdem)
		}
		if crudo != nil {
			req.Header.Set("Content-Type", "application/json")
		}
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
		return fmt.Errorf("%w: %v", ErrNoDisponible, err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated:
		if err := json.NewDecoder(resp.Body).Decode(destino); err != nil {
			return fmt.Errorf("decodificar la respuesta de Operaciones: %w", err)
		}
		return nil
	case resp.StatusCode == http.StatusNotFound:
		return ErrNoEncontrada
	case resp.StatusCode >= http.StatusBadRequest && resp.StatusCode < http.StatusInternalServerError:
		var p struct {
			Codigo  string                `json:"codigo"`
			Titulo  string                `json:"title"`
			Detalle string                `json:"detail"`
			Errores []problema.ErrorCampo `json:"errores"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&p)
		return ErrRechazo{Estado: resp.StatusCode, Codigo: p.Codigo, Titulo: p.Titulo, Detalle: p.Detalle, Errores: p.Errores}
	default:
		return fmt.Errorf("respuesta inesperada de Operaciones: %d", resp.StatusCode)
	}
}
