package dominio

import "time"

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
	SolicitudRecibida            EstadoSolicitud = "RECIBIDA"
	SolicitudContrapropuesta     EstadoSolicitud = "CONTRAPROPUESTA"
	SolicitudAceptada            EstadoSolicitud = "ACEPTADA"
	SolicitudRechazada           EstadoSolicitud = "RECHAZADA"
	SolicitudRechazadaPorCliente EstadoSolicitud = "RECHAZADA_POR_CLIENTE"
	SolicitudCancelada           EstadoSolicitud = "CANCELADA"
)

// EstadoCancelacion es el estado de un pedido de cancelación de un traslado ya aceptado.
type EstadoCancelacion string

const (
	CancelacionPendienteAprobacion EstadoCancelacion = "PENDIENTE_APROBACION"
	CancelacionAprobada            EstadoCancelacion = "APROBADA"
	CancelacionRechazada           EstadoCancelacion = "RECHAZADA"
)

// EstadoTraslado es el ciclo del traslado (dominio.md §10).
type EstadoTraslado string

const (
	TrasladoProgramado   EstadoTraslado = "PROGRAMADO"
	TrasladoAsignado     EstadoTraslado = "ASIGNADO"
	TrasladoEnCurso      EstadoTraslado = "EN_CURSO"
	TrasladoCompletado   EstadoTraslado = "COMPLETADO"
	TrasladoCancelado    EstadoTraslado = "CANCELADO"
	TrasladoNoRealizado  EstadoTraslado = "NO_REALIZADO"
	TrasladoInterrumpido EstadoTraslado = "INTERRUMPIDO"
)

// Extremo indica qué punta del traslado es el aeropuerto (regla de clima R-41).
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
	EventoRecibida              Evento = "operaciones.solicitud.recibida"
	EventoAceptada              Evento = "operaciones.solicitud.aceptada"
	EventoContrapropuesta       Evento = "operaciones.solicitud.contrapropuesta"
	EventoRechazada             Evento = "operaciones.solicitud.rechazada"
	EventoRechazadaPorCliente   Evento = "operaciones.solicitud.rechazada_por_cliente"
	EventoVencida               Evento = "operaciones.solicitud.vencida"
	EventoCancelada             Evento = "operaciones.solicitud.cancelada"
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

// Ubicacion es el origen o el destino de un traslado.
type Ubicacion struct {
	Nombre    string
	Direccion string
	Contacto  *string
}

// Contrapropuesta es el horario que propone Rumbo y hasta cuándo puede responder el cliente.
type Contrapropuesta struct {
	Hora    time.Time
	VenceEn time.Time
}

// Cancelacion es el pedido de cancelación de un traslado ya aceptado.
type Cancelacion struct {
	Estado       EstadoCancelacion
	Motivo       string
	SolicitadaEn time.Time
}

// AlertaClimatica es la marca que deja la regla de clima en un traslado al aeropuerto.
type AlertaClimatica struct {
	Estado      EstadoAlerta
	Motivos     []string
	ObservadoEn *time.Time
}

// Operabilidad es lo que Rumbo necesita saber del clima del aeropuerto, ya traducido del
// contrato del proveedor (el dominio no conoce su JSON).
type Operabilidad struct {
	Operable    bool
	Motivos     []string
	ObservadoEn time.Time
}
