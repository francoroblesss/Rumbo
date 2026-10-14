package clima

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/karlseguin/ccache/v3"
	"github.com/sony/gobreaker/v2"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/francoroblesss/Rumbo/servicios/comun/observabilidad"
	"github.com/francoroblesss/Rumbo/servicios/operaciones/dominio"
	"github.com/francoroblesss/Rumbo/servicios/operaciones/puertos"
)

// Parámetros de ADR-012.
const (
	TimeoutIntento    = 1 * time.Second // por intento
	EsperaMinima      = 100 * time.Millisecond
	EsperaMaxima      = 300 * time.Millisecond
	RetryAfterMaximo  = 1 * time.Second // con más espera no se reintenta
	FallasParaAbrir   = 5               // fallas consecutivas que abren el breaker
	TiempoAbierto     = 30 * time.Second
	DuracionCache     = 5 * time.Minute // por el límite de 60 pedidos por minuto
	TamanioCache      = 10
	headerAPIKey      = "X-API-Key"
	headerRetryAfter  = "Retry-After"
	nombreDependencia = "clima"
)

// Cliente es el adaptador de salida hacia la API de Clima del sistema de aeropuerto (contrato
// fijado en docs/contracts/consumidos/clima-v1.yaml). Implementa puertos.ConsultaClima: traduce
// su JSON a dominio.Operabilidad, así el núcleo no conoce el contrato del proveedor (teórico 8).
type Cliente struct {
	url     string
	apiKey  string
	http    *http.Client
	breaker *gobreaker.CircuitBreaker[dominio.Operabilidad]
	cache   *ccache.Cache[dominio.Operabilidad]

	cacheOperaciones metric.Int64Counter
}

var _ puertos.ConsultaClima = (*Cliente)(nil)

// NuevoCliente arma el adaptador con timeout, breaker y caché (ADR-012, teórico 7).
func NuevoCliente(baseURL, apiKey string) *Cliente {
	medidor := observabilidad.Medidor()
	estadoCircuito, _ := medidor.Int64Gauge("rumbo_circuito_estado",
		metric.WithDescription("Estado del circuit breaker: 0 cerrado, 1 semiabierto, 2 abierto"))
	cacheOperaciones, _ := medidor.Int64Counter("rumbo_cache_operaciones_total",
		metric.WithDescription("Operaciones de caché por resultado"))
	dependencia := metric.WithAttributes(attribute.String("dependencia", nombreDependencia))
	estadoCircuito.Record(context.Background(), int64(gobreaker.StateClosed), dependencia)

	return &Cliente{
		url:    baseURL,
		apiKey: apiKey,
		http:   observabilidad.ClienteHTTP(TimeoutIntento),
		breaker: gobreaker.NewCircuitBreaker[dominio.Operabilidad](gobreaker.Settings{
			Name:    nombreDependencia,
			Timeout: TiempoAbierto,
			ReadyToTrip: func(c gobreaker.Counts) bool {
				return c.ConsecutiveFailures >= FallasParaAbrir
			},
			// Solo cuentan como falla los problemas del proveedor (timeout, red, 5xx). Un 4xx o un
			// 429 no dicen que el proveedor esté caído: no suman ni restan.
			IsSuccessful: func(err error) bool { return err == nil },
			IsExcluded:   func(err error) bool { return err != nil && !esFalla(err) },
			OnStateChange: func(_ string, desde, hacia gobreaker.State) {
				estadoCircuito.Record(context.Background(), int64(hacia), dependencia)
				slog.Warn("cambió el estado del circuit breaker", slog.String("dependencia", nombreDependencia),
					slog.String("desde", desde.String()), slog.String("hacia", hacia.String()))
			},
		}),
		cache:            ccache.New(ccache.Configure[dominio.Operabilidad]().MaxSize(TamanioCache)),
		cacheOperaciones: cacheOperaciones,
	}
}

// Operabilidad devuelve la operabilidad del aeropuerto o puertos.ErrClimaNoDisponible.
func (c *Cliente) Operabilidad(ctx context.Context, icao string) (dominio.Operabilidad, error) {
	if item := c.cache.Get(icao); item != nil && !item.Expired() {
		c.contarCache(ctx, "hit")
		return item.Value(), nil
	}
	c.contarCache(ctx, "miss")

	o, err := c.consultar(ctx, icao)
	if err != nil {
		return dominio.Operabilidad{}, fmt.Errorf("%w: %v", puertos.ErrClimaNoDisponible, err)
	}
	// Solo se guardan las respuestas 200: un error no se cachea.
	c.cache.Set(icao, o, DuracionCache)
	return o, nil
}

