package models

import "time"

// SolicitudExterna es el documento de la colección solicitudes_externas. Guarda en un solo
// documento la solicitud, su clave de idempotencia, la copia del estado que informa Operaciones
// y la marca de pendiente de envío: MongoDB es atómico por documento, así que la intención de
// avisar a Operaciones nunca se pierde aunque el broker esté caído (equivale al outbox, ADR-003).
type SolicitudExterna struct {
	ID             string            `bson:"_id" json:"id"` // "sol-<uuid>"
	OrganizacionID string            `bson:"organizacion_id" json:"organizacion_id"`
	Pedido         PedidoTraslado    `bson:"pedido" json:"pedido"`             // lo que mandó el consumidor
	Idempotencia   ClaveIdempotencia `bson:"idempotencia" json:"idempotencia"` // clave + hash del cuerpo + respuesta original
	Vista          VistaSolicitud    `bson:"vista" json:"vista"`               // estado tal como lo informa el contrato
	Version        int64             `bson:"version" json:"version"`           // 0 al crear; luego la agregado_version del último evento aplicado
	PendienteEnvio bool              `bson:"pendiente_envio" json:"pendiente_envio"`
	ReclamadoHasta *time.Time        `bson:"reclamado_hasta,omitempty" json:"reclamado_hasta,omitempty"`
	ReclamadoPor   string            `bson:"reclamado_por,omitempty" json:"reclamado_por,omitempty"`
	ContextoTraza  map[string]string `bson:"contexto_traza,omitempty" json:"contexto_traza,omitempty"` // propagation.MapCarrier (traceparent)
	CreadoEn       time.Time         `bson:"creado_en" json:"creado_en"`
	ActualizadoEn  time.Time         `bson:"actualizado_en" json:"actualizado_en"`
}

// ClaveIdempotencia es lo que se conserva de la Idempotency-Key (teórico 7): la clave, el hash
// del cuerpo normalizado y la respuesta original, para devolver lo mismo ante un reintento.
type ClaveIdempotencia struct {
	Clave      string         `bson:"clave" json:"clave"`
	HashCuerpo string         `bson:"hash_cuerpo" json:"hash_cuerpo"` // SHA-256 hexadecimal
	Respuesta  VistaSolicitud `bson:"respuesta" json:"respuesta"`
}

// zonaLocal es la zona en la que se informan las horas en JSON. Se guarda en UTC.
var zonaLocal = cargarZona()

func cargarZona() *time.Location {
	if z, err := time.LoadLocation("America/Argentina/Cordoba"); err == nil {
		return z
	}
	return time.FixedZone("-03", -3*60*60)
}

// HoraLocal pasa una hora a la zona de Córdoba (-03:00) para responderla en JSON.
func HoraLocal(t time.Time) time.Time { return t.In(zonaLocal) }

// HoraLocalPtr hace lo mismo con una hora opcional.
func HoraLocalPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	local := HoraLocal(*t)
	return &local
}
