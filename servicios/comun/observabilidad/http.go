package observabilidad

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// HeaderCorrelacion viaja en todas las llamadas HTTP internas (docs/interfaces-internas.md §2).
const HeaderCorrelacion = "X-Correlation-ID"

// Instrumentar agrega a un router de Gin: la traza de cada request (continuando la del
// llamador), el correlation id en el contexto y en la respuesta, la métrica de duración y un
// log por request. Usar con gin.New() (no gin.Default(), que agrega su propio log de texto).
func Instrumentar(r *gin.Engine, servicio string) {
	duracion, _ := Medidor().Float64Histogram("rumbo_http_duracion_segundos",
		metric.WithDescription("Duración de las requests HTTP atendidas"), metric.WithUnit("s"))
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

type transporteCorrelacion struct{ base http.RoundTripper }

func (t transporteCorrelacion) RoundTrip(req *http.Request) (*http.Response, error) {
	if id := Correlacion(req.Context()); id != "" && req.Header.Get(HeaderCorrelacion) == "" {
		req = req.Clone(req.Context())
		req.Header.Set(HeaderCorrelacion, id)
	}
	return t.base.RoundTrip(req)
}
