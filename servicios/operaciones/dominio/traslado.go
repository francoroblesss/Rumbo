package dominio

import "time"

// Traslado es el servicio comprometido que nace al aceptar la solicitud. Es una entidad dentro
// del agregado Solicitud: solo cambia a través de ella.
type Traslado struct {
	ID                      string // "tra-" + uuid
	Estado                  EstadoTraslado
	HoraLlegadaComprometida time.Time  // en una reserva externa, el fin de la franja
	FranjaInicio            *time.Time // solo en una reserva externa (ADR-021)
	DemoraMinutos           *int
	FinalizadoEn            *time.Time
	DetalleResultado        *string
	AeropuertoExtremo       *Extremo
	Alerta                  *AlertaClimatica
}

// VentanaClima es cuánto antes de la hora comprometida se consulta el clima (R-41).
const VentanaClima = 6 * time.Hour

// DentroDeVentanaClima indica si la hora cae dentro de las próximas 6 h (sin haber pasado).
func DentroDeVentanaClima(hora, ahora time.Time) bool {
	falta := hora.Sub(ahora)
	return falta > 0 && falta <= VentanaClima
}

// cancelable indica si el traslado todavía no empezó ni terminó (R-17: un tramo iniciado no se
// cancela).
func (t *Traslado) cancelable() bool {
	return t.Estado == TrasladoProgramado || t.Estado == TrasladoAsignado
}
