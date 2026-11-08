package dominio

import (
	"slices"
	"time"
)

// OrigenSolicitud indica por dónde entró la solicitud: la capacidad publicada (externo) o la
// aplicación web de Rumbo (interno).
type OrigenSolicitud string

const (
	OrigenExterno OrigenSolicitud = "externo"
	OrigenInterno OrigenSolicitud = "interno"
)

// EstadoSolicitud es el ciclo de la solicitud (dominio.md §6).
type EstadoSolicitud string

const (
	// Desde ADR-024 solo se produce ACEPTADA (la reserva). Los demás estados de la revisión
	// (RECIBIDA, CONTRAPROPUESTA, RECHAZADA, …) quedaron en el tag entrega-2.
	SolicitudAceptada EstadoSolicitud = "ACEPTADA"
)

// EstadoCancelacion es el estado de la cancelación de un traslado ya aceptado.
type EstadoCancelacion string

// Una reserva se cancela siempre en el momento (R-45): solo se produce APROBADA.
const CancelacionAprobada EstadoCancelacion = "APROBADA"

// EstadoTraslado es el ciclo del traslado (dominio.md §10).
type EstadoTraslado string

const (
	TrasladoProgramado EstadoTraslado = "PROGRAMADO"
	TrasladoEnCurso    EstadoTraslado = "EN_CURSO"
	TrasladoCompletado EstadoTraslado = "COMPLETADO"
	TrasladoCancelado  EstadoTraslado = "CANCELADO"
	// ASIGNADO, NO_REALIZADO e INTERRUMPIDO siguen en el contrato público, pero ningún método
	// los produce en esta etapa: no hay recorridos ni ejecución del chofer (ADR-024).
	TrasladoAsignado     EstadoTraslado = "ASIGNADO"
	TrasladoNoRealizado  EstadoTraslado = "NO_REALIZADO"
	TrasladoInterrumpido EstadoTraslado = "INTERRUMPIDO"
)

// Extremo indica qué punta del traslado es el aeropuerto (regla de clima R-46).
type Extremo string

const (
	ExtremoOrigen  Extremo = "ORIGEN"
	ExtremoDestino Extremo = "DESTINO"
)

// EstadoAlerta es el resultado de la regla de clima cuando hay algo que avisar.
type EstadoAlerta string

const (
	AlertaNoOperable   EstadoAlerta = "NO_OPERABLE"   // el aeropuerto no es operable
	AlertaNoVerificado EstadoAlerta = "NO_VERIFICADO" // el servicio de clima no respondió
)

// Evento es el tipo de evento que una transición pide publicar (docs/interfaces-internas.md §5.4).
type Evento string

const (
	EventoAceptada              Evento = "operaciones.solicitud.aceptada"
	EventoCancelacionRegistrada Evento = "operaciones.cancelacion.registrada"
	EventoTrasladoEstado        Evento = "operaciones.traslado.estado_cambiado"
	EventoAlertaClimatica       Evento = "operaciones.traslado.alerta_climatica"
)

// Cosa es lo que se traslada (contrato genérico, ADR-006: descripción y cantidad obligatorias).
type Cosa struct {
	Descripcion string
	Cantidad    int
	PesoKg      *float64
	VolumenM3   *float64
}

// peso es el peso informado, o 0 si no se informó (ocupa un lugar de la franja igual).
func (c Cosa) peso() float64 {
	if c.PesoKg == nil {
		return 0
	}
	return *c.PesoKg
}

// Ubicacion es el origen o el destino de un traslado.
type Ubicacion struct {
	Nombre    string
	Direccion string
	Contacto  *string
}

// Cancelacion es la cancelación de un traslado ya aceptado. Tardia indica que se pidió con menos
// de 24 h de anticipación.
type Cancelacion struct {
	Estado       EstadoCancelacion
	Motivo       string
	SolicitadaEn time.Time
	Tardia       bool
}

// AlertaClimatica es la marca que deja la regla de clima en un traslado al aeropuerto.
type AlertaClimatica struct {
	Estado      EstadoAlerta
	Motivos     []string
	ObservadoEn *time.Time
}

// Igual indica si las dos alertas dicen lo mismo: mismo estado y mismos motivos (la hora de
// observación no cuenta). Con esto, un evento reentregado o una consulta repetida no vuelve a
// escribir ni a publicar la misma alerta (idempotencia, teórico 4).
func (a *AlertaClimatica) Igual(otra *AlertaClimatica) bool {
	if a == nil || otra == nil {
		return a == otra
	}
	return a.Estado == otra.Estado && slices.Equal(a.Motivos, otra.Motivos)
}

// Operabilidad es lo que Rumbo necesita saber del clima del aeropuerto, ya traducido del
// contrato del proveedor (el dominio no conoce su JSON).
type Operabilidad struct {
	Operable    bool
	Motivos     []string
	ObservadoEn time.Time
}
