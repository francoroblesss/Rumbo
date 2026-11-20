package aplicacion

import (
	"context"
	"log/slog"

	"github.com/francoroblesss/Rumbo/servicios/operaciones/puertos"
)

// ClimaActual muestra en la web las condiciones actuales del aeropuerto (§13.5, ADR-034). Usa el
// mismo adaptador que las reservas, con su timeout, reintento, breaker y caché de 5 min: la web no
// suma pedidos al proveedor. Solo lee: no registra alertas, no escribe en MySQL y no cuenta en
// rumbo_clima_consultas_total, que es la métrica de la regla de las reservas (§13.4).
type ClimaActual struct {
	clima puertos.ConsultaClima
	icao  string
}

var _ puertos.ClimaActual = (*ClimaActual)(nil)

func NuevoClimaActual(clima puertos.ConsultaClima, icao string) *ClimaActual {
	return &ClimaActual{clima: clima, icao: icao}
}

// Consultar devuelve las condiciones o el error del adaptador (envuelve ErrClimaNoDisponible).
// No hay fallback: un clima inventado o viejo mostrado como actual sería engañar (teórico 7).
func (s *ClimaActual) Consultar(ctx context.Context) (puertos.ClimaAeropuerto, error) {
	o, err := s.clima.Operabilidad(ctx, s.icao)
	if err != nil {
		slog.WarnContext(ctx, "clima no disponible para la web", slog.String("error", err.Error()))
		return puertos.ClimaAeropuerto{}, err
	}
	motivos := o.Motivos
	if motivos == nil {
		motivos = []string{}
	}
	c := o.Condiciones
	return puertos.ClimaAeropuerto{
		Aeropuerto:  puertos.AeropuertoClima{ICAO: s.icao, Nombre: o.AeropuertoNombre},
		ObservadoEn: hora(o.ObservadoEn),
		Operable:    o.Operable,
		Motivos:     motivos,
		Condiciones: puertos.CondicionesActual{
			TemperaturaC: c.TemperaturaC, VientoKmh: c.VientoKmh, RafagasKmh: c.RafagasKmh,
			DireccionVientoGrados: c.DireccionVientoGrados, VisibilidadM: c.VisibilidadM,
			PrecipitacionMm: c.PrecipitacionMm, Tormenta: c.Tormenta,
		},
	}, nil
}
