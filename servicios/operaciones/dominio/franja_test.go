package dominio

import (
	"testing"
	"time"
)

// cordoba es -03:00 sin depender de la base de zonas horarias del sistema.
var cordoba = time.FixedZone("-03", -3*60*60)

// calendario de prueba: franjas de 2 h entre las 8 y las 20, 3 traslados y 1000 kg por franja,
// 2 h de anticipación mínima y 30 días de horizonte. ahora = 15/10 09:00 en Córdoba.
var calendario = Calendario{AperturaHora: 8, CierreHora: 20, DuracionHoras: 2, CupoTraslados: 3, CupoKg: 1000,
	Anticipacion: 2 * time.Hour, Horizonte: 30 * 24 * time.Hour, Zona: cordoba}

func local(dia, hora, minuto int) time.Time {
	return time.Date(2026, 10, dia, hora, minuto, 0, 0, cordoba)
}

func TestCalendarioValidar(t *testing.T) {
	f, err := calendario.Validar(local(16, 10, 0), ahora)
	if err != nil {
		t.Fatalf("franja válida: %v", err)
	}
	if !f.Inicio.Equal(local(16, 10, 0)) || !f.Fin.Equal(local(16, 12, 0)) || f.CupoTraslados != 3 || f.CupoKg != 1000 {
		t.Fatalf("franja inesperada: %+v", f)
	}

	casos := map[string]struct {
		inicio   time.Time
		esperado error
	}{
		"no alineada (minutos)":        {local(16, 10, 30), ErrFranjaInvalida},
		"no alineada (hora impar)":     {local(16, 9, 0), ErrFranjaInvalida},
		"antes de la apertura":         {local(16, 6, 0), ErrFranjaInvalida},
		"termina después del cierre":   {local(16, 20, 0), ErrFranjaInvalida},
		"ya pasó":                      {local(15, 8, 0), ErrHorarioEnElPasado},
		"menos de 2 h de anticipación": {local(15, 10, 0), ErrAnticipacionInsuficiente},
		"fuera del horizonte":          {local(16, 10, 0).AddDate(0, 0, 31), ErrFranjaInvalida},
	}
	for nombre, c := range casos {
		t.Run(nombre, func(t *testing.T) {
			_, err := calendario.Validar(c.inicio, ahora)
			esperarError(t, err, c.esperado)
		})
	}
}

func TestCalendarioDelDia(t *testing.T) {
	// Hoy (15/10, 09:00): la de las 10 no llega a las 2 h de anticipación; quedan de 12 a 18.
	hoy, err := calendario.DelDia(2026, time.October, 15, ahora)
	if err != nil || len(hoy) != 4 || !hoy[0].Inicio.Equal(local(15, 12, 0)) {
		t.Fatalf("hoy: %d franjas, err = %v", len(hoy), err)
	}
	maniana, _ := calendario.DelDia(2026, time.October, 16, ahora)
	if len(maniana) != 6 || !maniana[5].Fin.Equal(local(16, 20, 0)) {
		t.Fatalf("mañana: %d franjas", len(maniana))
	}
	_, err = calendario.DelDia(2026, time.October, 14, ahora)
	esperarError(t, err, ErrFranjaInvalida)
	_, err = calendario.DelDia(2026, time.December, 1, ahora)
	esperarError(t, err, ErrFranjaInvalida)
}

func TestFranjaReservarYLiberar(t *testing.T) {
	f := calendario.Nueva(local(16, 10, 0))
	esperarError(t, f.Reservar(1500), ErrExcedeCapacidad)
	for _, peso := range []float64{400, 400} {
		if err := f.Reservar(peso); err != nil {
			t.Fatalf("reservar %v: %v", peso, err)
		}
	}
	if f.Admite(300) || !f.Admite(200) {
		t.Fatalf("quedan %v kg: admite 300 = %v, admite 200 = %v", f.KgDisponibles(), f.Admite(300), f.Admite(200))
	}
	esperarError(t, f.Reservar(300), ErrSinDisponibilidad)
	if err := f.Reservar(0); err != nil { // sin peso informado ocupa un lugar igual
		t.Fatal(err)
	}
	esperarError(t, f.Reservar(0), ErrSinDisponibilidad) // no quedan traslados
	f.Liberar(400)
	if f.TrasladosDisponibles() != 1 || f.KgDisponibles() != 600 {
		t.Fatalf("después de liberar: %d traslados, %v kg", f.TrasladosDisponibles(), f.KgDisponibles())
	}
}

