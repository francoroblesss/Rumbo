package dominio

import (
	"errors"
	"testing"
	"time"
)

var ahora = time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)

// palabras son las de AEROPUERTO_PALABRAS por defecto.
var palabras = []string{"aeropuerto", "saco", "pajas blancas"}

func datosValidos() DatosSolicitud {
	return DatosSolicitud{
		Cosa:  Cosa{Descripcion: "Cajas", Cantidad: 20},
		Desde: Ubicacion{Nombre: "Sucursal Centro", Direccion: "Av. Colón 1234"},
		Hasta: Ubicacion{Nombre: "Sucursal Cerro", Direccion: "Av. R. Núñez 4500"},
	}
}

// reservada es una reserva cuya franja termina (hora comprometida) dentro de faltan.
func reservada(t *testing.T, faltan time.Duration, d DatosSolicitud) *Solicitud {
	t.Helper()
	fin := ahora.Add(faltan)
	franja := &Franja{Inicio: fin.Add(-2 * time.Hour), Fin: fin, CupoTraslados: 4, CupoKg: 1000}
	s, _, err := NuevaReservaExterna("sol-1", "tra-1", "org-1", d, franja, palabras, ahora)
	if err != nil {
		t.Fatalf("reservar: %v", err)
	}
	return s
}

func alAeropuerto() DatosSolicitud {
	d := datosValidos()
	d.Hasta = Ubicacion{Nombre: "Aeropuerto Internacional de Córdoba", Direccion: "Av. La Voz del Interior 8500"}
	return d
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

func TestNuevaReservaExternaValidaDatos(t *testing.T) {
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
			d := datosValidos()
			romper(&d)
			franja := &Franja{Inicio: ahora, Fin: ahora.Add(2 * time.Hour), CupoTraslados: 1, CupoKg: 100}
			_, _, err := NuevaReservaExterna("sol-1", "tra-1", "org-1", d, franja, palabras, ahora)
			esperarError(t, err, ErrDatosInvalidos)
			if franja.TrasladosReservados != 0 {
				t.Fatal("con datos inválidos no se toma lugar")
			}
		})
	}
}

func TestExtremoAeropuerto(t *testing.T) {
	comun := Ubicacion{Nombre: "Sucursal Centro", Direccion: "Av. Colón 1234"}
	casos := []struct {
		nombre          string
		origen, destino Ubicacion
		esperado        *Extremo
	}{
		{"nombre con tilde", comun, Ubicacion{Nombre: "Aeropuerto Internacional de Córdoba", Direccion: "Camino a Pajas"}, ptr(ExtremoDestino)},
		{"mayúsculas", comun, Ubicacion{Nombre: "AEROPUERTO", Direccion: "Ruta 9"}, ptr(ExtremoDestino)},
		{"mayúsculas con tilde en la palabra", comun, Ubicacion{Nombre: "AEROPUÉRTO", Direccion: "Ruta 9"}, ptr(ExtremoDestino)},
		{"pajas blancas", comun, Ubicacion{Nombre: "Aeropuerto Pajas Blancas", Direccion: "Ruta 9"}, ptr(ExtremoDestino)},
		{"código en la dirección", comun, Ubicacion{Nombre: "Terminal de cargas", Direccion: "Av. La Voz del Interior 8500 (SACO)"}, ptr(ExtremoDestino)},
		{"el aeropuerto como origen", Ubicacion{Nombre: "Aeropuerto Internacional de Córdoba", Direccion: "Av. La Voz del Interior 8500"},
			Ubicacion{Nombre: "Aeropuerto de Mendoza", Direccion: "Ruta 40"}, ptr(ExtremoOrigen)},
		{"destino común", comun, Ubicacion{Nombre: "Sucursal Cerro", Direccion: "Av. R. Núñez 4500"}, nil},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got := ExtremoAeropuerto(c.origen, c.destino, palabras)
			if (got == nil) != (c.esperado == nil) || (got != nil && *got != *c.esperado) {
				t.Fatalf("extremo = %v, se esperaba %v", valor(got), valor(c.esperado))
			}
		})
	}
	t.Run("sin palabras no marca nada", func(t *testing.T) {
		if ExtremoAeropuerto(comun, Ubicacion{Nombre: "Aeropuerto", Direccion: "x"}, nil) != nil {
			t.Fatal("se esperaba nil")
		}
	})
	t.Run("la reserva guarda el extremo", func(t *testing.T) {
		s := reservada(t, 48*time.Hour, alAeropuerto())
		if s.Traslado.AeropuertoExtremo == nil || *s.Traslado.AeropuertoExtremo != ExtremoDestino {
			t.Fatalf("extremo = %v", valor(s.Traslado.AeropuertoExtremo))
		}
		if reservada(t, 48*time.Hour, datosValidos()).Traslado.AeropuertoExtremo != nil {
			t.Fatal("un destino común no se marca")
		}
	})
}

