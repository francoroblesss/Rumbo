package models

import "time"

// Tipos del contrato público "Solicitar traslado" v2 (docs/contracts/traslados-v2.yaml).
// Los nombres JSON son exactamente los del contrato; los campos opcionales son punteros con
// omitempty, así "no vino" se distingue de "vino vacío" y no se responde nada que el contrato
// no declare.

// Estados de la solicitud que informa el contrato (EstadoSolicitud).
const (
	EstadoRecibida            = "RECIBIDA"
	EstadoContrapropuesta     = "CONTRAPROPUESTA"
	EstadoAceptada            = "ACEPTADA"
	EstadoRechazada           = "RECHAZADA"
	EstadoRechazadaPorCliente = "RECHAZADA_POR_CLIENTE"
	EstadoCancelada           = "CANCELADA"
)

// EstadosSolicitud lista los estados válidos para filtrar el listado.
var EstadosSolicitud = []string{EstadoRecibida, EstadoContrapropuesta, EstadoAceptada,
	EstadoRechazada, EstadoRechazadaPorCliente, EstadoCancelada}

// Ubicacion es un origen o un destino.
type Ubicacion struct {
	Nombre    string  `json:"nombre" bson:"nombre"`
	Direccion string  `json:"direccion" bson:"direccion"`
	Contacto  *string `json:"contacto,omitempty" bson:"contacto,omitempty"`
}

// Cosa es lo que hay que trasladar.
type Cosa struct {
	Descripcion string   `json:"descripcion" bson:"descripcion"`
	Cantidad    int      `json:"cantidad" bson:"cantidad"`
	PesoKg      *float64 `json:"peso_kg,omitempty" bson:"peso_kg,omitempty"`
	VolumenM3   *float64 `json:"volumen_m3,omitempty" bson:"volumen_m3,omitempty"`
}

// PedidoTraslado es el cuerpo de POST /v2/traslados (NuevoTraslado del contrato).
type PedidoTraslado struct {
	ReferenciaExterna     *string    `json:"referencia_externa,omitempty" bson:"referencia_externa,omitempty"`
	Origen                Ubicacion  `json:"origen" bson:"origen"`
	Destino               Ubicacion  `json:"destino" bson:"destino"`
	HoraLlegadaSolicitada *time.Time `json:"hora_llegada_solicitada,omitempty" bson:"hora_llegada_solicitada,omitempty"`
	ToleranciaMinutos     *int       `json:"tolerancia_minutos,omitempty" bson:"tolerancia_minutos,omitempty"`
	Cosa                  Cosa       `json:"cosa" bson:"cosa"`
	Observaciones         *string    `json:"observaciones,omitempty" bson:"observaciones,omitempty"`
}

// Contrapropuesta es el horario que propone Rumbo y su vencimiento.
type Contrapropuesta struct {
	HoraLlegadaPropuesta time.Time `json:"hora_llegada_propuesta" bson:"hora_llegada_propuesta"`
	VenceEn              time.Time `json:"vence_en" bson:"vence_en"`
}

// AlertaClimatica es un campo interno de la vista (interfaces-internas §6): el contrato público
// no lo informa.
type AlertaClimatica struct {
	Estado      string     `json:"estado" bson:"estado"`
	Motivos     []string   `json:"motivos,omitempty" bson:"motivos,omitempty"`
	ObservadoEn *time.Time `json:"observado_en,omitempty" bson:"observado_en,omitempty"`
}

// Traslado existe cuando la solicitud fue aceptada.
type Traslado struct {
	ID                      string           `json:"id" bson:"id"`
	Estado                  string           `json:"estado" bson:"estado"`
	HoraLlegadaComprometida time.Time        `json:"hora_llegada_comprometida" bson:"hora_llegada_comprometida"`
	DemoraMinutos           *int             `json:"demora_minutos,omitempty" bson:"demora_minutos,omitempty"`
	FinalizadoEn            *time.Time       `json:"finalizado_en,omitempty" bson:"finalizado_en,omitempty"`
	DetalleResultado        *string          `json:"detalle_resultado,omitempty" bson:"detalle_resultado,omitempty"`
	AlertaClimatica         *AlertaClimatica `json:"alerta_climatica,omitempty" bson:"alerta_climatica,omitempty"` // interno
}

// Cancelacion registra un pedido de cancelación de un traslado aceptado.
type Cancelacion struct {
	Estado       string    `json:"estado" bson:"estado"`
	Motivo       *string   `json:"motivo,omitempty" bson:"motivo,omitempty"`
	SolicitadaEn time.Time `json:"solicitada_en" bson:"solicitada_en"`
}

// VistaSolicitud es la solicitud tal como la informa Operaciones (interfaces-internas §6): la
// forma de SolicitudTraslado del contrato más los campos internos version y
// traslado.alerta_climatica. El controlador responde al consumidor solo los campos del contrato.
type VistaSolicitud struct {
	ID                    string           `json:"id" bson:"id"`
	ReferenciaExterna     *string          `json:"referencia_externa,omitempty" bson:"referencia_externa,omitempty"`
	Estado                string           `json:"estado" bson:"estado"`
	Cosa                  Cosa             `json:"cosa" bson:"cosa"`
	Origen                Ubicacion        `json:"origen" bson:"origen"`
	Destino               Ubicacion        `json:"destino" bson:"destino"`
	HoraLlegadaSolicitada *time.Time       `json:"hora_llegada_solicitada,omitempty" bson:"hora_llegada_solicitada,omitempty"`
	ToleranciaMinutos     int              `json:"tolerancia_minutos" bson:"tolerancia_minutos"`
	Observaciones         *string          `json:"observaciones,omitempty" bson:"observaciones,omitempty"`
	Contrapropuesta       *Contrapropuesta `json:"contrapropuesta,omitempty" bson:"contrapropuesta,omitempty"`
	MotivoRechazo         *string          `json:"motivo_rechazo,omitempty" bson:"motivo_rechazo,omitempty"`
	Traslado              *Traslado        `json:"traslado,omitempty" bson:"traslado,omitempty"`
	Cancelacion           *Cancelacion     `json:"cancelacion,omitempty" bson:"cancelacion,omitempty"`
	Version               int64            `json:"version" bson:"version"` // interno
	CreadoEn              time.Time        `json:"creado_en" bson:"creado_en"`
	ActualizadoEn         time.Time        `json:"actualizado_en" bson:"actualizado_en"`
}

// PaginaSolicitudes es la respuesta de GET /v2/traslados.
type PaginaSolicitudes struct {
	Resultados []VistaSolicitud `json:"resultados"`
	Pagina     int              `json:"pagina"`
	Tamanio    int              `json:"tamanio"`
	Total      int64            `json:"total"`
}

// RespuestaContrapropuesta es el cuerpo de POST /v2/traslados/{id}/respuesta-contrapropuesta.
// Acepta es puntero para distinguir "false" de "no vino" (es obligatorio).
type RespuestaContrapropuesta struct {
	Acepta *bool `json:"acepta"`
}

// PedidoCancelacion es el cuerpo de POST /v2/traslados/{id}/cancelacion.
type PedidoCancelacion struct {
	Motivo string `json:"motivo"`
}
