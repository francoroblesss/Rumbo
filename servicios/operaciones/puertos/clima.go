package puertos

import "context"

// ClimaActual es el caso de uso de la web: las condiciones actuales del aeropuerto (§13.5, ADR-034).
type ClimaActual interface {
	Consultar(ctx context.Context) (ClimaAeropuerto, error) // ErrClimaNoDisponible si no se pudo saber
}

// ClimaAeropuerto es la respuesta de GET /clima, con exactamente los campos de §13.5.
type ClimaAeropuerto struct {
	Aeropuerto  AeropuertoClima   `json:"aeropuerto"`
	ObservadoEn string            `json:"observado_en"` // RFC 3339 con -03:00
	Operable    bool              `json:"operable"`
	Motivos     []string          `json:"motivos"` // nunca null: [] si es operable
	Condiciones CondicionesActual `json:"condiciones"`
}

type AeropuertoClima struct {
	ICAO   string `json:"icao"`
	Nombre string `json:"nombre"`
}

type CondicionesActual struct {
	TemperaturaC          float64 `json:"temperatura_c"`
	VientoKmh             float64 `json:"viento_kmh"`
	RafagasKmh            float64 `json:"rafagas_kmh"`
	DireccionVientoGrados int     `json:"direccion_viento_grados"`
	VisibilidadM          float64 `json:"visibilidad_m"`
	PrecipitacionMm       float64 `json:"precipitacion_mm"`
	Tormenta              bool    `json:"tormenta"`
}
