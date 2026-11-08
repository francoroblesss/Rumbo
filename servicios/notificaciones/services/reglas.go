package services

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/francoroblesss/Rumbo/servicios/comun/identidad"
	"github.com/francoroblesss/Rumbo/servicios/comun/mensajeria"
	"github.com/francoroblesss/Rumbo/servicios/notificaciones/models"
)

// avisosPara aplica la tabla de reglas de docs/interfaces-internas.md §14 a un evento de
// Operaciones y devuelve los avisos que genera, en el orden de la tabla. Es una función pura
// (sin MongoDB ni reloj): creado_en lo pone el servicio al guardarlos.
//   - Un tipo de evento sin regla, o un cambio a un estado sin aviso, devuelve (nil, nil): ACK sin aviso.
//   - Datos que no se pueden leer, o a los que les falta lo que usa el aviso, devuelven un error
//     permanente: el mensaje va a la DLQ sin reintentar (reintentar no lo arregla, teórico 4).
//   - El id de cada aviso deriva del id del mensaje: <id> si sale uno; <id>-1, <id>-2 si salen varios.
func avisosPara(sobre mensajeria.Sobre) ([]models.Aviso, error) {
	switch sobre.Tipo {
	case models.EventoSolicitudAceptada, models.EventoCancelacionRegistrada,
		models.EventoEstadoCambiado, models.EventoAlertaClimatica:
	default:
		return nil, nil
	}
	if sobre.ID == "" {
		return nil, ilegible(sobre, errors.New("el mensaje no tiene id"))
	}
	var datos models.DatosEvento
	if err := sobre.LeerDatos(&datos); err != nil {
		return nil, err // ya viene marcado como permanente
	}
	s := datos.Solicitud
	if s.ID == "" || s.Cosa.Descripcion == "" || s.Traslado == nil {
		return nil, ilegible(sobre, errors.New("falta solicitud.id, cosa.descripcion o traslado"))
	}
	operador := models.Destinatario{Tipo: models.DestinatarioRol, Valor: identidad.RolOperador}
	organizacion := models.Destinatario{Tipo: models.DestinatarioOrganizacion, Valor: datos.OrganizacionID}
	cosa := s.Cosa.Descripcion

	var avisos []models.Aviso
	agregar := func(d models.Destinatario, titulo, mensaje string) {
		avisos = append(avisos, models.Aviso{Destinatario: d, Titulo: titulo, Mensaje: mensaje,
			Entidad: models.Entidad{Tipo: models.EntidadSolicitud, ID: s.ID}})
	}

	switch sobre.Tipo {
	case models.EventoSolicitudAceptada:
		f := s.Traslado.Franja
		if f == nil {
			return nil, ilegible(sobre, errors.New("falta traslado.franja"))
		}
		agregar(operador, "Reserva nueva", fmt.Sprintf("Se reservó «%s» para la franja del %s de %s a %s.",
			cosa, dia(f), hora(f.Inicio), hora(f.Fin)))

	case models.EventoCancelacionRegistrada:
		f := s.Traslado.Franja
		if f == nil || s.Cancelacion == nil || datos.OrganizacionID == "" {
			return nil, ilegible(sobre, errors.New("falta traslado.franja, cancelacion u organizacion_id"))
		}
		if s.Cancelacion.Tardia {
			agregar(operador, "Cancelación tardía", fmt.Sprintf("Se canceló «%s» con menos de 24 h: era para la franja del %s de %s a %s.",
				cosa, dia(f), hora(f.Inicio), hora(f.Fin)))
		}
		agregar(organizacion, "Traslado cancelado", fmt.Sprintf("Se canceló el traslado de «%s» del %s.", cosa, dia(f)))

	case models.EventoAlertaClimatica:
		a := s.Traslado.AlertaClimatica
		if a == nil || datos.OrganizacionID == "" {
			return nil, ilegible(sobre, errors.New("falta traslado.alerta_climatica u organizacion_id"))
		}
		var mensaje string
		switch a.Estado {
		case models.AlertaNoOperable:
			mensaje = fmt.Sprintf("El aeropuerto no está operable para «%s» (%s).", cosa, strings.Join(a.Motivos, ", "))
		case models.AlertaNoVerificado:
			mensaje = fmt.Sprintf("No se pudo verificar el clima del aeropuerto para «%s».", cosa)
		default:
			return nil, ilegible(sobre, fmt.Errorf("estado de alerta desconocido %q", a.Estado))
		}
		agregar(operador, "Alerta climática", mensaje)
		agregar(organizacion, "Alerta climática", mensaje)

	case models.EventoEstadoCambiado:
		switch s.Traslado.Estado {
		case models.EstadoEnCurso:
			agregar(organizacion, "Traslado en curso", fmt.Sprintf("Salió el traslado de «%s».", cosa))
		case models.EstadoCompletado:
			agregar(organizacion, "Traslado completado", fmt.Sprintf("Llegó «%s» a %s.", cosa, s.Destino.Nombre))
		default:
			return nil, nil // otro estado: sin aviso
		}
		if datos.OrganizacionID == "" {
			return nil, ilegible(sobre, errors.New("falta organizacion_id"))
		}
	}

	if len(avisos) == 1 {
		avisos[0].ID = sobre.ID
		return avisos, nil
	}
	for i := range avisos {
		avisos[i].ID = fmt.Sprintf("%s-%d", sobre.ID, i+1)
	}
	return avisos, nil
}

func ilegible(sobre mensajeria.Sobre, err error) error {
	return mensajeria.Permanente(fmt.Errorf("datos inválidos en %s: %w", sobre.Tipo, err))
}

// dia y hora formatean la franja en hora de Argentina (dd/mm y HH:MM, §14).
func dia(f *models.Franja) string { return f.Inicio.In(models.ZonaLocal).Format("02/01") }

func hora(t time.Time) string { return t.In(models.ZonaLocal).Format("15:04") }
