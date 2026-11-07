package mensajeria

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/francoroblesss/Rumbo/servicios/comun/mensajeria"
	"github.com/francoroblesss/Rumbo/servicios/operaciones/puertos"
)

type casosFalsos struct {
	puertos.Solicitudes
	err error
	id  string
}

func (f *casosFalsos) RevisarClimaDeReserva(_ context.Context, id string) error {
	f.id = id
	return f.err
}

func sobre(datos string) mensajeria.Sobre {
	return mensajeria.Sobre{Tipo: "operaciones.solicitud.aceptada", Datos: json.RawMessage(datos)}
}

func TestManejadorClima(t *testing.T) {
	ctx := context.Background()
	valido := `{"organizacion_id":"org-1","origen":"externo","solicitud":{"id":"sol-1"}}`

	f := &casosFalsos{}
	if err := NuevoManejadorClima(f)(ctx, sobre(valido)); err != nil || f.id != "sol-1" {
		t.Fatalf("err = %v, id = %q", err, f.id)
	}

	casos := []struct {
		nombre     string
		datos      string
		err        error
		permanente bool
	}{
		{"datos ilegibles → DLQ", `{"solicitud":`, nil, true},
		{"sin id → DLQ", `{"solicitud":{}}`, nil, true},
		{"solicitud inexistente → DLQ", valido, puertos.ErrNoEncontrada, true},
		{"MySQL caído → reintento", valido, errors.New("conexión rechazada"), false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			err := NuevoManejadorClima(&casosFalsos{err: c.err})(ctx, sobre(c.datos))
			if err == nil || mensajeria.EsPermanente(err) != c.permanente {
				t.Fatalf("err = %v, permanente = %v", err, mensajeria.EsPermanente(err))
			}
		})
	}
}
