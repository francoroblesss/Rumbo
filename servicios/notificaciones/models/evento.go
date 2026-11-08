package models

import "time"

// Tipos de evento de Operaciones que generan avisos (docs/interfaces-internas.md §5.4 y §14).
const (
	EventoSolicitudAceptada     = "operaciones.solicitud.aceptada"
	EventoCancelacionRegistrada = "operaciones.cancelacion.registrada"
	EventoEstadoCambiado        = "operaciones.traslado.estado_cambiado"
	EventoAlertaClimatica       = "operaciones.traslado.alerta_climatica"
)

// Estados del traslado y de la alerta climática que usan las reglas de §14.
const (
	EstadoEnCurso      = "EN_CURSO"
	EstadoCompletado   = "COMPLETADO"
	AlertaNoOperable   = "NO_OPERABLE"
	AlertaNoVerificado = "NO_VERIFICADO"
)

// DatosEvento son los datos de los eventos de solicitudes y traslados (§5.4): la vista de la
// solicitud más la organización. Solo se leen los campos que necesitan los avisos; el resto de
// la vista (§6) se ignora.
type DatosEvento struct {
	OrganizacionID string         `json:"organizacion_id"`
	Solicitud      VistaSolicitud `json:"solicitud"`
}

// VistaSolicitud es el subconjunto de la vista de solicitud (§6) que usan los avisos.
type VistaSolicitud struct {
	ID          string       `json:"id"`
	Cosa        Cosa         `json:"cosa"`
	Destino     Ubicacion    `json:"destino"`
	Traslado    *Traslado    `json:"traslado"`
	Cancelacion *Cancelacion `json:"cancelacion"`
}

// Cosa es lo que se traslada.
type Cosa struct {
	Descripcion string `json:"descripcion"`
}

// Ubicacion es un origen o un destino.
type Ubicacion struct {
	Nombre string `json:"nombre"`
}

// Traslado es el traslado de la solicitud.
type Traslado struct {
	Estado          string           `json:"estado"`
	Franja          *Franja          `json:"franja"`
	AlertaClimatica *AlertaClimatica `json:"alerta_climatica"`
}

// Franja es la ventana reservada.
type Franja struct {
	Inicio time.Time `json:"inicio"`
	Fin    time.Time `json:"fin"`
}

// AlertaClimatica es la alerta que dejó la regla de clima (ADR-025).
type AlertaClimatica struct {
	Estado  string   `json:"estado"`
	Motivos []string `json:"motivos"`
}

// Cancelacion es la cancelación registrada. Tardia: se pidió con menos de 24 h.
type Cancelacion struct {
	Tardia bool `json:"tardia"`
}
