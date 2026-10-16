package dominio

import (
	"strings"
	"time"
)

// PlazoContrapropuesta es cuánto tiene el cliente para responder una contrapropuesta (R-08).
const PlazoContrapropuesta = 24 * time.Hour

// PlazoCancelacion es la anticipación mínima para cancelar sin aprobación del operador (R-15/R-16).
const PlazoCancelacion = 24 * time.Hour

// Solicitud es el agregado raíz de Operaciones (DDD, teórico 8): protege las invariantes del
// ciclo del pedido y de su traslado. Una transacción cambia una sola solicitud.
//
// Cada transición que se aplica incrementa Version (docs/interfaces-internas.md §5.1: la
// agregado_version) y devuelve el tipo de evento a publicar. El tiempo llega siempre como
// parámetro (ahora), así las reglas se prueban sin reloj real.
type Solicitud struct {
	ID                    string
	OrganizacionID        string
	Origen                OrigenSolicitud
	ReferenciaExterna     *string
	Cosa                  Cosa
	Desde                 Ubicacion
	Hasta                 Ubicacion
	HoraLlegadaSolicitada *time.Time
	ToleranciaMinutos     int
	Observaciones         *string
	Estado                EstadoSolicitud
	Contrapropuesta       *Contrapropuesta
	MotivoRechazo         *string
	Traslado              *Traslado
	Cancelacion           *Cancelacion
	Version               int64
	CreadoEn              time.Time
	ActualizadoEn         time.Time
}

// DatosSolicitud son los datos del pedido tal como llegan del cliente.
type DatosSolicitud struct {
	ReferenciaExterna     *string
	Cosa                  Cosa
	Desde                 Ubicacion
	Hasta                 Ubicacion
	HoraLlegadaSolicitada *time.Time
	ToleranciaMinutos     int
	Observaciones         *string
}

// NuevaSolicitudExterna registra una solicitud que llegó por la capacidad publicada, con el mismo
// id que le dio Integración de carga.
func NuevaSolicitudExterna(id, organizacionID string, d DatosSolicitud, ahora time.Time) (*Solicitud, Evento, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(organizacionID) == "" ||
		strings.TrimSpace(d.Cosa.Descripcion) == "" || d.Cosa.Cantidad < 1 ||
		!d.Desde.completa() || !d.Hasta.completa() {
		return nil, "", ErrDatosInvalidos
	}
	return &Solicitud{
		ID:                    id,
		OrganizacionID:        organizacionID,
		Origen:                OrigenExterno,
		ReferenciaExterna:     d.ReferenciaExterna,
		Cosa:                  d.Cosa,
		Desde:                 d.Desde,
		Hasta:                 d.Hasta,
		HoraLlegadaSolicitada: d.HoraLlegadaSolicitada,
		ToleranciaMinutos:     d.ToleranciaMinutos,
		Observaciones:         d.Observaciones,
		Estado:                SolicitudRecibida,
		Version:               1,
		CreadoEn:              ahora,
		ActualizadoEn:         ahora,
	}, EventoRecibida, nil
}

func (u Ubicacion) completa() bool {
	return strings.TrimSpace(u.Nombre) != "" && strings.TrimSpace(u.Direccion) != ""
}

// Aceptar confirma la hora pedida como compromiso y crea el traslado (R-06). Sin hora pedida no
// se puede aceptar: Rumbo nunca fija un horario por su cuenta, tiene que contraproponer (R-07).
// idTraslado lo genera quien llama (el dominio no depende de la librería de uuid).
func (s *Solicitud) Aceptar(idTraslado string, ahora time.Time, extremo *Extremo) (Evento, error) {
	if s.Estado != SolicitudRecibida {
		return "", ErrEstadoNoPermite
	}
	if s.HoraLlegadaSolicitada == nil {
		return "", ErrSinHoraPedida
	}
	if !s.HoraLlegadaSolicitada.After(ahora) {
		return "", ErrHorarioEnElPasado
	}
	if extremo != nil && *extremo != ExtremoOrigen && *extremo != ExtremoDestino {
		return "", ErrDatosInvalidos
	}
	s.Estado = SolicitudAceptada
	s.Traslado = &Traslado{
		ID:                      idTraslado,
		Estado:                  TrasladoProgramado,
		HoraLlegadaComprometida: *s.HoraLlegadaSolicitada,
		AeropuertoExtremo:       extremo,
	}
	s.tocar(ahora)
	return EventoAceptada, nil
}

// Contraproponer propone otro horario; el cliente tiene 24 h para responder (R-07, R-08).
func (s *Solicitud) Contraproponer(hora, ahora time.Time) (Evento, error) {
	if s.Estado != SolicitudRecibida {
		return "", ErrEstadoNoPermite
	}
	if !hora.After(ahora) {
		return "", ErrHorarioEnElPasado
	}
	s.Estado = SolicitudContrapropuesta
	s.Contrapropuesta = &Contrapropuesta{Hora: hora, VenceEn: ahora.Add(PlazoContrapropuesta)}
	s.tocar(ahora)
	return EventoContrapropuesta, nil
}

