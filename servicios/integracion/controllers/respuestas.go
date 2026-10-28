package controllers

import (
	"time"

	"github.com/francoroblesss/Rumbo/servicios/integracion/models"
)

// Tipos de respuesta con SOLO los campos del contrato público (TrasladoReservado). El consumidor
// ve el traslado que reservó, no la solicitud interna de Operaciones: la vista trae además el
// estado de la solicitud, version y traslado.alerta_climatica, que no se responden. Las horas se
// informan en RFC 3339 con zona -03:00.

type franjaContrato struct {
	Inicio time.Time `json:"inicio"`
	Fin    time.Time `json:"fin"`
}

type cancelacionContrato struct {
	Motivo      *string   `json:"motivo,omitempty"`
	Tardia      bool      `json:"tardia"`
	CanceladaEn time.Time `json:"cancelada_en"`
}

type trasladoContrato struct {
	ID                string               `json:"id"`
	ReferenciaExterna *string              `json:"referencia_externa,omitempty"`
	Estado            string               `json:"estado"`
	Cosa              models.Cosa          `json:"cosa"`
	Origen            models.Ubicacion     `json:"origen"`
	Destino           models.Ubicacion     `json:"destino"`
	Observaciones     *string              `json:"observaciones,omitempty"`
	Franja            franjaContrato       `json:"franja"`
	DemoraMinutos     *int                 `json:"demora_minutos,omitempty"`
	FinalizadoEn      *time.Time           `json:"finalizado_en,omitempty"`
	DetalleResultado  *string              `json:"detalle_resultado,omitempty"`
	Cancelacion       *cancelacionContrato `json:"cancelacion,omitempty"`
	CreadoEn          time.Time            `json:"creado_en"`
	ActualizadoEn     time.Time            `json:"actualizado_en"`
}

type paginaContrato struct {
	Resultados []trasladoContrato `json:"resultados"`
	Pagina     int                `json:"pagina"`
	Tamanio    int                `json:"tamanio"`
	Total      int64              `json:"total"`
}

func aContrato(v models.VistaSolicitud) trasladoContrato {
	r := trasladoContrato{
		ID:                v.ID,
		ReferenciaExterna: v.ReferenciaExterna,
		Estado:            models.EstadoProgramado,
		Cosa:              v.Cosa,
		Origen:            v.Origen,
		Destino:           v.Destino,
		Observaciones:     v.Observaciones,
		CreadoEn:          models.HoraLocal(v.CreadoEn),
		ActualizadoEn:     models.HoraLocal(v.ActualizadoEn),
	}
	if t := v.Traslado; t != nil {
		r.Estado = t.Estado
		r.DemoraMinutos = t.DemoraMinutos
		r.FinalizadoEn = models.HoraLocalPtr(t.FinalizadoEn)
		r.DetalleResultado = t.DetalleResultado
		r.Franja = franjaContrato{Inicio: models.HoraLocal(t.HoraLlegadaComprometida), Fin: models.HoraLocal(t.HoraLlegadaComprometida)}
		if f := t.Franja; f != nil {
			r.Franja = franjaContrato{Inicio: models.HoraLocal(f.Inicio), Fin: models.HoraLocal(f.Fin)}
		}
	}
	if c := v.Cancelacion; c != nil {
		r.Cancelacion = &cancelacionContrato{Motivo: c.Motivo, Tardia: c.Tardia, CanceladaEn: models.HoraLocal(c.SolicitadaEn)}
	}
	return r
}

func paginaAContrato(p models.PaginaSolicitudes) paginaContrato {
	r := paginaContrato{Resultados: make([]trasladoContrato, 0, len(p.Resultados)), Pagina: p.Pagina, Tamanio: p.Tamanio, Total: p.Total}
	for _, v := range p.Resultados {
		r.Resultados = append(r.Resultados, aContrato(v))
	}
	return r
}

func disponibilidadAContrato(d models.Disponibilidad) models.Disponibilidad {
	r := models.Disponibilidad{Fecha: d.Fecha, Franjas: make([]models.FranjaDisponible, 0, len(d.Franjas))}
	for _, f := range d.Franjas {
		f.Inicio, f.Fin = models.HoraLocal(f.Inicio), models.HoraLocal(f.Fin)
		r.Franjas = append(r.Franjas, f)
	}
	return r
}
