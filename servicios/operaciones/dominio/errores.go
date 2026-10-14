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
)
