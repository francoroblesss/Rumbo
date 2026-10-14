package aplicacion

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/francoroblesss/Rumbo/servicios/comun/mensajeria"
	"github.com/francoroblesss/Rumbo/servicios/operaciones/dominio"
	"github.com/francoroblesss/Rumbo/servicios/operaciones/puertos"
)

var ahora = time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)

type escenario struct {
	uow   *uowMemoria
	clima *climaFalso
	casos *Solicitudes
	reloj time.Time
}

func nuevoEscenario() *escenario {
	e := &escenario{uow: nuevaUoW(), clima: &climaFalso{operabilidad: dominio.Operabilidad{Operable: true, ObservadoEn: ahora}}, reloj: ahora}
	e.casos = NuevasSolicitudes(e.uow, e.clima, "SACO", func() time.Time { return e.reloj }, "test")
	return e
}

func comando(mensajeID, id string, hora *time.Time) puertos.ComandoSolicitudExterna {
	return puertos.ComandoSolicitudExterna{
		MensajeID: mensajeID, SolicitudID: id, OrganizacionID: "org-1",
		Datos: dominio.DatosSolicitud{
			Cosa:                  dominio.Cosa{Descripcion: "Cajas", Cantidad: 20},
			Desde:                 dominio.Ubicacion{Nombre: "Sucursal Centro", Direccion: "Av. Colón 1234"},
			Hasta:                 dominio.Ubicacion{Nombre: "Aeropuerto", Direccion: "Av. La Voz del Interior 8500"},
			HoraLlegadaSolicitada: hora,
		},
	}
}

func (e *escenario) registrar(t *testing.T, id string, hora *time.Time) {
	t.Helper()
	if err := e.casos.RegistrarExterna(context.Background(), comando("msg-"+id, id, hora)); err != nil {
		t.Fatalf("registrar: %v", err)
	}
}

func horaEn(d time.Duration) *time.Time {
	h := ahora.Add(d)
	return &h
}

// datosEvento es la forma de "datos" de los eventos de §5.4.
type datosEvento struct {
	OrganizacionID string        `json:"organizacion_id"`
	Origen         string        `json:"origen"`
	Solicitud      puertos.Vista `json:"solicitud"`
}

// verificarEvento revisa el mensaje i del outbox: exchange, tipo, versión y la vista.
func verificarEvento(t *testing.T, m mensajeOutbox, tipo dominio.Evento, version int64, estado string) datosEvento {
	t.Helper()
	if m.Exchange != ExchangeEventos || m.RoutingKey != string(tipo) || m.Sobre.Tipo != string(tipo) {
		t.Fatalf("mensaje inesperado: %s/%s (%s), se esperaba %s", m.Exchange, m.RoutingKey, m.Sobre.Tipo, tipo)
	}
	if m.Sobre.AgregadoVersion != version {
		t.Fatalf("%s: agregado_version = %d, se esperaba %d", tipo, m.Sobre.AgregadoVersion, version)
	}
	var d datosEvento
	if err := json.Unmarshal(m.Sobre.Datos, &d); err != nil {
		t.Fatal(err)
	}
	if d.OrganizacionID != "org-1" || d.Origen != "externo" || d.Solicitud.Estado != estado || d.Solicitud.Version != version {
		t.Fatalf("%s: datos inesperados: %+v", tipo, d)
	}
	if d.Solicitud.ID != m.Sobre.AgregadoID {
		t.Fatalf("agregado_id = %s, vista = %s", m.Sobre.AgregadoID, d.Solicitud.ID)
	}
	return d
}

