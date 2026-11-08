package mensajeria

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/francoroblesss/Rumbo/servicios/comun/mensajeria"
	"github.com/francoroblesss/Rumbo/servicios/operaciones/puertos"
)

type busquedaFalsa struct {
	puertos.Busqueda
	err     error
	org     string
	vista   puertos.Vista
	ocurrio time.Time
}

func (b *busquedaFalsa) IndexarEvento(_ context.Context, org string, v puertos.Vista, ocurrio time.Time) error {
	b.org, b.vista, b.ocurrio = org, v, ocurrio
	return b.err
}

func TestManejadorIndexador(t *testing.T) {
	ctx := context.Background()
	cuando := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	s := mensajeria.Sobre{Tipo: "operaciones.traslado.estado_cambiado", AgregadoVersion: 3, OcurridoEn: cuando,
		Datos: json.RawMessage(`{"organizacion_id":"org-1","origen":"externo","solicitud":{"id":"sol-1","version":3}}`)}

	b := &busquedaFalsa{}
	if err := NuevoManejadorIndexador(b)(ctx, s); err != nil || b.org != "org-1" || b.vista.ID != "sol-1" ||
		b.vista.Version != 3 || !b.ocurrio.Equal(cuando) {
		t.Fatalf("err = %v, %+v", err, b)
	}

	// Solr caído: transitorio (reintento con backoff).
	b.err = puertos.ErrBusquedaNoDisponible
	if err := NuevoManejadorIndexador(b)(ctx, s); err == nil || mensajeria.EsPermanente(err) {
		t.Fatalf("Solr caído tiene que ser transitorio: %v", err)
	}

	// Datos ilegibles o sin solicitud: a la DLQ.
	for _, datos := range []string{`{`, `{"organizacion_id":"org-1","solicitud":{}}`, `{"solicitud":{"id":"sol-1"}}`} {
		s.Datos = json.RawMessage(datos)
		if err := NuevoManejadorIndexador(&busquedaFalsa{})(ctx, s); !mensajeria.EsPermanente(err) {
			t.Errorf("%s: se esperaba permanente, vino %v", datos, err)
		}
	}

}