func reserva(t *testing.T, peso *float64) (*Solicitud, *Franja) {
	t.Helper()
	f := calendario.Nueva(local(17, 14, 0))
	d := datosValidos(nil)
	d.Cosa.PesoKg = peso
	s, ev, err := NuevaReservaExterna("sol-1", "tra-1", "org-1", d, &f, ahora)
	esperarEvento(t, ev, err, EventoAceptada)
	return s, &f
}

func TestNuevaReservaExterna(t *testing.T) {
	peso := 350.0
	s, f := reserva(t, &peso)
	if s.Estado != SolicitudAceptada || s.Origen != OrigenExterno || s.Version != 1 || s.Traslado == nil ||
		s.Traslado.Estado != TrasladoProgramado || !s.Traslado.HoraLlegadaComprometida.Equal(f.Fin) ||
		!s.Traslado.FranjaInicio.Equal(f.Inicio) {
		t.Fatalf("reserva inesperada: %+v %+v", s, s.Traslado)
	}
	if f.TrasladosReservados != 1 || f.KgReservados != 350 {
		t.Fatalf("la franja no descontó: %+v", f)
	}

	t.Run("sin lugar no crea nada", func(t *testing.T) {
		llena := calendario.Nueva(local(17, 14, 0))
		llena.TrasladosReservados = llena.CupoTraslados
		_, _, err := NuevaReservaExterna("sol-2", "tra-2", "org-1", datosValidos(nil), &llena, ahora)
		esperarError(t, err, ErrSinDisponibilidad)
	})
	t.Run("datos inválidos no toman lugar", func(t *testing.T) {
		libre := calendario.Nueva(local(17, 14, 0))
		d := datosValidos(nil)
		d.Cosa.Descripcion = ""
		_, _, err := NuevaReservaExterna("sol-3", "tra-3", "org-1", d, &libre, ahora)
		esperarError(t, err, ErrDatosInvalidos)
		if libre.TrasladosReservados != 0 {
			t.Fatal("tomó lugar con datos inválidos")
		}
	})
}

func TestCancelarReservaExterna(t *testing.T) {
	t.Run("con anticipación: se cancela y libera la franja", func(t *testing.T) {
		peso := 100.0
		s, _ := reserva(t, &peso)
		ev, err := s.Cancelar("ya no hace falta", ahora)
		esperarEvento(t, ev, err, EventoCancelacionRegistrada)
		inicio, kg, libera := s.LiberaFranja()
		if s.Cancelacion.Estado != CancelacionAprobada || s.Cancelacion.Tardia || !libera || kg != 100 ||
			!inicio.Equal(local(17, 14, 0)) {
			t.Fatalf("cancelación inesperada: %+v, libera = %v %v %v", s.Cancelacion, inicio, kg, libera)
		}
	})
	t.Run("tardía: se aplica igual, sin esperar al operador", func(t *testing.T) {
		s, _ := reserva(t, nil)
		ev, err := s.Cancelar("ya no hace falta", local(17, 12, 0))
		esperarEvento(t, ev, err, EventoCancelacionRegistrada)
		if s.Cancelacion.Estado != CancelacionAprobada || !s.Cancelacion.Tardia || s.Traslado.Estado != TrasladoCancelado {
			t.Fatalf("cancelación inesperada: %+v %+v", s.Cancelacion, s.Traslado)
		}
	})
	t.Run("una solicitud interna tardía sigue esperando al operador", func(t *testing.T) {
		s := aceptada(t, 3*time.Hour)
		if _, err := s.Cancelar("cambio de planes", ahora); err != nil {
			t.Fatal(err)
		}
		if _, _, libera := s.LiberaFranja(); libera || s.Cancelacion.Estado != CancelacionPendienteAprobacion || !s.Cancelacion.Tardia {
			t.Fatalf("cancelación inesperada: %+v", s.Cancelacion)
		}
	})
}