func TestRegistrarExterna(t *testing.T) {
	e := nuevoEscenario()
	e.registrar(t, "sol-1", nil)

	outbox := e.uow.outbox()
	if len(outbox) != 1 {
		t.Fatalf("outbox = %d mensajes, se esperaba 1", len(outbox))
	}
	d := verificarEvento(t, outbox[0], dominio.EventoRecibida, 1, "RECIBIDA")
	if d.Solicitud.CreadoEn != "2026-10-15T09:00:00-03:00" {
		t.Fatalf("las horas tienen que ir con -03:00: %s", d.Solicitud.CreadoEn)
	}

	t.Run("el mismo comando repetido no duplica", func(t *testing.T) {
		e.registrar(t, "sol-1", nil)
		// Otro mensaje con el mismo id de solicitud (Integración lo republicó) tampoco.
		if err := e.casos.RegistrarExterna(context.Background(), comando("otro-msg", "sol-1", nil)); err != nil {
			t.Fatal(err)
		}
		if n := len(e.uow.outbox()); n != 1 {
			t.Fatalf("outbox = %d mensajes, se esperaba 1", n)
		}
		p, _ := e.casos.Listar(context.Background(), "", 1, 20)
		if p.Total != 1 {
			t.Fatalf("hay %d solicitudes, se esperaba 1", p.Total)
		}
	})

	t.Run("datos inválidos: error permanente y nada guardado", func(t *testing.T) {
		cmd := comando("msg-mal", "sol-mal", nil)
		cmd.Datos.Cosa.Cantidad = 0
		err := e.casos.RegistrarExterna(context.Background(), cmd)
		if !mensajeria.EsPermanente(err) || !errors.Is(err, dominio.ErrDatosInvalidos) {
			t.Fatalf("error = %v, se esperaba un error permanente de datos inválidos", err)
		}
		if _, err := e.casos.Obtener(context.Background(), "", "sol-mal"); !errors.Is(err, puertos.ErrNoEncontrada) {
			t.Fatalf("no tenía que guardarse: %v", err)
		}
	})
}

