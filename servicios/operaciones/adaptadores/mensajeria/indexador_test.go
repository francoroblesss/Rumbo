package mensajeria

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/francoroblesss/Rumbo/servicios/comun/mensajeria"
	"github.com/francoroblesss/Rumbo/servicios/operaciones/dominio"
	"github.com/francoroblesss/Rumbo/servicios/operaciones/puertos"
)

type busquedaFalsa struct {
	puertos.Busqueda
	err        error
	org        string
	vista      puertos.Vista
	ocurridoEn time.Time
}

func (f *busquedaFalsa) IndexarEvento(_ context.Context, org string, v puertos.Vista, ocurridoEn time.Time) error {
	f.org, f.vista, f.ocurridoEn = org, v, ocurridoEn
	return f.err
}

func TestManejadorIndexador(t *testing.T) {
	ctx := context.Background()
	ocurrido := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	evento := func(datos string) mensajeria.Sobre {
		return mensajeria.Sobre{Tipo: "operaciones.traslado.estado_cambiado", AgregadoID: "sol-1", AgregadoVersion: 7,
			OcurridoEn: ocurrido, Datos: json.RawMessage(datos)}
	}
	valido := `{"organizacion_id":"org-1","origen":"externo","solicitud":{"id":"sol-1","version":6,"traslado":{"id":"tra-1","estado":"EN_CURSO"}}}`

	t.Run("usa la versión y la fecha del sobre", func(t *testing.T) {
		f := &busquedaFalsa{}
		if err := NuevoManejadorIndexador(f)(ctx, evento(valido)); err != nil {
			t.Fatal(err)
		}
		if f.org != "org-1" || f.vista.ID != "sol-1" || f.vista.Version != 7 || f.vista.Traslado.Estado != "EN_CURSO" || !f.ocurridoEn.Equal(ocurrido) {
			t.Fatalf("org = %q, vista = %+v, ocurrido = %v", f.org, f.vista, f.ocurridoEn)
		}
	})

	casos := []struct {
		nombre     string
		datos      string
		err        error
		permanente bool
	}{
		{"datos ilegibles → DLQ", `{"solicitud":`, nil, true},
		{"sin solicitud.id → DLQ", `{"organizacion_id":"org-1","solicitud":{}}`, nil, true},
		{"sin organizacion_id → DLQ", `{"solicitud":{"id":"sol-1"}}`, nil, true},
		{"vista inválida → DLQ", valido, fmt.Errorf("%w: fecha", dominio.ErrDatosInvalidos), true},
		{"Solr caído → reintento", valido, fmt.Errorf("%w: timeout", puertos.ErrBusquedaNoDisponible), false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			err := NuevoManejadorIndexador(&busquedaFalsa{err: c.err})(ctx, evento(c.datos))
			if err == nil || mensajeria.EsPermanente(err) != c.permanente {
				t.Fatalf("err = %v, permanente = %v", err, mensajeria.EsPermanente(err))
			}
		})
	}
}
