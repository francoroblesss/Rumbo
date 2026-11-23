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

func TestClimaActual(t *testing.T) {
	ctx := context.Background()
	observado := time.Date(2026, 10, 9, 14, 0, 0, 0, time.UTC)

	t.Run("operable: campos mapeados, hora con -03:00 y motivos vacíos", func(t *testing.T) {
		clima := &climaFalso{operabilidad: dominio.Operabilidad{
			Operable: true, ObservadoEn: observado, AeropuertoNombre: "Aeropuerto Taravella",
			Condiciones: &dominio.CondicionesClima{TemperaturaC: 22.4, VientoKmh: 18, RafagasKmh: 27.5,
				DireccionVientoGrados: 340, VisibilidadM: 10000, PrecipitacionMm: 0, Tormenta: false},
		}}
		c, err := NuevoClimaActual(clima, "SACO").Consultar(ctx)
		if err != nil {
			t.Fatal(err)
		}
		esperado := puertos.ClimaAeropuerto{
			Aeropuerto:  puertos.AeropuertoClima{ICAO: "SACO", Nombre: "Aeropuerto Taravella"},
			ObservadoEn: "2026-10-09T11:00:00-03:00", Operable: true, Motivos: []string{},
			Condiciones: puertos.CondicionesActual{TemperaturaC: 22.4, VientoKmh: 18, RafagasKmh: 27.5,
				DireccionVientoGrados: 340, VisibilidadM: 10000, PrecipitacionMm: 0, Tormenta: false},
		}
		if fmt.Sprintf("%#v", c) != fmt.Sprintf("%#v", esperado) {
			t.Fatalf("clima = %+v, se esperaba %+v", c, esperado)
		}
		// En el JSON, motivos tiene que ser [] y no null.
		crudo, _ := json.Marshal(c)
		var m map[string]any
		_ = json.Unmarshal(crudo, &m)
		if lista, ok := m["motivos"].([]any); !ok || len(lista) != 0 {
			t.Fatalf("motivos = %v", m["motivos"])
		}
	})

	t.Run("no operable: operable=false y los motivos", func(t *testing.T) {
		clima := &climaFalso{operabilidad: dominio.Operabilidad{Operable: false, Motivos: []string{"HIGH_WIND", "HIGH_GUSTS"}, ObservadoEn: observado,
			Condiciones: &dominio.CondicionesClima{VientoKmh: 58.3, RafagasKmh: 81}}}
		c, err := NuevoClimaActual(clima, "SACO").Consultar(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if c.Operable || fmt.Sprint(c.Motivos) != "[HIGH_WIND HIGH_GUSTS]" {
			t.Fatalf("operable = %v, motivos = %v", c.Operable, c.Motivos)
		}
	})

	t.Run("sin condiciones: ErrClimaNoDisponible (no se muestran ceros)", func(t *testing.T) {
		clima := &climaFalso{operabilidad: dominio.Operabilidad{Operable: true, Motivos: []string{}, ObservadoEn: observado}}
		if _, err := NuevoClimaActual(clima, "SACO").Consultar(ctx); !errors.Is(err, puertos.ErrClimaNoDisponible) {
			t.Fatalf("err = %v, se esperaba ErrClimaNoDisponible", err)
		}
	})

	t.Run("sin clima: ErrClimaNoDisponible", func(t *testing.T) {
		clima := &climaFalso{err: fmt.Errorf("%w: timeout", puertos.ErrClimaNoDisponible)}
		if _, err := NuevoClimaActual(clima, "SACO").Consultar(ctx); !errors.Is(err, puertos.ErrClimaNoDisponible) {
			t.Fatalf("error = %v", err)
		}
	})
}
