package models

import "time"

// Tipos del contrato público "Reservar traslado" (docs/contracts/traslados.yaml, 4.0.0).
// Los nombres JSON son exactamente los del contrato; los campos opcionales son punteros con
// omitempty, así "no vino" se distingue de "vino vacío" y no se responde nada que el contrato
// no declare.

// Estados del traslado que informa el contrato (EstadoTraslado). Una reserva nace PROGRAMADO.
const (
	EstadoProgramado   = "PROGRAMADO"
	EstadoAsignado     = "ASIGNADO"
	EstadoEnCurso      = "EN_CURSO"
	EstadoCompletado   = "COMPLETADO"
	EstadoCancelado    = "CANCELADO"
	EstadoNoRealizado  = "NO_REALIZADO"
	EstadoInterrumpido = "INTERRUMPIDO"
)

// EstadosTraslado lista los estados válidos para filtrar el listado.
var EstadosTraslado = []string{EstadoProgramado, EstadoAsignado, EstadoEnCurso, EstadoCompletado,
	EstadoCancelado, EstadoNoRealizado, EstadoInterrumpido}

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

// PedidoTraslado es el cuerpo de POST /traslados (NuevoTraslado del contrato). Se reenvía tal
// cual a Operaciones (POST /internas/reservas).
type PedidoTraslado struct {
	ReferenciaExterna *string    `json:"referencia_externa,omitempty" bson:"referencia_externa,omitempty"`
	Cosa              Cosa       `json:"cosa" bson:"cosa"`
	Origen            Ubicacion  `json:"origen" bson:"origen"`
	Destino           Ubicacion  `json:"destino" bson:"destino"`
	FranjaInicio      *time.Time `json:"franja_inicio,omitempty" bson:"franja_inicio,omitempty"`
	Observaciones     *string    `json:"observaciones,omitempty" bson:"observaciones,omitempty"`
}

// Franja es la ventana de llegada comprometida.
type Franja struct {
	Inicio time.Time `json:"inicio" bson:"inicio"`
	Fin    time.Time `json:"fin" bson:"fin"`
}

// AlertaClimatica es un campo interno de la vista (interfaces-internas §6): el contrato público
// no lo informa.
type AlertaClimatica struct {
	Estado      string     `json:"estado" bson:"estado"`
	Motivos     []string   `json:"motivos,omitempty" bson:"motivos,omitempty"`
	ObservadoEn *time.Time `json:"observado_en,omitempty" bson:"observado_en,omitempty"`
}

// Traslado es el traslado comprometido de la solicitud (en una reserva, existe desde el inicio).
type Traslado struct {
	ID                      string           `json:"id" bson:"id"`
	Estado                  string           `json:"estado" bson:"estado"`
	HoraLlegadaComprometida time.Time        `json:"hora_llegada_comprometida" bson:"hora_llegada_comprometida"`
	Franja                  *Franja          `json:"franja,omitempty" bson:"franja,omitempty"`
	DemoraMinutos           *int             `json:"demora_minutos,omitempty" bson:"demora_minutos,omitempty"`
	FinalizadoEn            *time.Time       `json:"finalizado_en,omitempty" bson:"finalizado_en,omitempty"`
	DetalleResultado        *string          `json:"detalle_resultado,omitempty" bson:"detalle_resultado,omitempty"`
	AlertaClimatica         *AlertaClimatica `json:"alerta_climatica,omitempty" bson:"alerta_climatica,omitempty"` // interno
}

// Cancelacion registra la cancelación de un traslado. Tardia: se pidió con menos de 24 h.
type Cancelacion struct {
	Estado       string    `json:"estado" bson:"estado"`
	Motivo       *string   `json:"motivo,omitempty" bson:"motivo,omitempty"`
	SolicitadaEn time.Time `json:"solicitada_en" bson:"solicitada_en"`
	Tardia       bool      `json:"tardia" bson:"tardia"`
}

// VistaSolicitud es la solicitud tal como la informa Operaciones (interfaces-internas §6). El
// controlador la traduce al recurso del contrato (TrasladoReservado): el consumidor ve el
// traslado, no la solicitud interna.
type VistaSolicitud struct {
	ID                string       `json:"id" bson:"id"`
	ReferenciaExterna *string      `json:"referencia_externa,omitempty" bson:"referencia_externa,omitempty"`
	Estado            string       `json:"estado" bson:"estado"` // de la solicitud (interno)
	Cosa              Cosa         `json:"cosa" bson:"cosa"`
	Origen            Ubicacion    `json:"origen" bson:"origen"`
	Destino           Ubicacion    `json:"destino" bson:"destino"`
	Observaciones     *string      `json:"observaciones,omitempty" bson:"observaciones,omitempty"`
	Traslado          *Traslado    `json:"traslado,omitempty" bson:"traslado,omitempty"`
	Cancelacion       *Cancelacion `json:"cancelacion,omitempty" bson:"cancelacion,omitempty"`
	Version           int64        `json:"version" bson:"version"` // interno
	CreadoEn          time.Time    `json:"creado_en" bson:"creado_en"`
	ActualizadoEn     time.Time    `json:"actualizado_en" bson:"actualizado_en"`
}

// PaginaSolicitudes es una página del listado (GET /traslados).
type PaginaSolicitudes struct {
	Resultados []VistaSolicitud `json:"resultados"`
	Pagina     int              `json:"pagina"`
	Tamanio    int              `json:"tamanio"`
	Total      int64            `json:"total"`
}

// Disponibilidad es la respuesta de GET /disponibilidad: las franjas de la fecha que todavía se
// pueden reservar. Viene de Operaciones con la misma forma.
type Disponibilidad struct {
	Fecha   string             `json:"fecha"`
	Franjas []FranjaDisponible `json:"franjas"`
}

// FranjaDisponible dice si una carga del peso consultado entra en la franja y cuánto lugar queda.
type FranjaDisponible struct {
	Inicio               time.Time `json:"inicio"`
	Fin                  time.Time `json:"fin"`
	Disponible           bool      `json:"disponible"`
	TrasladosDisponibles int       `json:"traslados_disponibles"`
	KgDisponibles        float64   `json:"kg_disponibles"`
}

// PedidoCancelacion es el cuerpo de POST /traslados/{id}/cancelacion.
type PedidoCancelacion struct {
	Motivo string `json:"motivo"`
}
