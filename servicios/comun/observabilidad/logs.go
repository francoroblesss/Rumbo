package observabilidad

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

type claveCorrelacion struct{}

// ConCorrelacion guarda el correlation id en el contexto.
func ConCorrelacion(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, claveCorrelacion{}, id)
}

// Correlacion devuelve el correlation id del contexto ("" si no hay).
func Correlacion(ctx context.Context) string {
	id, _ := ctx.Value(claveCorrelacion{}).(string)
	return id
}

// manejadorContexto agrega a cada log los identificadores del contexto: trace_id y span_id de
// la traza activa y el correlation_id, para poder seguir una operación entre servicios.
type manejadorContexto struct{ slog.Handler }

func (m manejadorContexto) Handle(ctx context.Context, r slog.Record) error {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		r.AddAttrs(slog.String("trace_id", sc.TraceID().String()), slog.String("span_id", sc.SpanID().String()))
	}
	if id := Correlacion(ctx); id != "" {
		r.AddAttrs(slog.String("correlation_id", id))
	}
	return m.Handler.Handle(ctx, r)
}

func (m manejadorContexto) WithAttrs(attrs []slog.Attr) slog.Handler {
	return manejadorContexto{m.Handler.WithAttrs(attrs)}
}

func (m manejadorContexto) WithGroup(nombre string) slog.Handler {
	return manejadorContexto{m.Handler.WithGroup(nombre)}
}
