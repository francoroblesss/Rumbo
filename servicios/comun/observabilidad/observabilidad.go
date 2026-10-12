// Package observabilidad configura logs, métricas y trazas de un servicio con OpenTelemetry
// (D11). Los tres se exportan por OTLP al stack de Grafana (variable
// OTEL_EXPORTER_OTLP_ENDPOINT) y los logs también salen en JSON por stdout.
// Contrato en docs/interfaces-internas.md §7, §8 y §9.
package observabilidad

import (
	"context"
	"errors"
	"log/slog"
	"os"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// NombreInstrumentacion identifica a Rumbo como fuente de métricas y trazas propias.
const NombreInstrumentacion = "github.com/francoroblesss/Rumbo"

// Instancia devuelve el nombre de la réplica: la variable INSTANCIA o, si no está, el hostname
// del contenedor. Permite ver en logs y métricas qué réplica atendió cada request (D12).
func Instancia() string {
	if v := os.Getenv("INSTANCIA"); v != "" {
		return v
	}
	if h, err := os.Hostname(); err == nil {
		return h
	}
	return "desconocida"
}

// Medidor devuelve el meter de Rumbo para crear métricas propias (docs/interfaces-internas.md §8).
func Medidor() metric.Meter { return otel.Meter(NombreInstrumentacion) }

// Trazador devuelve el tracer de Rumbo para crear spans propios.
func Trazador() trace.Tracer { return otel.Tracer(NombreInstrumentacion) }

// Iniciar configura la observabilidad del servicio y deja listo slog.Default().
// Si OTEL_EXPORTER_OTLP_ENDPOINT está vacío (por ejemplo, en tests), no exporta: los logs
// salen igual por stdout y las métricas y trazas quedan en memoria.
// Devuelve una función para vaciar los exportadores al apagar el servicio.
func Iniciar(ctx context.Context, servicio string) (apagar func(context.Context) error, err error) {
	instancia := Instancia()
	res, err := resource.New(ctx, resource.WithAttributes(
		attribute.String("service.name", servicio),
		attribute.String("service.instance.id", instancia),
	))
	if err != nil {
		return nil, err
	}

	// Propagación W3C Trace Context: la usan HTTP (otelhttp/otelgin) y RabbitMQ (mensajeria).
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{}))

	exportar := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != ""
	var apagados []func(context.Context) error

	opcionesTrazas := []sdktrace.TracerProviderOption{sdktrace.WithResource(res)}
	opcionesMetricas := []sdkmetric.Option{sdkmetric.WithResource(res)}
	opcionesLogs := []sdklog.LoggerProviderOption{sdklog.WithResource(res)}

	if exportar {
		// Los exportadores leen OTEL_EXPORTER_OTLP_ENDPOINT del entorno.
		expTrazas, err := otlptracehttp.New(ctx)
		if err != nil {
			return nil, err
		}
		expMetricas, err := otlpmetrichttp.New(ctx)
		if err != nil {
			return nil, err
		}
		expLogs, err := otlploghttp.New(ctx)
		if err != nil {
			return nil, err
		}
		opcionesTrazas = append(opcionesTrazas, sdktrace.WithBatcher(expTrazas))
		opcionesMetricas = append(opcionesMetricas, sdkmetric.WithReader(sdkmetric.NewPeriodicReader(expMetricas)))
		opcionesLogs = append(opcionesLogs, sdklog.WithProcessor(sdklog.NewBatchProcessor(expLogs)))
	}

	proveedorTrazas := sdktrace.NewTracerProvider(opcionesTrazas...)
	proveedorMetricas := sdkmetric.NewMeterProvider(opcionesMetricas...)
	proveedorLogs := sdklog.NewLoggerProvider(opcionesLogs...)
	otel.SetTracerProvider(proveedorTrazas)
	otel.SetMeterProvider(proveedorMetricas)
	global.SetLoggerProvider(proveedorLogs)
	apagados = append(apagados, proveedorTrazas.Shutdown, proveedorMetricas.Shutdown, proveedorLogs.Shutdown)

	// Logs: JSON por stdout (legible con docker compose logs) y, si se exporta, también a
	// Grafana por OpenTelemetry. Ambos llevan trace_id, span_id y correlation_id del contexto.
	manejadores := []slog.Handler{
		slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}).
			WithAttrs([]slog.Attr{slog.String("servicio", servicio), slog.String("instancia", instancia)}),
	}
	if exportar {
		manejadores = append(manejadores, otelslog.NewHandler(servicio, otelslog.WithLoggerProvider(proveedorLogs)))
	}
	slog.SetDefault(slog.New(manejadorContexto{multiManejador(manejadores)}))

	return func(ctx context.Context) error {
		var errs []error
		for _, f := range apagados {
			errs = append(errs, f(ctx))
		}
		return errors.Join(errs...)
	}, nil
}
