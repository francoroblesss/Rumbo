package aplicacion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/francoroblesss/Rumbo/servicios/operaciones/dominio"
	"github.com/francoroblesss/Rumbo/servicios/operaciones/puertos"
)

// ahora = 15/10/2026 09:00 en Córdoba.
var ahora = time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)

// calendario de prueba: franjas de 2 h entre las 8 y las 20, con lugar para 2 traslados y 1000 kg.
var calendario = dominio.Calendario{AperturaHora: 8, CierreHora: 20, DuracionHoras: 2, CupoTraslados: 2, CupoKg: 1000,
	Anticipacion: 2 * time.Hour, Horizonte: 30 * 24 * time.Hour, Zona: zonaRumbo}

// palabras son las de AEROPUERTO_PALABRAS por defecto.
var palabras = []string{"aeropuerto", "saco", "pajas blancas"}

type escenario struct {
	uow    *uowMemoria
	clima  *climaFalso
	casos  *Solicitudes
	reloj  time.Time
	claves int
}

func nuevoEscenario() *escenario {
	e := &escenario{uow: nuevaUoW(), clima: &climaFalso{operabilidad: dominio.Operabilidad{Operable: true, ObservadoEn: ahora}}, reloj: ahora}
	e.casos = NuevasSolicitudes(e.uow, e.clima, "SACO", palabras, calendario, func() time.Time { return e.reloj }, "test")
	return e
}

func datos(hora *time.Time) dominio.DatosSolicitud {
	return dominio.DatosSolicitud{
		Cosa:                  dominio.Cosa{Descripcion: "Cajas", Cantidad: 20},
		Desde:                 dominio.Ubicacion{Nombre: "Sucursal Centro", Direccion: "Av. Colón 1234"},
		Hasta:                 dominio.Ubicacion{Nombre: "Aeropuerto", Direccion: "Av. La Voz del Interior 8500"},
		HoraLlegadaSolicitada: hora,
	}
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
	if d.OrganizacionID != "org-1" || d.Origen == "" || d.Solicitud.Estado != estado || d.Solicitud.Version != version {
		t.Fatalf("%s: datos inesperados: %+v", tipo, d)
	}
	if d.Solicitud.ID != m.Sobre.AgregadoID {
		t.Fatalf("agregado_id = %s, vista = %s", m.Sobre.AgregadoID, d.Solicitud.ID)
	}
	return d
}

// franja16 es la franja del 16/10 de 10 a 12 (hora de Córdoba).
var franja16 = time.Date(2026, 10, 16, 13, 0, 0, 0, time.UTC)

func clave(c, hash string) *puertos.Idempotencia {
	return &puertos.Idempotencia{Endpoint: "reserva", Clave: c, Hash: hash}
}

func conPeso(kg float64) dominio.DatosSolicitud {
	d := datos(nil)
	d.Cosa.PesoKg = &kg
	return d
}