func TestCicloContrapropuesta(t *testing.T) {
	ctx := context.Background()
	e := nuevoEscenario()
	e.registrar(t, "sol-1", nil)

	hora := ahora.Add(30 * time.Hour)
	v, err := e.casos.Contraproponer(ctx, "sol-1", hora)
	if err != nil {
		t.Fatal(err)
	}
	if v.Estado != "CONTRAPROPUESTA" || v.Contrapropuesta.VenceEn != "2026-10-16T09:00:00-03:00" {
		t.Fatalf("vista inesperada: %+v", v)
	}
	verificarEvento(t, e.uow.outbox()[1], dominio.EventoContrapropuesta, 2, "CONTRAPROPUESTA")

	idem := &puertos.Idempotencia{Endpoint: "respuesta-contrapropuesta", Clave: "clave-0001", Hash: "h1"}
	v, err = e.casos.ResponderContrapropuesta(ctx, "org-1", "sol-1", true, idem)
	if err != nil {
		t.Fatal(err)
	}
	if v.Estado != "ACEPTADA" || v.Traslado == nil || v.Traslado.Estado != "PROGRAMADO" ||
		v.Traslado.HoraLlegadaComprometida != v.Contrapropuesta.HoraLlegadaPropuesta {
		t.Fatalf("vista inesperada: %+v", v)
	}
	verificarEvento(t, e.uow.outbox()[2], dominio.EventoAceptada, 3, "ACEPTADA")

	t.Run("misma clave y mismo cuerpo: la misma respuesta sin repetir el efecto", func(t *testing.T) {
		otra, err := e.casos.ResponderContrapropuesta(ctx, "org-1", "sol-1", true, idem)
		if err != nil {
			t.Fatal(err)
		}
		a, _ := json.Marshal(v)
		b, _ := json.Marshal(otra)
		if string(a) != string(b) {
			t.Fatalf("respuestas distintas:\n%s\n%s", a, b)
		}
		if n := len(e.uow.outbox()); n != 3 {
			t.Fatalf("outbox = %d, se esperaba 3", n)
		}
	})
	t.Run("misma clave con otro cuerpo: 409", func(t *testing.T) {
		_, err := e.casos.ResponderContrapropuesta(ctx, "org-1", "sol-1", false,
			&puertos.Idempotencia{Endpoint: idem.Endpoint, Clave: idem.Clave, Hash: "h2"})
		if !errors.Is(err, puertos.ErrClaveReutilizada) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("clave nueva sobre una solicitud ya aceptada: estado no permite", func(t *testing.T) {
		_, err := e.casos.ResponderContrapropuesta(ctx, "org-1", "sol-1", true,
			&puertos.Idempotencia{Endpoint: idem.Endpoint, Clave: "clave-0002", Hash: "h1"})
		if !errors.Is(err, dominio.ErrEstadoNoPermite) {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestRechazarYRespuestaNegativa(t *testing.T) {
	ctx := context.Background()
	e := nuevoEscenario()
	e.registrar(t, "sol-1", nil)
	e.registrar(t, "sol-2", nil)

	if _, err := e.casos.Rechazar(ctx, "sol-1", "sin vehículos"); err != nil {
		t.Fatal(err)
	}
	verificarEvento(t, e.uow.outbox()[2], dominio.EventoRechazada, 2, "RECHAZADA")

	if _, err := e.casos.Contraproponer(ctx, "sol-2", ahora.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := e.casos.ResponderContrapropuesta(ctx, "org-1", "sol-2", false, nil); err != nil {
		t.Fatal(err)
	}
	verificarEvento(t, e.uow.outbox()[4], dominio.EventoRechazadaPorCliente, 3, "RECHAZADA_POR_CLIENTE")
}

func TestErrorDeDominioNoDejaEventos(t *testing.T) {
	e := nuevoEscenario()
	e.registrar(t, "sol-1", nil)
	_, err := e.casos.Aceptar(context.Background(), "", "sol-1", nil)
	if !errors.Is(err, dominio.ErrSinHoraPedida) {
		t.Fatalf("error = %v", err)
	}
	if n := len(e.uow.outbox()); n != 1 {
		t.Fatalf("outbox = %d, se esperaba 1", n)
	}
}

func TestOrganizacionAjena(t *testing.T) {
	ctx := context.Background()
	e := nuevoEscenario()
	e.registrar(t, "sol-1", nil)

	if _, err := e.casos.Obtener(ctx, "org-otra", "sol-1"); !errors.Is(err, puertos.ErrNoEncontrada) {
		t.Fatalf("Obtener: %v", err)
	}
	if _, err := e.casos.Cancelar(ctx, "org-otra", "sol-1", "no", nil); !errors.Is(err, puertos.ErrNoEncontrada) {
		t.Fatalf("Cancelar: %v", err)
	}
	if _, err := e.casos.ResponderContrapropuesta(ctx, "org-otra", "sol-1", true, nil); !errors.Is(err, puertos.ErrNoEncontrada) {
		t.Fatalf("ResponderContrapropuesta: %v", err)
	}
	if v, err := e.casos.Obtener(ctx, "org-1", "sol-1"); err != nil || v.ID != "sol-1" {
		t.Fatalf("el dueño tiene que verla: %v", err)
	}
	if _, err := e.casos.Obtener(ctx, "", "sol-1"); err != nil {
		t.Fatalf("el personal de Rumbo ve todas: %v", err)
	}
}

func TestCancelaciones(t *testing.T) {
	ctx := context.Background()
	e := nuevoEscenario()
	e.registrar(t, "sol-1", nil)
	e.registrar(t, "sol-2", horaEn(3*time.Hour))
	e.registrar(t, "sol-3", horaEn(48*time.Hour))

	v, err := e.casos.Cancelar(ctx, "org-1", "sol-1", "ya no hace falta", nil)
	if err != nil || v.Estado != "CANCELADA" {
		t.Fatalf("v = %+v, err = %v", v, err)
	}
	verificarEvento(t, e.uow.outbox()[3], dominio.EventoCancelada, 2, "CANCELADA")

	if _, err := e.casos.Aceptar(ctx, "", "sol-3", nil); err != nil {
		t.Fatal(err)
	}
	v, err = e.casos.Cancelar(ctx, "org-1", "sol-3", "cambio de planes", nil)
	if err != nil || v.Cancelacion.Estado != "APROBADA" || v.Traslado.Estado != "CANCELADO" {
		t.Fatalf("v = %+v, err = %v", v, err)
	}
	verificarEvento(t, e.uow.outbox()[5], dominio.EventoCancelacionRegistrada, 3, "ACEPTADA")

	if _, err := e.casos.Aceptar(ctx, "", "sol-2", nil); err != nil {
		t.Fatal(err)
	}
	v, err = e.casos.Cancelar(ctx, "org-1", "sol-2", "cambio de planes", nil)
	if err != nil || v.Cancelacion.Estado != "PENDIENTE_APROBACION" || v.Traslado.Estado != "PROGRAMADO" {
		t.Fatalf("v = %+v, err = %v", v, err)
	}
	v, err = e.casos.ResolverCancelacionTardia(ctx, "sol-2", true)
	if err != nil || v.Cancelacion.Estado != "APROBADA" || v.Traslado.Estado != "CANCELADO" {
		t.Fatalf("v = %+v, err = %v", v, err)
	}
	outbox := e.uow.outbox()
	verificarEvento(t, outbox[len(outbox)-1], dominio.EventoTrasladoEstado, 4, "ACEPTADA")
}

func TestVencerContrapropuestas(t *testing.T) {
	ctx := context.Background()
	e := nuevoEscenario()
	e.registrar(t, "sol-1", nil)
	e.registrar(t, "sol-2", nil)
	if _, err := e.casos.Contraproponer(ctx, "sol-1", ahora.Add(30*time.Hour)); err != nil {
		t.Fatal(err)
	}

	if n, err := e.casos.VencerContrapropuestas(ctx); err != nil || n != 0 {
		t.Fatalf("antes de las 24 h no vence nada: n = %d, err = %v", n, err)
	}
	e.reloj = ahora.Add(24 * time.Hour)
	if n, err := e.casos.VencerContrapropuestas(ctx); err != nil || n != 1 {
		t.Fatalf("n = %d, err = %v", n, err)
	}
	outbox := e.uow.outbox()
	verificarEvento(t, outbox[len(outbox)-1], dominio.EventoVencida, 3, "RECHAZADA_POR_CLIENTE")

	// Responder después de vencida: CONTRAPROPUESTA_VENCIDA o estado no permite.
	if _, err := e.casos.ResponderContrapropuesta(ctx, "org-1", "sol-1", true, nil); !errors.Is(err, dominio.ErrEstadoNoPermite) {
		t.Fatalf("error = %v", err)
	}
}

func TestAceptarConClima(t *testing.T) {
	ctx := context.Background()
	destino := dominio.ExtremoDestino

	t.Run("no operable: alerta con motivos y dos eventos", func(t *testing.T) {
		e := nuevoEscenario()
		e.clima.operabilidad = dominio.Operabilidad{Operable: false, Motivos: []string{"HIGH_WIND", "HIGH_GUSTS"}, ObservadoEn: ahora}
		e.registrar(t, "sol-1", horaEn(3*time.Hour))

		v, err := e.casos.Aceptar(ctx, "", "sol-1", &destino)
		if err != nil {
			t.Fatal(err)
		}
		a := v.Traslado.AlertaClimatica
		if a == nil || a.Estado != "NO_OPERABLE" || len(a.Motivos) != 2 || *a.ObservadoEn != "2026-10-15T09:00:00-03:00" {
			t.Fatalf("alerta inesperada: %+v", a)
		}
		outbox := e.uow.outbox()
		if len(outbox) != 3 {
			t.Fatalf("outbox = %d, se esperaba 3", len(outbox))
		}
		aceptada := verificarEvento(t, outbox[1], dominio.EventoAceptada, 2, "ACEPTADA")
		if aceptada.Solicitud.Traslado.AlertaClimatica != nil {
			t.Fatal("el evento aceptada lleva la vista de su transición, todavía sin alerta")
		}
		verificarEvento(t, outbox[2], dominio.EventoAlertaClimatica, 3, "ACEPTADA")
		if e.clima.consultas != 1 {
			t.Fatalf("consultas = %d", e.clima.consultas)
		}
	})

	t.Run("operable: sin alerta", func(t *testing.T) {
		e := nuevoEscenario()
		e.registrar(t, "sol-1", horaEn(3*time.Hour))
		v, err := e.casos.Aceptar(ctx, "", "sol-1", &destino)
		if err != nil || v.Traslado.AlertaClimatica != nil {
			t.Fatalf("v = %+v, err = %v", v.Traslado, err)
		}
		if n := len(e.uow.outbox()); n != 2 || e.clima.consultas != 1 {
			t.Fatalf("outbox = %d, consultas = %d", n, e.clima.consultas)
		}
	})

	t.Run("proveedor caído: se acepta igual, clima no verificado", func(t *testing.T) {
		e := nuevoEscenario()
		e.clima.err = puertos.ErrClimaNoDisponible
		e.registrar(t, "sol-1", horaEn(3*time.Hour))
		v, err := e.casos.Aceptar(ctx, "", "sol-1", &destino)
		if err != nil {
			t.Fatalf("la aceptación no puede fallar por el clima: %v", err)
		}
		a := v.Traslado.AlertaClimatica
		if v.Estado != "ACEPTADA" || a == nil || a.Estado != "NO_VERIFICADO" || a.ObservadoEn != nil {
			t.Fatalf("vista inesperada: %+v", a)
		}
		verificarEvento(t, e.uow.outbox()[2], dominio.EventoAlertaClimatica, 3, "ACEPTADA")
	})

	t.Run("faltan más de 6 h: no se consulta", func(t *testing.T) {
		e := nuevoEscenario()
		e.registrar(t, "sol-1", horaEn(6*time.Hour+time.Minute))
		v, err := e.casos.Aceptar(ctx, "", "sol-1", &destino)
		if err != nil || v.Traslado.AlertaClimatica != nil || e.clima.consultas != 0 {
			t.Fatalf("err = %v, consultas = %d", err, e.clima.consultas)
		}
	})

	t.Run("sin aeropuerto: no se consulta", func(t *testing.T) {
		e := nuevoEscenario()
		e.registrar(t, "sol-1", horaEn(time.Hour))
		if _, err := e.casos.Aceptar(ctx, "", "sol-1", nil); err != nil || e.clima.consultas != 0 {
			t.Fatalf("err = %v, consultas = %d", err, e.clima.consultas)
		}
	})
}

func TestListar(t *testing.T) {
	ctx := context.Background()
	e := nuevoEscenario()
	for i, id := range []string{"sol-1", "sol-2", "sol-3"} {
		e.reloj = ahora.Add(time.Duration(i) * time.Minute)
		e.registrar(t, id, nil)
	}
	p, err := e.casos.Listar(ctx, "RECIBIDA", 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if p.Total != 3 || len(p.Resultados) != 2 || p.Resultados[0].ID != "sol-3" {
		t.Fatalf("página inesperada: %+v", p)
	}
	if _, err := e.casos.Listar(ctx, "INVENTADO", 1, 20); !errors.Is(err, dominio.ErrDatosInvalidos) {
		t.Fatalf("error = %v", err)
	}
	if _, err := e.casos.Listar(ctx, "", 1, 101); !errors.Is(err, dominio.ErrDatosInvalidos) {
		t.Fatalf("error = %v", err)
	}
}