func (c *Cliente) contarCache(ctx context.Context, resultado string) {
	c.cacheOperaciones.Add(ctx, 1, metric.WithAttributes(attribute.String("cache", nombreDependencia), attribute.String("resultado", resultado)))
}

// consultar hace hasta dos intentos: el reintento solo ante errores transitorios (es una lectura,
// se puede repetir sin efectos). Cada intento pasa por el breaker: abierto, no se llama.
func (c *Cliente) consultar(ctx context.Context, icao string) (dominio.Operabilidad, error) {
	o, err := c.breaker.Execute(func() (dominio.Operabilidad, error) { return c.intento(ctx, icao) })
	var e *errorIntento
	if err == nil || !errors.As(err, &e) || !e.reintentable {
		return o, err
	}
	select {
	case <-ctx.Done():
		return dominio.Operabilidad{}, ctx.Err()
	case <-time.After(e.espera):
	}
	return c.breaker.Execute(func() (dominio.Operabilidad, error) { return c.intento(ctx, icao) })
}

// errorIntento clasifica un intento fallido.
type errorIntento struct {
	causa        error
	falla        bool          // cuenta para el breaker (timeout, red, 5xx)
	reintentable bool          // vale la pena un segundo intento
	espera       time.Duration // antes del reintento
}

func (e *errorIntento) Error() string { return e.causa.Error() }

func esFalla(err error) bool {
	var e *errorIntento
	return errors.As(err, &e) && e.falla
}

func jitter() time.Duration {
	return EsperaMinima + rand.N(EsperaMaxima-EsperaMinima+1)
}

// respuesta es lo único que Rumbo lee del contrato (se ignoran los campos desconocidos).
type respuesta struct {
	ObservedAt  time.Time `json:"observed_at"`
	Operability struct {
		Operable bool     `json:"operable"`
		Reasons  []string `json:"reasons"`
	} `json:"operability"`
}

func (c *Cliente) intento(ctx context.Context, icao string) (dominio.Operabilidad, error) {
	pedido, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.url+"/v1/airports/"+url.PathEscape(icao)+"/conditions", nil)
	if err != nil {
		return dominio.Operabilidad{}, &errorIntento{causa: err}
	}
	pedido.Header.Set(headerAPIKey, c.apiKey)
	pedido.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(pedido)
	if err != nil {
		// Timeout o error de red: transitorio.
		return dominio.Operabilidad{}, &errorIntento{causa: err, falla: true, reintentable: true, espera: jitter()}
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusOK:
		var r respuesta
		if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
			return dominio.Operabilidad{}, &errorIntento{causa: fmt.Errorf("respuesta del clima inválida: %w", err)}
		}
		return dominio.Operabilidad{Operable: r.Operability.Operable, Motivos: r.Operability.Reasons, ObservadoEn: r.ObservedAt.UTC()}, nil

	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable:
		// Límite superado o fuente caída: se reintenta solo si el proveedor pide esperar poco.
		_, _ = io.Copy(io.Discard, resp.Body)
		espera, ok := retryAfter(resp.Header.Get(headerRetryAfter))
		return dominio.Operabilidad{}, &errorIntento{
			causa:        fmt.Errorf("el clima respondió %d (Retry-After %q)", resp.StatusCode, resp.Header.Get(headerRetryAfter)),
			falla:        resp.StatusCode >= http.StatusInternalServerError,
			reintentable: ok && espera <= RetryAfterMaximo,
			espera:       espera,
		}

	case resp.StatusCode >= http.StatusInternalServerError:
		_, _ = io.Copy(io.Discard, resp.Body)
		return dominio.Operabilidad{}, &errorIntento{causa: fmt.Errorf("el clima respondió %d", resp.StatusCode),
			falla: true, reintentable: true, espera: jitter()}

	default:
		// 400, 401, 404: es un error de configuración nuestro (clave o ICAO). No se reintenta.
		cuerpo, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		var p struct {
			Code string `json:"code"`
		}
		_ = json.Unmarshal(cuerpo, &p)
		slog.ErrorContext(ctx, "el servicio de clima rechazó el pedido: revisar CLIMA_API_KEY y CLIMA_ICAO",
			slog.Int("estado_http", resp.StatusCode), slog.String("codigo", p.Code))
		return dominio.Operabilidad{}, &errorIntento{causa: fmt.Errorf("el clima respondió %d %s", resp.StatusCode, p.Code)}
	}
}

// retryAfter interpreta Retry-After en segundos (el formato del contrato del proveedor).
func retryAfter(valor string) (time.Duration, bool) {
	segundos, err := strconv.Atoi(valor)
	if err != nil || segundos < 0 {
		return 0, false
	}
	return time.Duration(segundos) * time.Second, true
}
