package dominio

import (
	"errors"
	"testing"
	"time"
)

var ahora = time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)

func datosValidos(hora *time.Time) DatosSolicitud {
	return DatosSolicitud{
		Cosa:                  Cosa{Descripcion: "Cajas", Cantidad: 20},
		Desde:                 Ubicacion{Nombre: "Sucursal Centro", Direccion: "Av. Colón 1234"},
		Hasta:                 Ubicacion{Nombre: "Sucursal Cerro", Direccion: "Av. R. Núñez 4500"},
		HoraLlegadaSolicitada: hora,
	}
}

func recibida(t *testing.T, hora *time.Time) *Solicitud {
	t.Helper()
	s, _, err := NuevaSolicitud("sol-1", "org-1", OrigenInterno, datosValidos(hora), ahora)
	if err != nil {
		t.Fatalf("crear solicitud: %v", err)
	}
	return s
}

func en(d time.Duration) *time.Time {
	h := ahora.Add(d)
	return &h
}

func aceptada(t *testing.T, faltan time.Duration) *Solicitud {
	t.Helper()
	s := recibida(t, en(faltan))
	if _, err := s.Aceptar("tra-1", ahora, nil); err != nil {
		t.Fatalf("aceptar: %v", err)
	}
	return s
}

func esperarError(t *testing.T, err, esperado error) {
	t.Helper()
	if !errors.Is(err, esperado) {
		t.Fatalf("error = %v, se esperaba %v", err, esperado)
	}
}

func esperarEvento(t *testing.T, ev Evento, err error, esperado Evento) {
	t.Helper()
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if ev != esperado {
		t.Fatalf("evento = %q, se esperaba %q", ev, esperado)
	}
}

func TestNuevaSolicitud(t *testing.T) {
	s, ev, err := NuevaSolicitud("sol-1", "org-1", OrigenInterno, datosValidos(nil), ahora)
	esperarEvento(t, ev, err, EventoRecibida)
	if s.Estado != SolicitudRecibida || s.Version != 1 || s.Origen != OrigenInterno || !s.CreadoEn.Equal(ahora) {
		t.Fatalf("solicitud inesperada: %+v", s)
	}

	invalidos := map[string]func(*DatosSolicitud){
		"sin descripción":       func(d *DatosSolicitud) { d.Cosa.Descripcion = " " },
		"cantidad cero":         func(d *DatosSolicitud) { d.Cosa.Cantidad = 0 },
		"peso cero":             func(d *DatosSolicitud) { cero := 0.0; d.Cosa.PesoKg = &cero },
		"origen sin nombre":     func(d *DatosSolicitud) { d.Desde.Nombre = "" },
		"origen sin dirección":  func(d *DatosSolicitud) { d.Desde.Direccion = "" },
		"destino sin nombre":    func(d *DatosSolicitud) { d.Hasta.Nombre = "" },
		"destino sin dirección": func(d *DatosSolicitud) { d.Hasta.Direccion = "" },
	}
	for nombre, romper := range invalidos {
		t.Run(nombre, func(t *testing.T) {
			d := datosValidos(nil)
			romper(&d)
			_, _, err := NuevaSolicitud("sol-1", "org-1", OrigenInterno, d, ahora)
			esperarError(t, err, ErrDatosInvalidos)
		})
	}
}

