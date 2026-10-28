package dominio

import "time"

// Franja es una ventana de llegada en la que Rumbo se compromete a entregar lo que reserva un
// sistema externo (capacidad publicada, ADR-021). Tiene un cupo de traslados y de kg: reservar
// descuenta del cupo y cancelar lo devuelve. Es el recurso que dos reservas simultáneas no pueden
// tomar a la vez (D4, ADR-022).
type Franja struct {
	Inicio              time.Time
	Fin                 time.Time
	CupoTraslados       int
	CupoKg              float64
	TrasladosReservados int
	KgReservados        float64
}

// TrasladosDisponibles es cuántas reservas más entran en la franja.
func (f Franja) TrasladosDisponibles() int { return max(f.CupoTraslados-f.TrasladosReservados, 0) }

// KgDisponibles es cuánto peso más entra en la franja.
func (f Franja) KgDisponibles() float64 { return max(f.CupoKg-f.KgReservados, 0) }

// Admite indica si una carga de pesoKg entra hoy en la franja.
func (f Franja) Admite(pesoKg float64) bool {
	return f.TrasladosDisponibles() >= 1 && pesoKg <= f.KgDisponibles()
}

// Reservar toma un lugar de la franja. Una carga más pesada que el cupo total nunca va a entrar
// (EXCEDE_CAPACIDAD); si entra pero ya no queda lugar, es SIN_DISPONIBILIDAD.
func (f *Franja) Reservar(pesoKg float64) error {
	if pesoKg > f.CupoKg {
		return ErrExcedeCapacidad
	}
	if !f.Admite(pesoKg) {
		return ErrSinDisponibilidad
	}
	f.TrasladosReservados++
	f.KgReservados += pesoKg
	return nil
}

// Liberar devuelve el lugar de un traslado cancelado.
func (f *Franja) Liberar(pesoKg float64) {
	f.TrasladosReservados = max(f.TrasladosReservados-1, 0)
	f.KgReservados = max(f.KgReservados-pesoKg, 0)
}

// Calendario son las reglas con las que se arman las franjas: horario de atención, duración,
// cupo de cada franja, anticipación mínima para reservar y hasta cuántos días adelante se puede.
// Las franjas no se cargan a mano: existen todas las que caen en el horario y se guardan recién
// cuando alguien reserva en ellas.
type Calendario struct {
	AperturaHora  int // hora local de inicio de la primera franja
	CierreHora    int // hora local de fin de la última franja
	DuracionHoras int
	CupoTraslados int
	CupoKg        float64
	Anticipacion  time.Duration
	Horizonte     time.Duration
	Zona          *time.Location // hora en la que opera Rumbo (-03:00)
}

// Nueva arma la franja que empieza en inicio, con el cupo completo. No valida: ver Validar.
func (c Calendario) Nueva(inicio time.Time) Franja {
	return Franja{
		Inicio:        inicio.UTC(),
		Fin:           inicio.UTC().Add(time.Duration(c.DuracionHoras) * time.Hour),
		CupoTraslados: c.CupoTraslados,
		CupoKg:        c.CupoKg,
	}
}

// Validar verifica que inicio sea el comienzo de una franja que todavía se puede reservar y
// devuelve esa franja con el cupo completo.
func (c Calendario) Validar(inicio, ahora time.Time) (Franja, error) {
	if !c.alineada(inicio) {
		return Franja{}, ErrFranjaInvalida
	}
	falta := inicio.Sub(ahora)
	switch {
	case falta <= 0:
		return Franja{}, ErrHorarioEnElPasado
	case falta < c.Anticipacion:
		return Franja{}, ErrAnticipacionInsuficiente
	case falta > c.Horizonte:
		return Franja{}, ErrFranjaInvalida
	}
	return c.Nueva(inicio), nil
}

// DelDia devuelve, con el cupo completo, las franjas de la fecha (en hora local) que todavía se
// pueden reservar. Una fecha fuera del horizonte es ErrFranjaInvalida.
func (c Calendario) DelDia(anio int, mes time.Month, dia int, ahora time.Time) ([]Franja, error) {
	medianoche := time.Date(anio, mes, dia, 0, 0, 0, 0, c.Zona)
	hoy := ahora.In(c.Zona)
	if medianoche.Before(time.Date(hoy.Year(), hoy.Month(), hoy.Day(), 0, 0, 0, 0, c.Zona)) ||
		medianoche.Sub(ahora) > c.Horizonte {
		return nil, ErrFranjaInvalida
	}
	var franjas []Franja
	for h := c.AperturaHora; h+c.DuracionHoras <= c.CierreHora; h += c.DuracionHoras {
		if f, err := c.Validar(medianoche.Add(time.Duration(h)*time.Hour), ahora); err == nil {
			franjas = append(franjas, f)
		}
	}
	return franjas, nil
}

// alineada indica si inicio, en hora local, es el comienzo exacto de una franja del horario.
func (c Calendario) alineada(inicio time.Time) bool {
	local := inicio.In(c.Zona)
	h := local.Hour()
	return local.Minute() == 0 && local.Second() == 0 && local.Nanosecond() == 0 &&
		h >= c.AperturaHora && h+c.DuracionHoras <= c.CierreHora && (h-c.AperturaHora)%c.DuracionHoras == 0
}
