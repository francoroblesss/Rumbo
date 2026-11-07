package aplicacion

import (
	"context"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/francoroblesss/Rumbo/servicios/operaciones/dominio"
	"github.com/francoroblesss/Rumbo/servicios/operaciones/puertos"
)

// Clima en las reservas (docs/interfaces-internas.md §13, ADR-025).

// Momentos en los que se consulta el clima (atributo momento de rumbo_clima_consultas_total).
const (
	momentoReserva = "reserva" // después de reservar, en el consumidor
	momentoInicio  = "inicio"  // al pasar a EN_CURSO
)

// consultarClima aplica la regla de clima (R-46) a un traslado: si corresponde, consulta la
// operabilidad del aeropuerto con el adaptador de siempre (timeout, reintento, breaker y caché;
// ADR-012) y devuelve la alerta que hay que dejar, o nil si el aeropuerto es operable o si no
// correspondía consultar. resultado es "" si no se consultó.
//
// Fail-open (teórico 7): si el clima no responde, no es un error sino una alerta
// NO_VERIFICADO; quien llama sigue con su operación.
//
// Se llama siempre FUERA de una transacción: la latencia del proveedor no puede retener
// bloqueos de MySQL.
func (a *Solicitudes) consultarClima(ctx context.Context, s *dominio.Solicitud, ahora time.Time, momento string) (*dominio.AlertaClimatica, string) {
	if !s.RequiereConsultaClima(ahora) {
		return nil, ""
	}
	operabilidad, err := a.clima.Operabilidad(ctx, a.icao)
	var alerta *dominio.AlertaClimatica
	var resultado string
	switch {
	case err != nil:
		resultado = "no_verificado"
		alerta = &dominio.AlertaClimatica{Estado: dominio.AlertaNoVerificado}
		slog.WarnContext(ctx, "clima no verificado", slog.String("solicitud_id", s.ID),
			slog.String("momento", momento), slog.String("error", err.Error()))
	case !operabilidad.Operable:
		resultado = "no_operable"
		observado := operabilidad.ObservadoEn
		alerta = &dominio.AlertaClimatica{Estado: dominio.AlertaNoOperable, Motivos: operabilidad.Motivos, ObservadoEn: &observado}
	default:
		resultado = "operable"
	}
	a.climaConsultas.Add(ctx, 1, metric.WithAttributes(attribute.String("resultado", resultado), attribute.String("momento", momento)))
	return alerta, resultado
}

// RevisarClimaDeReserva es el caso de uso del consumidor de operaciones.solicitud.aceptada
// (§13.2): la reserva ya respondió su 201 y el clima se revisa después, sin demorarla (separar
// la aceptación del procesamiento, teórico 4).
//
//  1. relee la solicitud sin bloqueo; si no va al aeropuerto, ya no está PROGRAMADO o falta más
//     de 6 h, no hace nada;
//  2. consulta el clima fuera de la transacción;
//  3. en una transacción corta, con la solicitud bloqueada, registra la alerta si el traslado
//     sigue PROGRAMADO y la alerta cambió, con su evento en el outbox.
//
// Es idempotente: un evento reentregado encuentra la misma alerta y no escribe ni publica. Un
// clima caído no es un error (queda NO_VERIFICADO); un error de MySQL sí, y el consumidor lo
// reintenta. Una solicitud inexistente devuelve puertos.ErrNoEncontrada.
func (a *Solicitudes) RevisarClimaDeReserva(ctx context.Context, solicitudID string) error {
	ahora := a.reloj()
	s, err := a.leer(ctx, "", solicitudID)
	if err != nil {
		return err
	}
	alerta, resultado := a.consultarClima(ctx, s, ahora, momentoReserva)
	if resultado == "" {
		return nil
	}
	if alerta != nil {
		_, err = a.cambiar(ctx, "", solicitudID, nil, func(_ context.Context, _ puertos.Transaccion, s *dominio.Solicitud, emitir func(dominio.Evento) error) error {
			if s.Traslado == nil || s.Traslado.Estado != dominio.TrasladoProgramado || alerta.Igual(s.Traslado.Alerta) {
				return errSinCambios
			}
			return registrarAlerta(s, alerta, ahora, emitir)
		})
		if err != nil {
			return err
		}
	}
	slog.InfoContext(ctx, "clima revisado después de la reserva", slog.String("solicitud_id", solicitudID),
		slog.String("resultado", resultado))
	return nil
}