// Rechazar descarta la solicitud; el rechazo siempre lleva motivo (R-09).
func (s *Solicitud) Rechazar(motivo string, ahora time.Time) (Evento, error) {
	if s.Estado != SolicitudRecibida {
		return "", ErrEstadoNoPermite
	}
	if strings.TrimSpace(motivo) == "" {
		return "", ErrDatosInvalidos
	}
	s.Estado = SolicitudRechazada
	s.MotivoRechazo = &motivo
	s.tocar(ahora)
	return EventoRechazada, nil
}

// ResponderContrapropuesta registra la respuesta del cliente. Una contrapropuesta vencida no se
// puede responder; no se cambia nada acá porque la vence la tarea programada (Vencer).
func (s *Solicitud) ResponderContrapropuesta(acepta bool, idTraslado string, ahora time.Time) (Evento, error) {
	if s.Estado != SolicitudContrapropuesta || s.Contrapropuesta == nil {
		return "", ErrEstadoNoPermite
	}
	if !ahora.Before(s.Contrapropuesta.VenceEn) {
		return "", ErrContrapropuestaVencida
	}
	if !acepta {
		s.Estado = SolicitudRechazadaPorCliente
		s.tocar(ahora)
		return EventoRechazadaPorCliente, nil
	}
	s.Estado = SolicitudAceptada
	s.Traslado = &Traslado{
		ID:                      idTraslado,
		Estado:                  TrasladoProgramado,
		HoraLlegadaComprometida: s.Contrapropuesta.Hora,
	}
	s.tocar(ahora)
	return EventoAceptada, nil
}

// Vencer cierra una contrapropuesta sin respuesta a las 24 h y libera el horario (R-08).
func (s *Solicitud) Vencer(ahora time.Time) (Evento, error) {
	if s.Estado != SolicitudContrapropuesta || s.Contrapropuesta == nil || ahora.Before(s.Contrapropuesta.VenceEn) {
		return "", ErrEstadoNoPermite
	}
	s.Estado = SolicitudRechazadaPorCliente
	s.tocar(ahora)
	return EventoVencida, nil
}

// Cancelar retira la solicitud o pide cancelar su traslado (R-15, R-16, R-17):
//   - antes de aceptarse, la solicitud queda CANCELADA;
//   - aceptada, con 24 h o más de anticipación, el traslado se cancela y la cancelación queda
//     APROBADA; con menos, queda PENDIENTE_APROBACION para que decida el operador. En los dos
//     casos la cancelación queda registrada (dominio.md §9);
//   - un traslado iniciado o terminado no se cancela.
func (s *Solicitud) Cancelar(motivo string, ahora time.Time) (Evento, error) {
	if strings.TrimSpace(motivo) == "" {
		return "", ErrDatosInvalidos
	}
	switch s.Estado {
	case SolicitudRecibida, SolicitudContrapropuesta:
		s.Estado = SolicitudCancelada
		s.tocar(ahora)
		return EventoCancelada, nil
	case SolicitudAceptada:
		if s.Traslado == nil || !s.Traslado.cancelable() ||
			(s.Cancelacion != nil && s.Cancelacion.Estado == CancelacionPendienteAprobacion) {
			return "", ErrEstadoNoPermite
		}
		cancelacion := &Cancelacion{Estado: CancelacionPendienteAprobacion, Motivo: motivo, SolicitadaEn: ahora}
		if s.Traslado.HoraLlegadaComprometida.Sub(ahora) >= PlazoCancelacion {
			cancelacion.Estado = CancelacionAprobada
			s.Traslado.Estado = TrasladoCancelado
		}
		s.Cancelacion = cancelacion
		s.tocar(ahora)
		return EventoCancelacionRegistrada, nil
	default:
		return "", ErrEstadoNoPermite
	}
}

// ResolverCancelacionTardia es la decisión del operador sobre una cancelación con menos de 24 h.
func (s *Solicitud) ResolverCancelacionTardia(aprobar bool, ahora time.Time) (Evento, error) {
	if s.Cancelacion == nil || s.Cancelacion.Estado != CancelacionPendienteAprobacion || s.Traslado == nil {
		return "", ErrEstadoNoPermite
	}
	if !aprobar {
		s.Cancelacion.Estado = CancelacionRechazada
		s.tocar(ahora)
		return EventoCancelacionRegistrada, nil
	}
	s.Cancelacion.Estado = CancelacionAprobada
	s.Traslado.Estado = TrasladoCancelado
	s.tocar(ahora)
	return EventoTrasladoEstado, nil
}

// RegistrarAlertaClimatica deja en el traslado el resultado de la regla de clima (R-41). Rumbo no
// cancela ni reprograma: avisa y decide el operador.
func (s *Solicitud) RegistrarAlertaClimatica(alerta AlertaClimatica, ahora time.Time) (Evento, error) {
	if s.Traslado == nil {
		return "", ErrEstadoNoPermite
	}
	s.Traslado.Alerta = &alerta
	s.tocar(ahora)
	return EventoAlertaClimatica, nil
}

// RequiereConsultaClima indica si el traslado va o viene del aeropuerto y su hora comprometida
// cae dentro de las próximas 6 h (R-41).
func (s *Solicitud) RequiereConsultaClima(ahora time.Time) bool {
	return s.Traslado != nil && s.Traslado.AeropuertoExtremo != nil &&
		DentroDeVentanaClima(s.Traslado.HoraLlegadaComprometida, ahora)
}

// tocar registra que la transición se aplicó.
func (s *Solicitud) tocar(ahora time.Time) {
	s.Version++
	s.ActualizadoEn = ahora
}