func ptr(e Extremo) *Extremo { return &e }

func valor(e *Extremo) string {
	if e == nil {
		return "nil"
	}
	return string(*e)
}

func TestIniciarYCompletarTraslado(t *testing.T) {
	t.Run("PROGRAMADO → EN_CURSO → COMPLETADO", func(t *testing.T) {
		s := reservada(t, 48*time.Hour, datosValidos())
		ev, err := s.IniciarTraslado(ahora)
		esperarEvento(t, ev, err, EventoTrasladoEstado)
		if s.Traslado.Estado != TrasladoEnCurso || s.Version != 2 || s.Traslado.FinalizadoEn != nil {
			t.Fatalf("solicitud inesperada: %+v %+v", s, s.Traslado)
		}
		despues := ahora.Add(time.Hour)
		ev, err = s.CompletarTraslado(despues)
		esperarEvento(t, ev, err, EventoTrasladoEstado)
		if s.Traslado.Estado != TrasladoCompletado || s.Version != 3 || !s.Traslado.FinalizadoEn.Equal(despues) || !s.ActualizadoEn.Equal(despues) {
			t.Fatalf("solicitud inesperada: %+v %+v", s, s.Traslado)
		}
	})
	t.Run("transiciones inválidas no cambian nada", func(t *testing.T) {
		casos := []struct {
			nombre string
			desde  EstadoTraslado
			paso   func(*Solicitud) (Evento, error)
		}{
			{"CANCELADO → EN_CURSO", TrasladoCancelado, func(s *Solicitud) (Evento, error) { return s.IniciarTraslado(ahora) }},
			{"COMPLETADO → EN_CURSO", TrasladoCompletado, func(s *Solicitud) (Evento, error) { return s.IniciarTraslado(ahora) }},
			{"EN_CURSO → EN_CURSO", TrasladoEnCurso, func(s *Solicitud) (Evento, error) { return s.IniciarTraslado(ahora) }},
			{"PROGRAMADO → COMPLETADO", TrasladoProgramado, func(s *Solicitud) (Evento, error) { return s.CompletarTraslado(ahora) }},
			{"CANCELADO → COMPLETADO", TrasladoCancelado, func(s *Solicitud) (Evento, error) { return s.CompletarTraslado(ahora) }},
		}
		for _, c := range casos {
			t.Run(c.nombre, func(t *testing.T) {
				s := reservada(t, 48*time.Hour, datosValidos())
				s.Traslado.Estado = c.desde
				_, err := c.paso(s)
				esperarError(t, err, ErrEstadoNoPermite)
				if s.Traslado.Estado != c.desde || s.Version != 1 {
					t.Fatalf("un error no tiene que cambiar nada: %s, versión %d", s.Traslado.Estado, s.Version)
				}
			})
		}
	})
	t.Run("EstadoPedido", func(t *testing.T) {
		s := reservada(t, 48*time.Hour, datosValidos())
		if !s.EstadoPedido(TrasladoProgramado) || s.EstadoPedido(TrasladoEnCurso) {
			t.Fatal("EstadoPedido no refleja el estado actual")
		}
		if (&Solicitud{}).EstadoPedido(TrasladoEnCurso) {
			t.Fatal("sin traslado no hay estado")
		}
	})
}

func TestCancelar(t *testing.T) {
	t.Run("motivo obligatorio", func(t *testing.T) {
		_, err := reservada(t, 48*time.Hour, datosValidos()).Cancelar(" ", ahora)
		esperarError(t, err, ErrDatosInvalidos)
	})
	t.Run("con 24 h o más: aprobada y no tardía", func(t *testing.T) {
		s := reservada(t, 24*time.Hour, datosValidos())
		ev, err := s.Cancelar("cambio de planes", ahora)
		esperarEvento(t, ev, err, EventoCancelacionRegistrada)
		if s.Cancelacion.Estado != CancelacionAprobada || s.Cancelacion.Tardia || s.Traslado.Estado != TrasladoCancelado || s.Version != 2 {
			t.Fatalf("solicitud inesperada: %+v %+v", s.Cancelacion, s.Traslado)
		}
	})
	t.Run("con menos de 24 h: aprobada y tardía", func(t *testing.T) {
		s := reservada(t, 24*time.Hour-time.Second, datosValidos())
		ev, err := s.Cancelar("cambio de planes", ahora)
		esperarEvento(t, ev, err, EventoCancelacionRegistrada)
		if s.Cancelacion.Estado != CancelacionAprobada || !s.Cancelacion.Tardia || s.Traslado.Estado != TrasladoCancelado {
			t.Fatalf("solicitud inesperada: %+v %+v", s.Cancelacion, s.Traslado)
		}
	})
	t.Run("EN_CURSO, COMPLETADO o ya CANCELADO no se cancela (R-17, R-47)", func(t *testing.T) {
		for _, estado := range []EstadoTraslado{TrasladoEnCurso, TrasladoCompletado, TrasladoCancelado} {
			s := reservada(t, 48*time.Hour, datosValidos())
			s.Traslado.Estado = estado
			_, err := s.Cancelar("cambio de planes", ahora)
			esperarError(t, err, ErrEstadoNoPermite)
			if s.Cancelacion != nil || s.Version != 1 {
				t.Fatalf("%s: un error no tiene que cambiar nada", estado)
			}
		}
	})
	t.Run("una solicitud sin traslado no se cancela", func(t *testing.T) {
		_, err := (&Solicitud{Estado: "RECIBIDA"}).Cancelar("motivo", ahora)
		esperarError(t, err, ErrEstadoNoPermite)
	})
}

