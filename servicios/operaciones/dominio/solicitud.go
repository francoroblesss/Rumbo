package dominio

import (
	"strings"
	"time"
)

// PlazoCancelacion es la anticipación por debajo de la cual una cancelación queda marcada como
// tardía (R-45).
const PlazoCancelacion = 24 * time.Hour

// Solicitud es el agregado raíz de Operaciones (DDD, teórico 8): protege las invariantes del
// ciclo del pedido y de su traslado. Una transacción cambia una sola solicitud.
//
// Cada transición que se aplica incrementa Version (docs/interfaces-internas.md §5.1: la
// agregado_version) y devuelve el tipo de evento a publicar. El tiempo llega siempre como
// parámetro (ahora), así las reglas se prueban sin reloj real.
//
// Desde ADR-024 las solicitudes nacen de una reserva de franja, ya aceptadas: no hay revisión
// del operador. HoraLlegadaSolicitada y MotivoRechazo quedan de la Entrega 2 y siempre vacíos.
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

// NuevaReservaExterna registra lo que pidió un sistema externo por la capacidad publicada: toma
// un lugar de la franja y deja la solicitud ya aceptada, con su traslado comprometido a llegar
// antes del fin de la franja. No hay revisión del operador ni contrapropuesta: la respuesta es
// definitiva en el momento (ADR-021). Si la franja no tiene lugar, no se crea nada.
// palabras son las de AEROPUERTO_PALABRAS: con ellas se marca qué extremo es el aeropuerto
// (R-46, ADR-025), sin llamadas externas.
func NuevaReservaExterna(id, idTraslado, organizacionID string, d DatosSolicitud, franja *Franja, palabras []string, ahora time.Time) (*Solicitud, Evento, error) {
	if err := validarDatos(id, organizacionID, d); err != nil {
		return nil, "", err
	}
	if err := franja.Reservar(d.Cosa.peso()); err != nil {
		return nil, "", err
	}
	inicio := franja.Inicio
	return &Solicitud{
		ID:                id,
		OrganizacionID:    organizacionID,
		Origen:            OrigenExterno,
		ReferenciaExterna: d.ReferenciaExterna,
		Cosa:              d.Cosa,
		Desde:             d.Desde,
		Hasta:             d.Hasta,
		Observaciones:     d.Observaciones,
		Estado:            SolicitudAceptada,
		Traslado: &Traslado{
			ID:                      idTraslado,
			Estado:                  TrasladoProgramado,
			HoraLlegadaComprometida: franja.Fin,
			FranjaInicio:            &inicio,
			AeropuertoExtremo:       ExtremoAeropuerto(d.Desde, d.Hasta, palabras),
		},
		Version:       1,
		CreadoEn:      ahora,
		ActualizadoEn: ahora,
	}, EventoAceptada, nil
}

func validarDatos(id, organizacionID string, d DatosSolicitud) error {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(organizacionID) == "" ||
		strings.TrimSpace(d.Cosa.Descripcion) == "" || d.Cosa.Cantidad < 1 ||
		(d.Cosa.PesoKg != nil && *d.Cosa.PesoKg <= 0) ||
		!d.Desde.completa() || !d.Hasta.completa() {
		return ErrDatosInvalidos
	}
	return nil
}

// LiberaFranja indica si el traslado reservó una franja que ahora hay que devolver, y cuánto peso.
func (s *Solicitud) LiberaFranja() (time.Time, float64, bool) {
	t := s.Traslado
	if t == nil || t.FranjaInicio == nil || t.Estado != TrasladoCancelado {
		return time.Time{}, 0, false
	}
	return *t.FranjaInicio, s.Cosa.peso(), true
}

func (u Ubicacion) completa() bool {
	return strings.TrimSpace(u.Nombre) != "" && strings.TrimSpace(u.Direccion) != ""
}

// Cancelar cancela el traslado de una reserva (R-45, R-17). Se aplica en el momento y la
// cancelación queda siempre APROBADA: nadie tiene que aprobarla (ADR-021). Si faltan menos de
// 24 h queda marcada como tardía, para que el operador se entere. Un traslado iniciado o
// terminado no se cancela.
func (s *Solicitud) Cancelar(motivo string, ahora time.Time) (Evento, error) {
	if strings.TrimSpace(motivo) == "" {
		return "", ErrDatosInvalidos
	}
	if s.Estado != SolicitudAceptada || s.Traslado == nil || !s.Traslado.cancelable() {
		return "", ErrEstadoNoPermite
	}
	s.Cancelacion = &Cancelacion{Estado: CancelacionAprobada, Motivo: motivo, SolicitadaEn: ahora,
		Tardia: s.Traslado.HoraLlegadaComprometida.Sub(ahora) < PlazoCancelacion}
	s.Traslado.Estado = TrasladoCancelado
	s.tocar(ahora)
	return EventoCancelacionRegistrada, nil
}

// IniciarTraslado marca que el traslado salió: PROGRAMADO → EN_CURSO (R-47). En esta etapa lo
// hace el operador desde la web, en lugar del chofer (ADR-024).
func (s *Solicitud) IniciarTraslado(ahora time.Time) (Evento, error) {
	if s.Traslado == nil || s.Traslado.Estado != TrasladoProgramado {
		return "", ErrEstadoNoPermite
	}
	s.Traslado.Estado = TrasladoEnCurso
	s.tocar(ahora)
	return EventoTrasladoEstado, nil
}

// CompletarTraslado marca que el traslado llegó: EN_CURSO → COMPLETADO (R-47).
func (s *Solicitud) CompletarTraslado(ahora time.Time) (Evento, error) {
	if s.Traslado == nil || s.Traslado.Estado != TrasladoEnCurso {
		return "", ErrEstadoNoPermite
	}
	s.Traslado.Estado = TrasladoCompletado
	finalizado := ahora
	s.Traslado.FinalizadoEn = &finalizado
	s.tocar(ahora)
	return EventoTrasladoEstado, nil
}

// EstadoPedido indica si el traslado ya está en el estado destino: repetir un avance que ya se
// hizo no cambia nada (R-47, reintento seguro).
func (s *Solicitud) EstadoPedido(destino EstadoTraslado) bool {
	return s.Traslado != nil && s.Traslado.Estado == destino
}

// RegistrarAlertaClimatica deja en el traslado el resultado de la regla de clima (R-46). Rumbo no
// cancela ni reprograma: avisa y decide el operador.
func (s *Solicitud) RegistrarAlertaClimatica(alerta AlertaClimatica, ahora time.Time) (Evento, error) {
	if s.Traslado == nil {
		return "", ErrEstadoNoPermite
	}
	s.Traslado.Alerta = &alerta
	s.tocar(ahora)
	return EventoAlertaClimatica, nil
}

// RequiereConsultaClima indica si corresponde consultar el clima (R-46): el traslado va o viene
// del aeropuerto, todavía no salió (PROGRAMADO) y su hora comprometida cae dentro de las
// próximas 6 h.
func (s *Solicitud) RequiereConsultaClima(ahora time.Time) bool {
	return s.Traslado != nil && s.Traslado.AeropuertoExtremo != nil && s.Traslado.Estado == TrasladoProgramado &&
		DentroDeVentanaClima(s.Traslado.HoraLlegadaComprometida, ahora)
}

// tocar registra que la transición se aplicó.
func (s *Solicitud) tocar(ahora time.Time) {
	s.Version++
	s.ActualizadoEn = ahora
}
