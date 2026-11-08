package aplicacion

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/francoroblesss/Rumbo/servicios/operaciones/dominio"
	"github.com/francoroblesss/Rumbo/servicios/operaciones/puertos"
)

// Casos de uso de la aplicación web (docs/interfaces-internas.md §12, ADR-024).

// TamanioMaximo es el tamaño de página máximo (§11.1).
const TamanioMaximo = 100

// estadosListables son los estados del traslado que se producen en esta etapa (ADR-024): los
// únicos que acepta el filtro del listado.
var estadosListables = map[dominio.EstadoTraslado]bool{
	dominio.TrasladoProgramado: true, dominio.TrasladoEnCurso: true,
	dominio.TrasladoCompletado: true, dominio.TrasladoCancelado: true,
}

// Franjas devuelve todas las franjas del calendario de la fecha con su ocupación. A diferencia
// de Disponibilidad, no aplica la anticipación ni el peso: el operador ve también las franjas de
// hoy que ya empezaron. Es una lectura sin bloqueo.
func (a *Solicitudes) Franjas(ctx context.Context, fecha string) (puertos.FranjasDelDia, error) {
	dia, err := time.ParseInLocation(time.DateOnly, fecha, zonaRumbo)
	if err != nil {
		return puertos.FranjasDelDia{}, dominio.ErrDatosInvalidos
	}
	franjas, err := a.calendario.Dia(dia.Year(), dia.Month(), dia.Day(), a.reloj())
	if errors.Is(err, dominio.ErrFranjaInvalida) {
		return puertos.FranjasDelDia{}, dominio.ErrDatosInvalidos
	}
	if err != nil {
		return puertos.FranjasDelDia{}, err
	}
	if franjas, err = a.conOcupacion(ctx, franjas); err != nil {
		return puertos.FranjasDelDia{}, err
	}
	resultado := puertos.FranjasDelDia{Fecha: fecha, Franjas: []puertos.FranjaOcupada{}}
	for _, f := range franjas {
		resultado.Franjas = append(resultado.Franjas, puertos.FranjaOcupada{
			Inicio: hora(f.Inicio), Fin: hora(f.Fin), CupoTraslados: f.CupoTraslados,
			TrasladosReservados: f.TrasladosReservados, CupoKg: f.CupoKg, KgReservados: f.KgReservados,
		})
	}
	return resultado, nil
}

// Listar devuelve una página de solicitudes, ordenadas por inicio de la franja y después por
// creación. La fecha (día en hora de Argentina) se traduce acá al rango UTC que se guarda.
func (a *Solicitudes) Listar(ctx context.Context, f puertos.FiltroListado) (puertos.Pagina, error) {
	if f.Pagina < 1 || f.Tamanio < 1 || f.Tamanio > TamanioMaximo ||
		(f.EstadoTraslado != "" && !estadosListables[dominio.EstadoTraslado(f.EstadoTraslado)]) {
		return puertos.Pagina{}, dominio.ErrDatosInvalidos
	}
	filtro := puertos.FiltroSolicitudes{Organizacion: f.Organizacion, EstadoTraslado: f.EstadoTraslado}
	if f.Fecha != "" {
		dia, err := time.ParseInLocation(time.DateOnly, f.Fecha, zonaRumbo)
		if err != nil {
			return puertos.Pagina{}, dominio.ErrDatosInvalidos
		}
		desde, hasta := dia.UTC(), dia.AddDate(0, 0, 1).UTC()
		filtro.FranjaDesde, filtro.FranjaHasta = &desde, &hasta
	}
	var lista []*dominio.Solicitud
	var total int
	err := a.uow.Ejecutar(ctx, func(ctx context.Context, tx puertos.Transaccion) error {
		var err error
		lista, total, err = tx.Listar(ctx, filtro, (f.Pagina-1)*f.Tamanio, f.Tamanio)
		return err
	})
	if err != nil {
		return puertos.Pagina{}, err
	}
	resultado := puertos.Pagina{Resultados: make([]puertos.Vista, 0, len(lista)), Pagina: f.Pagina, Tamanio: f.Tamanio, Total: total}
	for _, s := range lista {
		resultado.Resultados = append(resultado.Resultados, NuevaVista(s))
	}
	return resultado, nil
}

