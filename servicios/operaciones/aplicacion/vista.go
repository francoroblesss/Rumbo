package aplicacion

import (
	"time"

	"github.com/francoroblesss/Rumbo/servicios/operaciones/dominio"
	"github.com/francoroblesss/Rumbo/servicios/operaciones/puertos"
)

// zonaRumbo es la hora en la que opera Rumbo: las respuestas y los eventos van con -03:00
// (docs/interfaces-internas.md §6). Se guarda en UTC.
var zonaRumbo = cargarZona()

func cargarZona() *time.Location {
	if z, err := time.LoadLocation("America/Argentina/Cordoba"); err == nil {
		return z
	}
	return time.FixedZone("-03", -3*60*60)
}

// ZonaRumbo es la zona horaria en la que opera Rumbo (main la usa para armar el calendario).
func ZonaRumbo() *time.Location { return zonaRumbo }

func hora(t time.Time) string { return t.In(zonaRumbo).Format(time.RFC3339) }

func horaOpcional(t *time.Time) *string {
	if t == nil {
		return nil
	}
	h := hora(*t)
	return &h
}

// NuevaVista traduce el agregado a la vista de solicitud (§6).
func NuevaVista(s *dominio.Solicitud) puertos.Vista {
	v := puertos.Vista{
		ID:                s.ID,
		ReferenciaExterna: s.ReferenciaExterna,
		Estado:            string(s.Estado),
		Cosa: puertos.VistaCosa{
			Descripcion: s.Cosa.Descripcion,
			Cantidad:    s.Cosa.Cantidad,
			PesoKg:      s.Cosa.PesoKg,
			VolumenM3:   s.Cosa.VolumenM3,
		},
		Origen:                vistaUbicacion(s.Desde),
		Destino:               vistaUbicacion(s.Hasta),
		HoraLlegadaSolicitada: horaOpcional(s.HoraLlegadaSolicitada),
		ToleranciaMinutos:     s.ToleranciaMinutos,
		Observaciones:         s.Observaciones,
		MotivoRechazo:         s.MotivoRechazo,
		Version:               s.Version,
		CreadoEn:              hora(s.CreadoEn),
		ActualizadoEn:         hora(s.ActualizadoEn),
	}
	if c := s.Contrapropuesta; c != nil {
		v.Contrapropuesta = &puertos.VistaContrapropuesta{HoraLlegadaPropuesta: hora(c.Hora), VenceEn: hora(c.VenceEn)}
	}
	if t := s.Traslado; t != nil {
		v.Traslado = &puertos.VistaTraslado{
			ID:                      t.ID,
			Estado:                  string(t.Estado),
			HoraLlegadaComprometida: hora(t.HoraLlegadaComprometida),
			DemoraMinutos:           t.DemoraMinutos,
			FinalizadoEn:            horaOpcional(t.FinalizadoEn),
			DetalleResultado:        t.DetalleResultado,
		}
		if t.FranjaInicio != nil {
			v.Traslado.Franja = &puertos.VistaFranja{Inicio: hora(*t.FranjaInicio), Fin: hora(t.HoraLlegadaComprometida)}
		}
		if a := t.Alerta; a != nil {
			motivos := a.Motivos
			if motivos == nil {
				motivos = []string{}
			}
			v.Traslado.AlertaClimatica = &puertos.VistaAlerta{Estado: string(a.Estado), Motivos: motivos, ObservadoEn: horaOpcional(a.ObservadoEn)}
		}
	}
	if c := s.Cancelacion; c != nil {
		v.Cancelacion = &puertos.VistaCancelacion{Estado: string(c.Estado), Motivo: c.Motivo, SolicitadaEn: hora(c.SolicitadaEn), Tardia: c.Tardia}
	}
	return v
}

func vistaUbicacion(u dominio.Ubicacion) puertos.VistaUbicacion {
	return puertos.VistaUbicacion{Nombre: u.Nombre, Direccion: u.Direccion, Contacto: u.Contacto}
}
