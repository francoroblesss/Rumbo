package observabilidad

import (
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/francoroblesss/Rumbo/servicios/comun/identidad"
)

// HeaderCorrelacion viaja en todas las llamadas HTTP internas (docs/interfaces-internas.md §2).
const HeaderCorrelacion = "X-Correlation-ID"

// bucketsDuracionHTTP son los límites del histograma rumbo_http_duracion_segundos, en segundos
// como la métrica. Sin ellos, el SDK de OpenTelemetry usa límites pensados para milisegundos
// (0, 5, 10, 25…): toda request de menos de 5 s caía en el primer bucket y el p95 daba 4,75 s
// constante (docs/POSTMORTEM.md, acción de mejora 1). Cubren el SLO de 300 ms (ADR-014) y el
// deadline de 5 s. Los usan Instrumentar (Gin) e InstrumentarHTTP (gateway).
var bucketsDuracionHTTP = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

// Instrumentar agrega a un router de Gin: la traza de cada request (continuando la del
// llamador), el correlation id en el contexto y en la respuesta, la métrica de duración y un
// log por request. Usar con gin.New() (no gin.Default(), que agrega su propio log de texto).
func Instrumentar(r *gin.Engine, servicio string) {
	duracion, _ := Medidor().Float64Histogram("rumbo_http_duracion_segundos",
		metric.WithDescription("Duración de las requests HTTP atendidas"), metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(bucketsDuracionHTTP...))
	instancia := Instancia()

	r.Use(otelgin.Middleware(servicio), func(c *gin.Context) {
		id := c.GetHeader(HeaderCorrelacion)
		if id == "" {
			id = uuid.NewString()
		}
		ctx := ConCorrelacion(c.Request.Context(), id)
		c.Request = c.Request.WithContext(ctx)
		c.Header(HeaderCorrelacion, id)

		inicio := time.Now()
		c.Next()
		transcurrido := time.Since(inicio)

		ruta := c.FullPath()
		if ruta == "" {
			ruta = "sin_ruta"
		}
		duracion.Record(ctx, transcurrido.Seconds(), metric.WithAttributes(
			attribute.String("metodo", c.Request.Method),
			attribute.String("ruta", ruta),
			attribute.Int("estado", c.Writer.Status()),
			attribute.String("instancia", instancia),
		))
		nivel := slog.LevelInfo
		if c.Writer.Status() >= http.StatusInternalServerError {
			nivel = slog.LevelError
		}
		slog.Log(ctx, nivel, "request atendida",
			slog.String("metodo", c.Request.Method), slog.String("ruta", ruta),
			slog.Int("estado_http", c.Writer.Status()), slog.Int64("duracion_ms", transcurrido.Milliseconds()))
	})
}

// ClienteHTTP devuelve un cliente HTTP con timeout que propaga la traza (traceparent) y el
// correlation id del contexto en cada llamada. Usarlo con http.NewRequestWithContext.
func ClienteHTTP(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: transporteCorrelacion{base: otelhttp.NewTransport(http.DefaultTransport)},
	}
}

// ClienteInterno es ClienteHTTP más el secreto interno (identidad.HeaderInterno), para las
// llamadas entre servicios de Rumbo (docs/interfaces-internas.md §11.2, ADR-026). El secreto se
// lee de SECRETO_INTERNO al crear el cliente; si está vacío, no se agrega nada.
// Nunca usarlo para llamar a un sistema de otro grupo: le estaríamos mandando nuestro secreto.
func ClienteInterno(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: transporteCorrelacion{
			base:    otelhttp.NewTransport(http.DefaultTransport),
			secreto: os.Getenv(identidad.VariableSecretoInterno),
		},
	}
}

type transporteCorrelacion struct {
	base    http.RoundTripper
	secreto string // vacío en ClienteHTTP
}

func (t transporteCorrelacion) RoundTrip(req *http.Request) (*http.Response, error) {
	id := Correlacion(req.Context())
	agregarCorrelacion := id != "" && req.Header.Get(HeaderCorrelacion) == ""
	if agregarCorrelacion || t.secreto != "" {
		req = req.Clone(req.Context()) // un RoundTripper no debe modificar la request recibida
		if agregarCorrelacion {
			req.Header.Set(HeaderCorrelacion, id)
		}
		if t.secreto != "" {
			req.Header.Set(identidad.HeaderInterno, t.secreto)
		}
	}
	return t.base.RoundTrip(req)
}
