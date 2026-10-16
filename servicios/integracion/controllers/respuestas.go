package controllers

import (
	"time"

	"github.com/francoroblesss/Rumbo/servicios/integracion/models"
)

// Tipos de respuesta con SOLO los campos del contrato público (SolicitudTraslado). La vista
// interna trae además version y traslado.alerta_climatica, que el consumidor no recibe. Las
// horas se informan en RFC 3339 con zona -03:00.

type trasladoContrato struct {
	ID                      string     `json:"id"`
	Estado                  string     `json:"estado"`
	HoraLlegadaComprometida time.Time  `json:"hora_llegada_comprometida"`
	DemoraMinutos           *int       `json:"demora_minutos,omitempty"`
	FinalizadoEn            *time.Time `json:"finalizado_en,omitempty"`
	DetalleResultado        *string    `json:"detalle_resultado,omitempty"`
}

type solicitudContrato struct {
	ID                    string                  `json:"id"`
	ReferenciaExterna     *string                 `json:"referencia_externa,omitempty"`
	Estado                string                  `json:"estado"`
	Origen                models.Ubicacion        `json:"origen"`
	Destino               models.Ubicacion        `json:"destino"`
	HoraLlegadaSolicitada *time.Time              `json:"hora_llegada_solicitada,omitempty"`
	ToleranciaMinutos     int                     `json:"tolerancia_minutos"`
	Cosa                  models.Cosa             `json:"cosa"`
	Observaciones         *string                 `json:"observaciones,omitempty"`
	Contrapropuesta       *models.Contrapropuesta `json:"contrapropuesta,omitempty"`
	MotivoRechazo         *string                 `json:"motivo_rechazo,omitempty"`
	Traslado              *trasladoContrato       `json:"traslado,omitempty"`
	Cancelacion           *models.Cancelacion     `json:"cancelacion,omitempty"`
	CreadoEn              time.Time               `json:"creado_en"`
	ActualizadoEn         time.Time               `json:"actualizado_en"`
}

type paginaContrato struct {
	Resultados []solicitudContrato `json:"resultados"`
	Pagina     int                 `json:"pagina"`
	Tamanio    int                 `json:"tamanio"`
	Total      int64               `json:"total"`
}

func aContrato(v models.VistaSolicitud) solicitudContrato {
	r := solicitudContrato{
		ID:                    v.ID,
		ReferenciaExterna:     v.ReferenciaExterna,
		Estado:                v.Estado,
		Origen:                v.Origen,
		Destino:               v.Destino,
		HoraLlegadaSolicitada: models.HoraLocalPtr(v.HoraLlegadaSolicitada),
		ToleranciaMinutos:     v.ToleranciaMinutos,
		Cosa:                  v.Cosa,
		Observaciones:         v.Observaciones,
		MotivoRechazo:         v.MotivoRechazo,
		CreadoEn:              models.HoraLocal(v.CreadoEn),
		ActualizadoEn:         models.HoraLocal(v.ActualizadoEn),
	}
	if c := v.Contrapropuesta; c != nil {
		r.Contrapropuesta = &models.Contrapropuesta{
			HoraLlegadaPropuesta: models.HoraLocal(c.HoraLlegadaPropuesta),
			VenceEn:              models.HoraLocal(c.VenceEn),
		}
	}
	if t := v.Traslado; t != nil {
		r.Traslado = &trasladoContrato{
			ID:                      t.ID,
			Estado:                  t.Estado,
			HoraLlegadaComprometida: models.HoraLocal(t.HoraLlegadaComprometida),
			DemoraMinutos:           t.DemoraMinutos,
			FinalizadoEn:            models.HoraLocalPtr(t.FinalizadoEn),
			DetalleResultado:        t.DetalleResultado,
		}
	}
	if c := v.Cancelacion; c != nil {
		r.Cancelacion = &models.Cancelacion{Estado: c.Estado, Motivo: c.Motivo, SolicitadaEn: models.HoraLocal(c.SolicitadaEn)}
	}
	return r
}

func paginaAContrato(p models.PaginaSolicitudes) paginaContrato {
	r := paginaContrato{Resultados: make([]solicitudContrato, 0, len(p.Resultados)), Pagina: p.Pagina, Tamanio: p.Tamanio, Total: p.Total}
	for _, v := range p.Resultados {
		r.Resultados = append(r.Resultados, aContrato(v))
	}
	return r
}