func TestAceptar(t *testing.T) {
	t.Run("con hora pedida crea el traslado programado", func(t *testing.T) {
		s := recibida(t, en(48*time.Hour))
		extremo := ExtremoDestino
		ev, err := s.Aceptar("tra-1", ahora, &extremo)
		esperarEvento(t, ev, err, EventoAceptada)
		if s.Estado != SolicitudAceptada || s.Version != 2 || s.Traslado == nil {
			t.Fatalf("solicitud inesperada: %+v", s)
		}
		if s.Traslado.ID != "tra-1" || s.Traslado.Estado != TrasladoProgramado ||
			!s.Traslado.HoraLlegadaComprometida.Equal(*s.HoraLlegadaSolicitada) || *s.Traslado.AeropuertoExtremo != ExtremoDestino {
			t.Fatalf("traslado inesperado: %+v", s.Traslado)
		}
	})
	t.Run("sin hora pedida hay que contraproponer", func(t *testing.T) {
		s := recibida(t, nil)
		_, err := s.Aceptar("tra-1", ahora, nil)
		esperarError(t, err, ErrSinHoraPedida)
		if s.Version != 1 {
			t.Fatal("un error no tiene que cambiar la versión")
		}
	})
	t.Run("hora pedida en el pasado o ahora", func(t *testing.T) {
		_, err := recibida(t, en(0)).Aceptar("tra-1", ahora, nil)
		esperarError(t, err, ErrHorarioEnElPasado)
		_, err = recibida(t, en(-time.Minute)).Aceptar("tra-1", ahora, nil)
		esperarError(t, err, ErrHorarioEnElPasado)
	})
	t.Run("solo desde RECIBIDA", func(t *testing.T) {
		s := aceptada(t, 48*time.Hour)
		_, err := s.Aceptar("tra-2", ahora, nil)
		esperarError(t, err, ErrEstadoNoPermite)
	})
}

func TestContraproponer(t *testing.T) {
	s := recibida(t, nil)
	hora := ahora.Add(30 * time.Hour)
	ev, err := s.Contraproponer(hora, ahora)
	esperarEvento(t, ev, err, EventoContrapropuesta)
	if s.Estado != SolicitudContrapropuesta || !s.Contrapropuesta.Hora.Equal(hora) ||
		!s.Contrapropuesta.VenceEn.Equal(ahora.Add(24*time.Hour)) || s.Version != 2 {
		t.Fatalf("solicitud inesperada: %+v", s)
	}

	_, err = recibida(t, nil).Contraproponer(ahora, ahora)
	esperarError(t, err, ErrHorarioEnElPasado)
	_, err = s.Contraproponer(hora, ahora)
	esperarError(t, err, ErrEstadoNoPermite)
}

func TestRechazar(t *testing.T) {
	s := recibida(t, nil)
	_, err := s.Rechazar("  ", ahora)
	esperarError(t, err, ErrDatosInvalidos)
	ev, err := s.Rechazar("sin vehículos", ahora)
	esperarEvento(t, ev, err, EventoRechazada)
	if s.Estado != SolicitudRechazada || *s.MotivoRechazo != "sin vehículos" {
		t.Fatalf("solicitud inesperada: %+v", s)
	}
	_, err = s.Rechazar("otra vez", ahora)
	esperarError(t, err, ErrEstadoNoPermite)
}