func (e *escenario) disponibles(t *testing.T) puertos.FranjaDisponible {
	t.Helper()
	d, err := e.casos.Disponibilidad(context.Background(), "2026-10-16", 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range d.Franjas {
		if f.Inicio == "2026-10-16T10:00:00-03:00" {
			return f
		}
	}
	t.Fatalf("no está la franja de las 10: %+v", d.Franjas)
	return puertos.FranjaDisponible{}
}

func TestReservar(t *testing.T) {
	ctx := context.Background()
	e := nuevoEscenario()

	v, err := e.casos.Reservar(ctx, "org-1", conPeso(400), franja16, clave("clave-0001", "h1"))
	if err != nil {
		t.Fatal(err)
	}
	if v.Estado != "ACEPTADA" || v.Traslado == nil || v.Traslado.Estado != "PROGRAMADO" || v.Traslado.Franja == nil ||
		v.Traslado.Franja.Inicio != "2026-10-16T10:00:00-03:00" || v.Traslado.Franja.Fin != "2026-10-16T12:00:00-03:00" ||
		v.Traslado.HoraLlegadaComprometida != v.Traslado.Franja.Fin {
		t.Fatalf("vista inesperada: %+v %+v", v, v.Traslado)
	}
	outbox := e.uow.outbox()
	if len(outbox) != 1 {
		t.Fatalf("outbox = %d mensajes, se esperaba 1", len(outbox))
	}
	if d := verificarEvento(t, outbox[0], dominio.EventoAceptada, 1, "ACEPTADA"); d.Origen != "externo" {
		t.Fatalf("origen = %s", d.Origen)
	}
	if f := e.disponibles(t); f.TrasladosDisponibles != 1 || f.KgDisponibles != 600 || !f.Disponible {
		t.Fatalf("disponibilidad después de reservar: %+v", f)
	}

	t.Run("misma clave y mismo cuerpo: la misma reserva sin tomar otro lugar", func(t *testing.T) {
		otra, err := e.casos.Reservar(ctx, "org-1", conPeso(400), franja16, clave("clave-0001", "h1"))
		if err != nil || otra.ID != v.ID {
			t.Fatalf("id = %s (original %s), err = %v", otra.ID, v.ID, err)
		}
		if f := e.disponibles(t); f.TrasladosDisponibles != 1 || len(e.uow.outbox()) != 1 {
			t.Fatalf("el reintento tomó otro lugar: %+v", f)
		}
	})
	t.Run("misma clave con otro cuerpo: 409", func(t *testing.T) {
		_, err := e.casos.Reservar(ctx, "org-1", conPeso(10), franja16, clave("clave-0001", "h2"))
		if !errors.Is(err, puertos.ErrClaveReutilizada) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("la carga no entra en lo que queda: sin disponibilidad y nada guardado", func(t *testing.T) {
		_, err := e.casos.Reservar(ctx, "org-1", conPeso(700), franja16, clave("clave-0002", "h3"))
		if !errors.Is(err, dominio.ErrSinDisponibilidad) {
			t.Fatalf("error = %v", err)
		}
		if p, _ := e.casos.Listar(ctx, puertos.FiltroListado{Pagina: 1, Tamanio: 20}); p.Total != 1 || len(e.uow.outbox()) != 1 {
			t.Fatalf("se guardó algo: %d solicitudes", p.Total)
		}
	})
	t.Run("se llena la franja", func(t *testing.T) {
		if _, err := e.casos.Reservar(ctx, "org-1", datos(nil), franja16, clave("clave-0003", "h4")); err != nil {
			t.Fatal(err)
		}
		_, err := e.casos.Reservar(ctx, "org-1", datos(nil), franja16, clave("clave-0004", "h5"))
		if !errors.Is(err, dominio.ErrSinDisponibilidad) {
			t.Fatalf("error = %v", err)
		}
		if f := e.disponibles(t); f.Disponible || f.TrasladosDisponibles != 0 {
			t.Fatalf("la franja tiene que figurar llena: %+v", f)
		}
		// El reintento de la primera reserva sigue devolviendo la reserva original.
		if otra, err := e.casos.Reservar(ctx, "org-1", conPeso(400), franja16, clave("clave-0001", "h1")); err != nil || otra.ID != v.ID {
			t.Fatalf("reintento con la franja llena: %v", err)
		}
	})
	t.Run("cancelar devuelve el lugar", func(t *testing.T) {
		c, err := e.casos.Cancelar(ctx, "org-1", v.ID, "ya no hace falta", nil)
		if err != nil || c.Traslado.Estado != "CANCELADO" || c.Cancelacion.Estado != "APROBADA" || c.Cancelacion.Tardia {
			t.Fatalf("v = %+v, err = %v", c.Cancelacion, err)
		}
		if f := e.disponibles(t); f.TrasladosDisponibles != 1 || f.KgDisponibles != 1000 {
			t.Fatalf("no se devolvió el lugar: %+v", f)
		}
	})
	t.Run("franja que no se puede reservar", func(t *testing.T) {
		casos := map[time.Time]error{
			franja16.Add(30 * time.Minute):           dominio.ErrFranjaInvalida,
			ahora.Add(time.Hour).Truncate(time.Hour): dominio.ErrAnticipacionInsuficiente,
		}
		for inicio, esperado := range casos {
			_, err := e.casos.Reservar(ctx, "org-1", datos(nil), inicio, clave("clave-"+inicio.String(), "h"))
			if !errors.Is(err, esperado) {
				t.Fatalf("%s: error = %v, se esperaba %v", inicio, err, esperado)
			}
		}
	})
}

func TestDisponibilidad(t *testing.T) {
	e := nuevoEscenario()
	d, err := e.casos.Disponibilidad(context.Background(), "2026-10-15", 1500)
	if err != nil {
		t.Fatal(err)
	}
	// Hoy a las 09:00: la franja de las 10 ya no tiene 2 h de anticipación; quedan 12, 14, 16 y 18.
	if len(d.Franjas) != 4 || d.Franjas[0].Inicio != "2026-10-15T12:00:00-03:00" || d.Franjas[0].Disponible {
		t.Fatalf("disponibilidad inesperada (1500 kg no entran en ninguna): %+v", d)
	}
	for _, mala := range []string{"15/10/2026", "2026-10-14", "2027-01-01"} {
		if _, err := e.casos.Disponibilidad(context.Background(), mala, 0); err == nil {
			t.Fatalf("%s: se esperaba un error", mala)
		}
	}
}

// franjaHoy es la franja de hoy (15/10) de 12 a 14 en Córdoba: termina dentro de 5 h, en la
// ventana del clima (R-46).
var franjaHoy = time.Date(2026, 10, 15, 15, 0, 0, 0, time.UTC)

// alAeropuerto es una carga con destino al aeropuerto (se detecta por el nombre).
func alAeropuerto() dominio.DatosSolicitud {
	d := datos(nil)
	d.Hasta = dominio.Ubicacion{Nombre: "Aeropuerto Internacional de Córdoba", Direccion: "Av. La Voz del Interior 8500"}
	return d
}

func comun() dominio.DatosSolicitud {
	d := datos(nil)
	d.Hasta = dominio.Ubicacion{Nombre: "Sucursal Cerro", Direccion: "Av. R. Núñez 4500"}
	return d
}

// reservar hace una reserva con una clave nueva y devuelve su id.
func (e *escenario) reservar(t *testing.T, org string, d dominio.DatosSolicitud, inicio time.Time) string {
	t.Helper()
	e.claves++
	v, err := e.casos.Reservar(context.Background(), org, d, inicio, clave(fmt.Sprintf("clave-%04d", e.claves), "h"))
	if err != nil {
		t.Fatalf("reservar: %v", err)
	}
	return v.ID
}

var noOperable = dominio.Operabilidad{Operable: false, Motivos: []string{"HIGH_WIND", "HIGH_GUSTS"}, ObservadoEn: ahora}

func TestAvanzarEstado(t *testing.T) {
	ctx := context.Background()

	t.Run("PROGRAMADO → EN_CURSO → COMPLETADO con sus eventos", func(t *testing.T) {
		e := nuevoEscenario()
		id := e.reservar(t, "org-1", comun(), franja16)
		v, err := e.casos.AvanzarEstado(ctx, "", id, dominio.TrasladoEnCurso)
		if err != nil || v.Traslado.Estado != "EN_CURSO" || v.Version != 2 {
			t.Fatalf("v = %+v, err = %v", v.Traslado, err)
		}
		e.reloj = ahora.Add(time.Hour)
		v, err = e.casos.AvanzarEstado(ctx, "", id, dominio.TrasladoCompletado)
		if err != nil || v.Traslado.Estado != "COMPLETADO" || v.Traslado.FinalizadoEn == nil ||
			*v.Traslado.FinalizadoEn != "2026-10-15T10:00:00-03:00" || v.Version != 3 {
			t.Fatalf("v = %+v, err = %v", v.Traslado, err)
		}
		outbox := e.uow.outbox()
		if len(outbox) != 3 {
			t.Fatalf("outbox = %d, se esperaba 3", len(outbox))
		}
		if d := verificarEvento(t, outbox[1], dominio.EventoTrasladoEstado, 2, "ACEPTADA"); d.Solicitud.Traslado.Estado != "EN_CURSO" {
			t.Fatalf("estado en el evento = %s", d.Solicitud.Traslado.Estado)
		}
		if d := verificarEvento(t, outbox[2], dominio.EventoTrasladoEstado, 3, "ACEPTADA"); d.Solicitud.Traslado.Estado != "COMPLETADO" {
			t.Fatalf("estado en el evento = %s", d.Solicitud.Traslado.Estado)
		}
		if e.clima.consultas != 0 {
			t.Fatal("un traslado que no va al aeropuerto no consulta el clima")
		}

		t.Run("repetir el último avance: misma vista y sin evento (CA-38)", func(t *testing.T) {
			otra, err := e.casos.AvanzarEstado(ctx, "", id, dominio.TrasladoCompletado)
			if err != nil || mustJSON(t, otra) != mustJSON(t, v) || len(e.uow.outbox()) != 3 {
				t.Fatalf("err = %v, outbox = %d", err, len(e.uow.outbox()))
			}
		})
		t.Run("volver atrás: estado no permite", func(t *testing.T) {
			_, err := e.casos.AvanzarEstado(ctx, "", id, dominio.TrasladoEnCurso)
			if !errors.Is(err, dominio.ErrEstadoNoPermite) || len(e.uow.outbox()) != 3 {
				t.Fatalf("err = %v", err)
			}
		})
	})

	t.Run("transiciones y destinos inválidos", func(t *testing.T) {
		e := nuevoEscenario()
		id := e.reservar(t, "org-1", comun(), franja16)
		if _, err := e.casos.AvanzarEstado(ctx, "", id, dominio.TrasladoCompletado); !errors.Is(err, dominio.ErrEstadoNoPermite) {
			t.Fatalf("PROGRAMADO → COMPLETADO: %v", err)
		}
		for _, destino := range []dominio.EstadoTraslado{dominio.TrasladoProgramado, dominio.TrasladoCancelado, "VOLANDO"} {
			if _, err := e.casos.AvanzarEstado(ctx, "", id, destino); !errors.Is(err, dominio.ErrDatosInvalidos) {
				t.Fatalf("%s: %v", destino, err)
			}
		}
		if _, err := e.casos.Cancelar(ctx, "org-1", id, "no hace falta", nil); err != nil {
			t.Fatal(err)
		}
		if _, err := e.casos.AvanzarEstado(ctx, "", id, dominio.TrasladoEnCurso); !errors.Is(err, dominio.ErrEstadoNoPermite) {
			t.Fatalf("CANCELADO → EN_CURSO: %v", err)
		}
		if _, err := e.casos.AvanzarEstado(ctx, "", "sol-no-existe", dominio.TrasladoEnCurso); !errors.Is(err, puertos.ErrNoEncontrada) {
			t.Fatalf("inexistente: %v", err)
		}
	})

	t.Run("al aeropuerto con clima no operable: alerta y dos eventos", func(t *testing.T) {
		e := nuevoEscenario()
		e.clima.operabilidad = noOperable
		id := e.reservar(t, "org-1", alAeropuerto(), franjaHoy)
		v, err := e.casos.AvanzarEstado(ctx, "", id, dominio.TrasladoEnCurso)
		if err != nil {
			t.Fatal(err)
		}
		a := v.Traslado.AlertaClimatica
		if v.Traslado.Estado != "EN_CURSO" || a == nil || a.Estado != "NO_OPERABLE" || len(a.Motivos) != 2 ||
			*a.ObservadoEn != "2026-10-15T09:00:00-03:00" || v.Version != 3 {
			t.Fatalf("vista inesperada: %+v %+v", v.Traslado, a)
		}
		outbox := e.uow.outbox()
		if len(outbox) != 3 || e.clima.consultas != 1 {
			t.Fatalf("outbox = %d, consultas = %d", len(outbox), e.clima.consultas)
		}
		if d := verificarEvento(t, outbox[1], dominio.EventoTrasladoEstado, 2, "ACEPTADA"); d.Solicitud.Traslado.AlertaClimatica != nil {
			t.Fatal("el evento de estado lleva la vista de su transición, todavía sin alerta")
		}
		verificarEvento(t, outbox[2], dominio.EventoAlertaClimatica, 3, "ACEPTADA")
	})

	t.Run("al aeropuerto con el clima caído: cambia igual y queda NO_VERIFICADO (CA-36)", func(t *testing.T) {
		e := nuevoEscenario()
		e.clima.err = puertos.ErrClimaNoDisponible
		id := e.reservar(t, "org-1", alAeropuerto(), franjaHoy)
		v, err := e.casos.AvanzarEstado(ctx, "", id, dominio.TrasladoEnCurso)
		if err != nil {
			t.Fatalf("el cambio de estado no puede fallar por el clima: %v", err)
		}
		a := v.Traslado.AlertaClimatica
		if v.Traslado.Estado != "EN_CURSO" || a == nil || a.Estado != "NO_VERIFICADO" || a.ObservadoEn != nil || len(a.Motivos) != 0 {
			t.Fatalf("vista inesperada: %+v %+v", v.Traslado, a)
		}
		verificarEvento(t, e.uow.outbox()[2], dominio.EventoAlertaClimatica, 3, "ACEPTADA")
	})

	t.Run("al aeropuerto con la misma alerta que ya tenía: solo el evento de estado", func(t *testing.T) {
		e := nuevoEscenario()
		e.clima.operabilidad = noOperable
		id := e.reservar(t, "org-1", alAeropuerto(), franjaHoy)
		if err := e.casos.RevisarClimaDeReserva(ctx, id); err != nil {
			t.Fatal(err)
		}
		v, err := e.casos.AvanzarEstado(ctx, "", id, dominio.TrasladoEnCurso)
		if err != nil || v.Traslado.AlertaClimatica.Estado != "NO_OPERABLE" || e.clima.consultas != 2 {
			t.Fatalf("v = %+v, err = %v, consultas = %d", v.Traslado, err, e.clima.consultas)
		}
		outbox := e.uow.outbox()
		if len(outbox) != 3 || outbox[2].Sobre.Tipo != string(dominio.EventoTrasladoEstado) {
			t.Fatalf("outbox = %d, último = %s", len(outbox), outbox[len(outbox)-1].Sobre.Tipo)
		}
	})

	t.Run("al aeropuerto pero falta más de 6 h: no consulta", func(t *testing.T) {
		e := nuevoEscenario()
		id := e.reservar(t, "org-1", alAeropuerto(), franja16)
		if _, err := e.casos.AvanzarEstado(ctx, "", id, dominio.TrasladoEnCurso); err != nil || e.clima.consultas != 0 {
			t.Fatalf("err = %v, consultas = %d", err, e.clima.consultas)
		}
	})
}

func TestRevisarClimaDeReserva(t *testing.T) {
	ctx := context.Background()

	t.Run("no operable: alerta y evento", func(t *testing.T) {
		e := nuevoEscenario()
		e.clima.operabilidad = noOperable
		id := e.reservar(t, "org-1", alAeropuerto(), franjaHoy)
		if err := e.casos.RevisarClimaDeReserva(ctx, id); err != nil {
			t.Fatal(err)
		}
		v, _ := e.casos.Obtener(ctx, "", id)
		if a := v.Traslado.AlertaClimatica; a == nil || a.Estado != "NO_OPERABLE" || len(a.Motivos) != 2 || v.Version != 2 {
			t.Fatalf("vista inesperada: %+v", v.Traslado)
		}
		outbox := e.uow.outbox()
		if len(outbox) != 2 {
			t.Fatalf("outbox = %d, se esperaba 2", len(outbox))
		}
		verificarEvento(t, outbox[1], dominio.EventoAlertaClimatica, 2, "ACEPTADA")

		t.Run("evento reentregado: no escribe ni publica de nuevo", func(t *testing.T) {
			if err := e.casos.RevisarClimaDeReserva(ctx, id); err != nil {
				t.Fatal(err)
			}
			v, _ := e.casos.Obtener(ctx, "", id)
			if v.Version != 2 || len(e.uow.outbox()) != 2 {
				t.Fatalf("versión = %d, outbox = %d", v.Version, len(e.uow.outbox()))
			}
		})
	})

	t.Run("operable: sin escritura", func(t *testing.T) {
		e := nuevoEscenario()
		id := e.reservar(t, "org-1", alAeropuerto(), franjaHoy)
		if err := e.casos.RevisarClimaDeReserva(ctx, id); err != nil {
			t.Fatal(err)
		}
		v, _ := e.casos.Obtener(ctx, "", id)
		if v.Traslado.AlertaClimatica != nil || v.Version != 1 || len(e.uow.outbox()) != 1 || e.clima.consultas != 1 {
			t.Fatalf("versión = %d, outbox = %d, consultas = %d", v.Version, len(e.uow.outbox()), e.clima.consultas)
		}
	})

	t.Run("clima caído: NO_VERIFICADO sin error (fail-open)", func(t *testing.T) {
		e := nuevoEscenario()
		e.clima.err = puertos.ErrClimaNoDisponible
		id := e.reservar(t, "org-1", alAeropuerto(), franjaHoy)
		if err := e.casos.RevisarClimaDeReserva(ctx, id); err != nil {
			t.Fatalf("un clima caído no es un error del mensaje: %v", err)
		}
		v, _ := e.casos.Obtener(ctx, "", id)
		if a := v.Traslado.AlertaClimatica; a == nil || a.Estado != "NO_VERIFICADO" {
			t.Fatalf("alerta inesperada: %+v", a)
		}
		verificarEvento(t, e.uow.outbox()[1], dominio.EventoAlertaClimatica, 2, "ACEPTADA")
	})

	t.Run("fuera de la ventana o sin aeropuerto: no consulta", func(t *testing.T) {
		e := nuevoEscenario()
		lejos := e.reservar(t, "org-1", alAeropuerto(), franja16)
		cerca := e.reservar(t, "org-1", comun(), franjaHoy)
		for _, id := range []string{lejos, cerca} {
			if err := e.casos.RevisarClimaDeReserva(ctx, id); err != nil {
				t.Fatal(err)
			}
		}
		if e.clima.consultas != 0 || len(e.uow.outbox()) != 2 {
			t.Fatalf("consultas = %d, outbox = %d", e.clima.consultas, len(e.uow.outbox()))
		}
	})

	t.Run("cancelado mientras se consultaba: no registra la alerta", func(t *testing.T) {
		e := nuevoEscenario()
		e.clima.operabilidad = noOperable
		id := e.reservar(t, "org-1", alAeropuerto(), franjaHoy)
		e.clima.durante = func() {
			if _, err := e.casos.Cancelar(ctx, "org-1", id, "cambio de planes", nil); err != nil {
				t.Error(err)
			}
		}
		if err := e.casos.RevisarClimaDeReserva(ctx, id); err != nil {
			t.Fatal(err)
		}
		v, _ := e.casos.Obtener(ctx, "", id)
		if v.Traslado.AlertaClimatica != nil || v.Traslado.Estado != "CANCELADO" {
			t.Fatalf("vista inesperada: %+v", v.Traslado)
		}
	})

	t.Run("solicitud inexistente", func(t *testing.T) {
		if err := nuevoEscenario().casos.RevisarClimaDeReserva(ctx, "sol-no-existe"); !errors.Is(err, puertos.ErrNoEncontrada) {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestListar(t *testing.T) {
	ctx := context.Background()
	e := nuevoEscenario()
	franja17 := franja16.AddDate(0, 0, 1)
	// Se crean fuera de orden: el listado ordena por franja y después por creación.
	e.reloj = ahora.Add(time.Minute)
	b := e.reservar(t, "org-1", comun(), franja16)
	e.reloj = ahora
	c := e.reservar(t, "org-1", comun(), franja17)
	a := e.reservar(t, "org-1", comun(), franjaHoy)
	e.reloj = ahora.Add(2 * time.Minute)
	ajena := e.reservar(t, "org-2", comun(), franja16)
	if _, err := e.casos.AvanzarEstado(ctx, "", a, dominio.TrasladoEnCurso); err != nil {
		t.Fatal(err)
	}

	ids := func(p puertos.Pagina) []string {
		var r []string
		for _, v := range p.Resultados {
			r = append(r, v.ID)
		}
		return r
	}
	casos := []struct {
		nombre string
		filtro puertos.FiltroListado
		total  int
		ids    []string
	}{
		{"todas, en orden", puertos.FiltroListado{Pagina: 1, Tamanio: 20}, 4, []string{a, b, ajena, c}},
		{"por organización", puertos.FiltroListado{Organizacion: "org-1", Pagina: 1, Tamanio: 20}, 3, []string{a, b, c}},
		{"por fecha (día en Córdoba)", puertos.FiltroListado{Fecha: "2026-10-16", Pagina: 1, Tamanio: 20}, 2, []string{b, ajena}},
		{"por estado", puertos.FiltroListado{EstadoTraslado: "EN_CURSO", Pagina: 1, Tamanio: 20}, 1, []string{a}},
		{"organización y fecha", puertos.FiltroListado{Organizacion: "org-2", Fecha: "2026-10-16", Pagina: 1, Tamanio: 20}, 1, []string{ajena}},
		{"paginado", puertos.FiltroListado{Pagina: 2, Tamanio: 3}, 4, []string{c}},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			p, err := e.casos.Listar(ctx, c.filtro)
			if err != nil || p.Total != c.total || mustJSON(t, ids(p)) != mustJSON(t, c.ids) {
				t.Fatalf("total = %d, ids = %v, err = %v; se esperaba %d %v", p.Total, ids(p), err, c.total, c.ids)
			}
		})
	}
	for _, malo := range []puertos.FiltroListado{
		{EstadoTraslado: "ASIGNADO", Pagina: 1, Tamanio: 20},
		{Fecha: "16/10/2026", Pagina: 1, Tamanio: 20},
		{Pagina: 0, Tamanio: 20},
		{Pagina: 1, Tamanio: 101},
	} {
		if _, err := e.casos.Listar(ctx, malo); !errors.Is(err, dominio.ErrDatosInvalidos) {
			t.Fatalf("%+v: error = %v", malo, err)
		}
	}
}

func TestFranjas(t *testing.T) {
	ctx := context.Background()
	e := nuevoEscenario()
	e.reservar(t, "org-1", conPeso(400), franjaHoy)
	d, err := e.casos.Franjas(ctx, "2026-10-15")
	if err != nil {
		t.Fatal(err)
	}
	// Todas las franjas de hoy, también la de las 8 que ya empezó; la de las 12 con la reserva.
	if d.Fecha != "2026-10-15" || len(d.Franjas) != 6 || d.Franjas[0].Inicio != "2026-10-15T08:00:00-03:00" {
		t.Fatalf("franjas inesperadas: %+v", d)
	}
	conReserva, sinFila := d.Franjas[2], d.Franjas[3]
	if conReserva.Inicio != "2026-10-15T12:00:00-03:00" || conReserva.TrasladosReservados != 1 || conReserva.KgReservados != 400 ||
		conReserva.CupoTraslados != 2 || conReserva.CupoKg != 1000 {
		t.Fatalf("franja con reserva: %+v", conReserva)
	}
	if sinFila.TrasladosReservados != 0 || sinFila.KgReservados != 0 || sinFila.CupoTraslados != 2 || sinFila.Fin != "2026-10-15T16:00:00-03:00" {
		t.Fatalf("franja sin fila: %+v", sinFila)
	}
	for _, mala := range []string{"15/10/2026", "2026-10-14", "2027-01-01"} {
		if _, err := e.casos.Franjas(ctx, mala); !errors.Is(err, dominio.ErrDatosInvalidos) {
			t.Fatalf("%s: error = %v", mala, err)
		}
	}
}

func TestCancelarDesdeWeb(t *testing.T) {
	ctx := context.Background()
	e := nuevoEscenario()
	id := e.reservar(t, "org-1", conPeso(400), franja16)
	idem := &puertos.Idempotencia{Endpoint: "cancelacion", Clave: "clave-web", Hash: "h"}

	if _, err := e.casos.CancelarDesdeWeb(ctx, "org-otra", id, "no", idem); !errors.Is(err, puertos.ErrNoEncontrada) {
		t.Fatalf("otra organización: %v", err)
	}
	// El operador (org "") cancela: la clave queda con la organización de la solicitud.
	v, err := e.casos.CancelarDesdeWeb(ctx, "", id, "cambio de planes", idem)
	if err != nil || v.Traslado.Estado != "CANCELADO" || v.Cancelacion.Estado != "APROBADA" || v.Cancelacion.Tardia {
		t.Fatalf("v = %+v, err = %v", v.Cancelacion, err)
	}
	if f := e.disponibles(t); f.TrasladosDisponibles != 2 || f.KgDisponibles != 1000 {
		t.Fatalf("no se devolvió el lugar: %+v", f)
	}
	outbox := e.uow.outbox()
	verificarEvento(t, outbox[len(outbox)-1], dominio.EventoCancelacionRegistrada, 2, "ACEPTADA")

	t.Run("el cliente repite con la misma clave: la misma respuesta", func(t *testing.T) {
		otra, err := e.casos.CancelarDesdeWeb(ctx, "org-1", id, "cambio de planes", idem)
		if err != nil || mustJSON(t, otra) != mustJSON(t, v) || len(e.uow.outbox()) != len(outbox) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("clave nueva sobre un traslado cancelado: estado no permite", func(t *testing.T) {
		_, err := e.casos.CancelarDesdeWeb(ctx, "org-1", id, "otra vez",
			&puertos.Idempotencia{Endpoint: "cancelacion", Clave: "clave-otra", Hash: "h"})
		if !errors.Is(err, dominio.ErrEstadoNoPermite) {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestOrganizacionAjena(t *testing.T) {
	ctx := context.Background()
	e := nuevoEscenario()
	id := e.reservar(t, "org-1", comun(), franja16)

	if _, err := e.casos.Obtener(ctx, "org-otra", id); !errors.Is(err, puertos.ErrNoEncontrada) {
		t.Fatalf("Obtener: %v", err)
	}
	if _, err := e.casos.Cancelar(ctx, "org-otra", id, "no", nil); !errors.Is(err, puertos.ErrNoEncontrada) {
		t.Fatalf("Cancelar: %v", err)
	}
	if _, err := e.casos.AvanzarEstado(ctx, "org-otra", id, dominio.TrasladoEnCurso); !errors.Is(err, puertos.ErrNoEncontrada) {
		t.Fatalf("AvanzarEstado: %v", err)
	}
	if v, err := e.casos.Obtener(ctx, "org-1", id); err != nil || v.ID != id {
		t.Fatalf("el dueño tiene que verla: %v", err)
	}
	if _, err := e.casos.Obtener(ctx, "", id); err != nil {
		t.Fatalf("el personal de Rumbo ve todas: %v", err)
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
