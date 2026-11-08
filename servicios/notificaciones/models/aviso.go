package models

import "time"

// Tipos de destinatario de un aviso (docs/interfaces-internas.md §14). Ningún aviso es para un
// usuario puntual: va a un rol o a una organización, así Notificaciones no necesita conocer la
// lista de usuarios (ADR-027).
const (
	DestinatarioRol          = "ROL"
	DestinatarioOrganizacion = "ORGANIZACION"
)

// EntidadSolicitud es el tipo de entidad de los avisos de esta etapa.
const EntidadSolicitud = "solicitud"

// Destinatario es a quién va dirigido el aviso: ROL (valor = nombre del rol) u ORGANIZACION
// (valor = id de la organización).
type Destinatario struct {
	Tipo  string `bson:"tipo" json:"tipo"`
	Valor string `bson:"valor" json:"valor"`
}

// Entidad es el objeto del que habla el aviso (la web lo usa para abrir el detalle).
type Entidad struct {
	Tipo string `bson:"tipo" json:"tipo"`
	ID   string `bson:"id" json:"id"`
}

// Aviso es el documento de la colección avisos. El _id deriva del id del mensaje que lo originó:
// la unicidad del _id es la idempotencia del consumidor (teórico 4, restricción única).
type Aviso struct {
	ID           string       `bson:"_id" json:"id"`
	Destinatario Destinatario `bson:"destinatario" json:"destinatario"`
	Titulo       string       `bson:"titulo" json:"titulo"`
	Mensaje      string       `bson:"mensaje" json:"mensaje"`
	Entidad      Entidad      `bson:"entidad" json:"entidad"`
	CreadoEn     time.Time    `bson:"creado_en" json:"creado_en"`
	// Leido es compartido por el destinatario: si un operador lo marca, queda marcado para todos
	// los operadores (ADR-027).
	Leido bool `bson:"leido" json:"leido"`
}

// PaginaAvisos es una página de GET /avisos (§11.1).
type PaginaAvisos struct {
	Resultados []Aviso `json:"resultados"`
	Pagina     int     `json:"pagina"`
	Tamanio    int     `json:"tamanio"`
	Total      int64   `json:"total"`
}

// ZonaLocal es la zona en la que se informan las horas (JSON y textos de los avisos). Se guarda
// en UTC.
var ZonaLocal = cargarZona()

func cargarZona() *time.Location {
	if z, err := time.LoadLocation("America/Argentina/Cordoba"); err == nil {
		return z
	}
	return time.FixedZone("-03", -3*60*60)
}

// ParaRespuesta devuelve el aviso con creado_en en la zona de Córdoba (-03:00), para responderlo
// en JSON.
func (a Aviso) ParaRespuesta() Aviso {
	a.CreadoEn = a.CreadoEn.In(ZonaLocal)
	return a
}
