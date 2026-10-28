package dominio

import "errors"

// Errores de dominio: una regla impide la operación. El adaptador HTTP los traduce a los códigos
// del contrato (docs/interfaces-internas.md §3).
var (
	ErrEstadoNoPermite        = errors.New("el estado actual no permite la operación")
	ErrContrapropuestaVencida = errors.New("la contrapropuesta venció")
	ErrSinHoraPedida          = errors.New("la solicitud no tiene hora pedida: hay que contraproponer")
	ErrHorarioEnElPasado      = errors.New("el horario ya pasó")
	ErrDatosInvalidos         = errors.New("datos inválidos")
	// Reserva de una franja (capacidad publicada, ADR-021).
	ErrFranjaInvalida           = errors.New("no es el inicio de una franja que se pueda reservar")
	ErrAnticipacionInsuficiente = errors.New("la franja empieza antes de la anticipación mínima")
	ErrSinDisponibilidad        = errors.New("la franja no tiene lugar para esta carga")
	ErrExcedeCapacidad          = errors.New("la carga supera el cupo total de una franja")
)
