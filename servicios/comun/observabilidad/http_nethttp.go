package observabilidad

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// InstrumentarHTTP hace lo mismo que Instrumentar pero para net/http (el gateway no usa Gin):
// la traza de cada request (continuando la del llamador), el correlation id en el contexto, en
// la request reenviada y en la respuesta, la métrica de duración y un log por request.
//
// La métrica usa la ruta agrupada (ver rutaAgrupada) y no el path completo, para no crear una
// serie por cada id.
func InstrumentarHTTP(h http.Handler, servicio string) http.Handler {
	duracion, _ := Medidor().Float64Histogram("rumbo_http_duracion_segundos",
		metric.WithDescription("Duración de las requests HTTP atendidas"), metric.WithUnit("s"))
	instancia := Instancia()

	interno := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(HeaderCorrelacion)
		if id == "" {
			id = uuid.NewString()
			r.Header.Set(HeaderCorrelacion, id)
		}
		ctx := ConCorrelacion(r.Context(), id)
		r = r.WithContext(ctx)
		w.Header().Set(HeaderCorrelacion, id)

		registrador := &registradorEstado{ResponseWriter: w, estado: http.StatusOK}
		inicio := time.Now()
		h.ServeHTTP(registrador, r)
		transcurrido := time.Since(inicio)

		ruta := rutaAgrupada(r.URL.Path)
		duracion.Record(ctx, transcurrido.Seconds(), metric.WithAttributes(
			attribute.String("metodo", r.Method),
			attribute.String("ruta", ruta),
			attribute.Int("estado", registrador.estado),
			attribute.String("instancia", instancia),
		))
		nivel := slog.LevelInfo
		if registrador.estado >= http.StatusInternalServerError {
			nivel = slog.LevelError
		}
		slog.Log(ctx, nivel, "request atendida",
			slog.String("metodo", r.Method), slog.String("ruta", ruta),
			slog.Int("estado_http", registrador.estado), slog.Int64("duracion_ms", transcurrido.Milliseconds()))
	})

	return otelhttp.NewHandler(interno, servicio,
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
			return r.Method + " " + rutaAgrupada(r.URL.Path)
		}))
}

// rutaAgrupada reduce el path a su prefijo estable: /api/v1/<servicio> para la API de la
// aplicación web y /<versión>/<recurso> para el contrato público (por ejemplo /v2/traslados).
func rutaAgrupada(path string) string {
	segmentos := strings.Split(strings.Trim(path, "/"), "/")
	cantidad := 2
	if segmentos[0] == "api" {
		cantidad = 3
	}
	if len(segmentos) < cantidad {
		cantidad = len(segmentos)
	}
	return "/" + strings.Join(segmentos[:cantidad], "/")
}

// registradorEstado recuerda el código de estado que escribió el handler.
type registradorEstado struct {
	http.ResponseWriter
	estado int
}

func (r *registradorEstado) WriteHeader(estado int) {
	r.estado = estado
	r.ResponseWriter.WriteHeader(estado)
}

// Unwrap permite que http.NewResponseController llegue al ResponseWriter original
// (el proxy inverso lo necesita para hacer Flush).
func (r *registradorEstado) Unwrap() http.ResponseWriter { return r.ResponseWriter }