func TestAlertaIgual(t *testing.T) {
	viento := &AlertaClimatica{Estado: AlertaNoOperable, Motivos: []string{"HIGH_WIND"}, ObservadoEn: &ahora}
	otraHora := ahora.Add(time.Minute)
	casos := []struct {
		nombre string
		a, b   *AlertaClimatica
		igual  bool
	}{
		{"mismo estado y motivos, otra hora de observación", viento,
			&AlertaClimatica{Estado: AlertaNoOperable, Motivos: []string{"HIGH_WIND"}, ObservadoEn: &otraHora}, true},
		{"otros motivos", viento, &AlertaClimatica{Estado: AlertaNoOperable, Motivos: []string{"HIGH_WIND", "LOW_VISIBILITY"}}, false},
		{"otro estado", viento, &AlertaClimatica{Estado: AlertaNoVerificado}, false},
		{"no verificado sin motivos (nil y vacío)", &AlertaClimatica{Estado: AlertaNoVerificado},
			&AlertaClimatica{Estado: AlertaNoVerificado, Motivos: []string{}}, true},
		{"contra ninguna", viento, nil, false},
		{"ninguna contra ninguna", nil, nil, true},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := c.a.Igual(c.b); got != c.igual {
				t.Fatalf("Igual = %v, se esperaba %v", got, c.igual)
			}
		})
	}
}

func TestRegistrarAlertaClimatica(t *testing.T) {
	_, err := (&Solicitud{}).RegistrarAlertaClimatica(AlertaClimatica{Estado: AlertaNoVerificado}, ahora)
	esperarError(t, err, ErrEstadoNoPermite)

	s := reservada(t, 3*time.Hour, alAeropuerto())
	ev, err := s.RegistrarAlertaClimatica(AlertaClimatica{Estado: AlertaNoOperable, Motivos: []string{"HIGH_WIND"}}, ahora)
	esperarEvento(t, ev, err, EventoAlertaClimatica)
	if s.Traslado.Alerta.Estado != AlertaNoOperable || s.Version != 2 {
		t.Fatalf("solicitud inesperada: %+v", s)
	}
}

func TestRequiereConsultaClima(t *testing.T) {
	casos := []struct {
		nombre string
		faltan time.Duration
		datos  DatosSolicitud
		espera bool
	}{
		{"exactamente 6 h", 6 * time.Hour, alAeropuerto(), true},
		{"un segundo más de 6 h", 6*time.Hour + time.Second, alAeropuerto(), false},
		{"dentro de 1 h", time.Hour, alAeropuerto(), true},
		{"sin aeropuerto", time.Hour, datosValidos(), false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := reservada(t, c.faltan, c.datos).RequiereConsultaClima(ahora); got != c.espera {
				t.Fatalf("RequiereConsultaClima = %v, se esperaba %v", got, c.espera)
			}
		})
	}
	t.Run("hora ya pasada", func(t *testing.T) {
		if reservada(t, time.Hour, alAeropuerto()).RequiereConsultaClima(ahora.Add(time.Hour)) {
			t.Fatal("no se consulta para una hora que ya pasó")
		}
	})
	t.Run("traslado que ya no está PROGRAMADO", func(t *testing.T) {
		s := reservada(t, time.Hour, alAeropuerto())
		s.Traslado.Estado = TrasladoCancelado
		if s.RequiereConsultaClima(ahora) {
			t.Fatal("un traslado cancelado no consulta el clima")
		}
	})
	t.Run("sin traslado", func(t *testing.T) {
		if (&Solicitud{}).RequiereConsultaClima(ahora) {
			t.Fatal("sin traslado no hay consulta")
		}
	})
}
