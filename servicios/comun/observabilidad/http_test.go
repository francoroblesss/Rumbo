package observabilidad

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// Una request de ~20 ms tiene que caer en el bucket (0.01, 0.025], no en el primero: con los
// límites por defecto de OpenTelemetry (pensados para ms) caía en [0, 5] y el p95 daba 4,75 s
// (docs/POSTMORTEM.md, acción de mejora 1). Se prueba con Gin (Instrumentar) y con net/http
// (InstrumentarHTTP, el gateway).
func TestDuracionHTTPCaeEnElBucketEnSegundos(t *testing.T) {
	lenta := func() { time.Sleep(20 * time.Millisecond) }

	casos := map[string]func() http.Handler{
		"gin": func() http.Handler {
			gin.SetMode(gin.TestMode)
			r := gin.New()
			Instrumentar(r, "prueba")
			r.GET("/traslados", func(c *gin.Context) { lenta(); c.Status(http.StatusOK) })
			return r
		},
		"net/http": func() http.Handler {
			return InstrumentarHTTP(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				lenta()
				w.WriteHeader(http.StatusOK)
			}), "prueba")
		},
	}
	for nombre, armar := range casos {
		t.Run(nombre, func(t *testing.T) {
			lector := sdkmetric.NewManualReader()
			proveedor := sdkmetric.NewMeterProvider(sdkmetric.WithReader(lector))
			anterior := otel.GetMeterProvider()
			otel.SetMeterProvider(proveedor)
			t.Cleanup(func() { otel.SetMeterProvider(anterior) })

			armar().ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/traslados", nil))

			punto := puntoDuracion(t, lector)
			if !slices.Equal(punto.Bounds, bucketsDuracionHTTP) {
				t.Fatalf("límites = %v, se esperaban %v", punto.Bounds, bucketsDuracionHTTP)
			}
			// BucketCounts[i] cuenta (Bounds[i-1], Bounds[i]]: el índice 2 es (0.01, 0.025].
			if punto.Count != 1 || punto.BucketCounts[2] != 1 {
				t.Fatalf("la request de %.3f s no cayó en (0.01, 0.025]: buckets %v", punto.Sum, punto.BucketCounts)
			}
		})
	}
}

func puntoDuracion(t *testing.T, lector *sdkmetric.ManualReader) metricdata.HistogramDataPoint[float64] {
	t.Helper()
	var datos metricdata.ResourceMetrics
	if err := lector.Collect(context.Background(), &datos); err != nil {
		t.Fatal(err)
	}
	for _, alcance := range datos.ScopeMetrics {
		for _, m := range alcance.Metrics {
			if m.Name != "rumbo_http_duracion_segundos" {
				continue
			}
			h, ok := m.Data.(metricdata.Histogram[float64])
			if !ok || len(h.DataPoints) != 1 {
				t.Fatalf("histograma inesperado: %#v", m.Data)
			}
			return h.DataPoints[0]
		}
	}
	t.Fatal("no se registró rumbo_http_duracion_segundos")
	return metricdata.HistogramDataPoint[float64]{}
}