// AvanzarEstado aplica el avance que pide el operador (R-47): PROGRAMADO → EN_CURSO →
// COMPLETADO. Sigue el patrón de cambiar (bloqueo de la solicitud, regla del dominio, guardar,
// outbox). Si el traslado ya está en el estado pedido, devuelve la vista sin escribir ni
// publicar: el reintento es seguro (CA-38).
func (a *Solicitudes) AvanzarEstado(ctx context.Context, org, id string, destino dominio.EstadoTraslado) (puertos.Vista, error) {
	if destino != dominio.TrasladoEnCurso && destino != dominio.TrasladoCompletado {
		return puertos.Vista{}, dominio.ErrDatosInvalidos
	}
	ahora := a.reloj()

	// 1. Lectura sin bloqueo y, si sale al aeropuerto, el clima FUERA de la transacción: nunca se
	// llama a la red con un bloqueo de MySQL tomado (teórico 7, ADR-022).
	s, err := a.leer(ctx, org, id)
	if err != nil {
		return puertos.Vista{}, err
	}
	var alerta *dominio.AlertaClimatica
	if destino == dominio.TrasladoEnCurso && !s.EstadoPedido(destino) {
		alerta, _ = a.consultarClima(ctx, s, ahora, momentoInicio)
	}

	// 2. En la transacción: la transición y, si el clima dejó una alerta nueva, la alerta. El
	// cambio de estado se hace aunque el clima no haya respondido (fail-open, CA-36).
	aplicada := false
	vista, err := a.cambiar(ctx, org, id, nil, func(_ context.Context, _ puertos.Transaccion, s *dominio.Solicitud, emitir func(dominio.Evento) error) error {
		if s.EstadoPedido(destino) {
			return errSinCambios
		}
		var ev dominio.Evento
		var err error
		if destino == dominio.TrasladoEnCurso {
			ev, err = s.IniciarTraslado(ahora)
		} else {
			ev, err = s.CompletarTraslado(ahora)
		}
		if err != nil {
			return err
		}
		if err := emitir(ev); err != nil {
			return err
		}
		aplicada = true
		return registrarAlerta(s, alerta, ahora, emitir)
	})
	if err != nil {
		return puertos.Vista{}, err
	}
	if aplicada {
		a.trasladosEstado.Add(ctx, 1, metric.WithAttributes(attribute.String("estado", string(destino))))
		slog.InfoContext(ctx, "estado del traslado cambiado", slog.String("solicitud_id", id), slog.String("estado", string(destino)))
	}
	return vista, nil
}

// registrarAlerta deja la alerta en el traslado y emite su evento, salvo que no haya alerta
// (aeropuerto operable o sin consulta) o que ya tuviera la misma (idempotencia).
func registrarAlerta(s *dominio.Solicitud, alerta *dominio.AlertaClimatica, ahora time.Time, emitir func(dominio.Evento) error) error {
	if alerta == nil || s.Traslado == nil || alerta.Igual(s.Traslado.Alerta) {
		return nil
	}
	ev, err := s.RegistrarAlertaClimatica(*alerta, ahora)
	if err != nil {
		return err
	}
	return emitir(ev)
}

// CancelarDesdeWeb es el mismo caso de uso que la cancelación de Integración (Cancelar): se
// aplica en el momento y devuelve el lugar a la franja (R-45). La clave de idempotencia se
// guarda con la organización de la solicitud, así que primero se lee: para el operador (org "")
// es la única forma de saberla; para el cliente, además verifica que sea de su organización.
func (a *Solicitudes) CancelarDesdeWeb(ctx context.Context, org, id, motivo string, idem *puertos.Idempotencia) (puertos.Vista, error) {
	s, err := a.leer(ctx, org, id)
	if err != nil {
		return puertos.Vista{}, err
	}
	return a.Cancelar(ctx, s.OrganizacionID, id, motivo, idem)
}