func contrapropuesta(t *testing.T) *Solicitud {
	t.Helper()
	s := recibida(t, nil)
	if _, err := s.Contraproponer(ahora.Add(30*time.Hour), ahora); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestResponderContrapropuesta(t *testing.T) {
	t.Run("acepta: traslado con la hora propuesta", func(t *testing.T) {
		s := contrapropuesta(t)
		ev, err := s.ResponderContrapropuesta(true, "tra-1", ahora.Add(time.Hour))
		esperarEvento(t, ev, err, EventoAceptada)
		if s.Estado != SolicitudAceptada || s.Traslado.Estado != TrasladoProgramado ||
			!s.Traslado.HoraLlegadaComprometida.Equal(s.Contrapropuesta.Hora) || s.Version != 3 {
			t.Fatalf("solicitud inesperada: %+v", s)
		}
	})
	t.Run("rechaza", func(t *testing.T) {
		s := contrapropuesta(t)
		ev, err := s.ResponderContrapropuesta(false, "", ahora.Add(time.Hour))
		esperarEvento(t, ev, err, EventoRechazadaPorCliente)
		if s.Estado != SolicitudRechazadaPorCliente || s.Traslado != nil {
			t.Fatalf("solicitud inesperada: %+v", s)
		}
	})
	t.Run("vencida (exactamente a las 24 h) no cambia nada", func(t *testing.T) {
		s := contrapropuesta(t)
		_, err := s.ResponderContrapropuesta(true, "tra-1", ahora.Add(24*time.Hour))
		esperarError(t, err, ErrContrapropuestaVencida)
		if s.Estado != SolicitudContrapropuesta || s.Version != 2 {
			t.Fatalf("no tenía que cambiar: %+v", s)
		}
	})
	t.Run("solo desde CONTRAPROPUESTA", func(t *testing.T) {
		_, err := recibida(t, nil).ResponderContrapropuesta(true, "tra-1", ahora)
		esperarError(t, err, ErrEstadoNoPermite)
	})
}

func TestVencer(t *testing.T) {
	s := contrapropuesta(t)
	_, err := s.Vencer(ahora.Add(24*time.Hour - time.Second))
	esperarError(t, err, ErrEstadoNoPermite)
	ev, err := s.Vencer(ahora.Add(24 * time.Hour))
	esperarEvento(t, ev, err, EventoVencida)
	if s.Estado != SolicitudRechazadaPorCliente || s.Version != 3 {
		t.Fatalf("solicitud inesperada: %+v", s)
	}
	_, err = recibida(t, nil).Vencer(ahora.Add(48 * time.Hour))
	esperarError(t, err, ErrEstadoNoPermite)
}

func TestCancelar(t *testing.T) {
	t.Run("motivo obligatorio", func(t *testing.T) {
		_, err := recibida(t, nil).Cancelar("", ahora)
		esperarError(t, err, ErrDatosInvalidos)
	})
	t.Run("antes de aceptar: CANCELADA", func(t *testing.T) {
		for _, s := range []*Solicitud{recibida(t, nil), contrapropuesta(t)} {
			ev, err := s.Cancelar("ya no hace falta", ahora)
			esperarEvento(t, ev, err, EventoCancelada)
			if s.Estado != SolicitudCancelada {
				t.Fatalf("estado = %s", s.Estado)
			}
		}
	})
	t.Run("exactamente 24 h antes: se aprueba sola", func(t *testing.T) {
		s := aceptada(t, 24*time.Hour)
		ev, err := s.Cancelar("cambio de planes", ahora)
		esperarEvento(t, ev, err, EventoCancelacionRegistrada)
		if s.Cancelacion.Estado != CancelacionAprobada || s.Traslado.Estado != TrasladoCancelado || s.Estado != SolicitudAceptada {
			t.Fatalf("solicitud inesperada: %+v %+v", s.Cancelacion, s.Traslado)
		}
	})
	t.Run("menos de 24 h: pendiente de aprobación", func(t *testing.T) {
		s := aceptada(t, 24*time.Hour-time.Second)
		ev, err := s.Cancelar("cambio de planes", ahora)
		esperarEvento(t, ev, err, EventoCancelacionRegistrada)
		if s.Cancelacion.Estado != CancelacionPendienteAprobacion || s.Traslado.Estado != TrasladoProgramado {
			t.Fatalf("solicitud inesperada: %+v %+v", s.Cancelacion, s.Traslado)
		}
		_, err = s.Cancelar("otra vez", ahora)
		esperarError(t, err, ErrEstadoNoPermite)
	})
	t.Run("traslado asignado también se puede cancelar", func(t *testing.T) {
		s := aceptada(t, 48*time.Hour)
		s.Traslado.Estado = TrasladoAsignado
		_, err := s.Cancelar("cambio de planes", ahora)
		if err != nil || s.Traslado.Estado != TrasladoCancelado {
			t.Fatalf("err = %v, traslado = %s", err, s.Traslado.Estado)
		}
	})
	t.Run("traslado en curso o finalizado no se cancela", func(t *testing.T) {
		for _, estado := range []EstadoTraslado{TrasladoEnCurso, TrasladoCompletado, TrasladoCancelado, TrasladoNoRealizado, TrasladoInterrumpido} {
			s := aceptada(t, 48*time.Hour)
			s.Traslado.Estado = estado
			_, err := s.Cancelar("cambio de planes", ahora)
			esperarError(t, err, ErrEstadoNoPermite)
		}
	})
	t.Run("solicitud en estado final", func(t *testing.T) {
		s := recibida(t, nil)
		_, _ = s.Rechazar("no", ahora)
		_, err := s.Cancelar("cambio de planes", ahora)
		esperarError(t, err, ErrEstadoNoPermite)
	})
}

func pendiente(t *testing.T) *Solicitud {
	t.Helper()
	s := aceptada(t, 3*time.Hour)
	if _, err := s.Cancelar("cambio de planes", ahora); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestResolverCancelacionTardia(t *testing.T) {
	t.Run("aprueba: traslado cancelado", func(t *testing.T) {
		s := pendiente(t)
		ev, err := s.ResolverCancelacionTardia(true, ahora)
		esperarEvento(t, ev, err, EventoTrasladoEstado)
		if s.Cancelacion.Estado != CancelacionAprobada || s.Traslado.Estado != TrasladoCancelado {
			t.Fatalf("solicitud inesperada: %+v %+v", s.Cancelacion, s.Traslado)
		}
	})
	t.Run("rechaza: el traslado sigue", func(t *testing.T) {
		s := pendiente(t)
		ev, err := s.ResolverCancelacionTardia(false, ahora)
		esperarEvento(t, ev, err, EventoCancelacionRegistrada)
		if s.Cancelacion.Estado != CancelacionRechazada || s.Traslado.Estado != TrasladoProgramado {
			t.Fatalf("solicitud inesperada: %+v %+v", s.Cancelacion, s.Traslado)
		}
		// Rechazada la cancelación, el cliente puede volver a pedirla.
		if _, err := s.Cancelar("de nuevo", ahora); err != nil {
			t.Fatalf("nueva cancelación: %v", err)
		}
	})
	t.Run("sin cancelación pendiente", func(t *testing.T) {
		_, err := aceptada(t, 48*time.Hour).ResolverCancelacionTardia(true, ahora)
		esperarError(t, err, ErrEstadoNoPermite)
	})
}

func TestRegistrarAlertaClimatica(t *testing.T) {
	_, err := recibida(t, nil).RegistrarAlertaClimatica(AlertaClimatica{Estado: AlertaNoVerificado}, ahora)
	esperarError(t, err, ErrEstadoNoPermite)

	s := aceptada(t, 3*time.Hour)
	ev, err := s.RegistrarAlertaClimatica(AlertaClimatica{Estado: AlertaNoOperable, Motivos: []string{"HIGH_WIND"}}, ahora)
	esperarEvento(t, ev, err, EventoAlertaClimatica)
	if s.Traslado.Alerta.Estado != AlertaNoOperable || s.Version != 3 {
		t.Fatalf("solicitud inesperada: %+v", s)
	}
}

func TestRequiereConsultaClima(t *testing.T) {
	extremo := ExtremoOrigen
	casos := []struct {
		nombre  string
		faltan  time.Duration
		extremo *Extremo
		espera  bool
	}{
		{"exactamente 6 h", 6 * time.Hour, &extremo, true},
		{"un segundo más de 6 h", 6*time.Hour + time.Second, &extremo, false},
		{"dentro de 1 h", time.Hour, &extremo, true},
		{"sin aeropuerto", time.Hour, nil, false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			s := recibida(t, en(c.faltan))
			if _, err := s.Aceptar("tra-1", ahora, c.extremo); err != nil {
				t.Fatal(err)
			}
			if got := s.RequiereConsultaClima(ahora); got != c.espera {
				t.Fatalf("RequiereConsultaClima = %v, se esperaba %v", got, c.espera)
			}
		})
	}
	t.Run("hora ya pasada", func(t *testing.T) {
		s := aceptada(t, time.Hour)
		s.Traslado.AeropuertoExtremo = &extremo
		if s.RequiereConsultaClima(ahora.Add(time.Hour)) {
			t.Fatal("no se consulta para una hora que ya pasó")
		}
	})
	t.Run("sin traslado", func(t *testing.T) {
		if recibida(t, en(time.Hour)).RequiereConsultaClima(ahora) {
			t.Fatal("sin traslado no hay consulta")
		}
	})
}
