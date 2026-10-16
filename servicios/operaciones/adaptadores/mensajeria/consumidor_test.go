package mensajeria

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/francoroblesss/Rumbo/servicios/comun/mensajeria"
	"github.com/francoroblesss/Rumbo/servicios/operaciones/puertos"
)

// casosFalsos guarda el comando recibido; el resto de los casos de uso no se usa acá.
type casosFalsos struct {
	puertos.Solicitudes
	cmd puertos.ComandoSolicitudExterna
}

func (c *casosFalsos) RegistrarExterna(_ context.Context, cmd puertos.ComandoSolicitudExterna) error {
	c.cmd = cmd
	return nil
}

// El ejemplo de docs/interfaces-internas.md §5.3.
const datosEjemplo = `{
  "solicitud_id": "sol-1", "organizacion_id": "org-1", "referencia_externa": "TRF-2026-00123",
  "cosa": { "descripcion": "Cajas", "cantidad": 20, "peso_kg": 350, "volumen_m3": null },
  "origen":  { "nombre": "Sucursal Centro", "direccion": "Av. Colón 1234", "contacto": null },
  "destino": { "nombre": "Sucursal Cerro",  "direccion": "Av. R. Núñez 4500", "contacto": null },
  "hora_llegada_solicitada": "2026-10-16T14:00:00-03:00", "tolerancia_minutos": 120,
  "observaciones": null, "recibida_en": "2026-10-15T08:30:00-03:00" }`

func TestManejadorTraduceElComando(t *testing.T) {
	casos := &casosFalsos{}
	sobre := mensajeria.Sobre{ID: "msg-1", Tipo: TipoSolicitudExterna, Version: 1, AgregadoID: "sol-1",
		AgregadoVersion: 1, Datos: json.RawMessage(datosEjemplo)}
	if err := ManejadorSolicitudesExternas(casos)(context.Background(), sobre); err != nil {
		t.Fatal(err)
	}
	c := casos.cmd
	if c.MensajeID != "msg-1" || c.SolicitudID != "sol-1" || c.OrganizacionID != "org-1" ||
		c.Datos.Cosa.Cantidad != 20 || *c.Datos.Cosa.PesoKg != 350 || c.Datos.Cosa.VolumenM3 != nil ||
		c.Datos.Hasta.Nombre != "Sucursal Cerro" || c.Datos.ToleranciaMinutos != 120 || *c.Datos.ReferenciaExterna != "TRF-2026-00123" {
		t.Fatalf("comando inesperado: %+v", c)
	}
	if !c.Datos.HoraLlegadaSolicitada.Equal(time.Date(2026, 10, 16, 17, 0, 0, 0, time.UTC)) {
		t.Fatalf("hora = %v", c.Datos.HoraLlegadaSolicitada)
	}
}

func TestManejadorRechazaLoDesconocido(t *testing.T) {
	manejar := ManejadorSolicitudesExternas(&casosFalsos{})
	for _, s := range []mensajeria.Sobre{
		{ID: "1", Tipo: "otro.tipo", Version: 1, Datos: json.RawMessage(datosEjemplo)},
		{ID: "2", Tipo: TipoSolicitudExterna, Version: 2, Datos: json.RawMessage(datosEjemplo)},
		{ID: "3", Tipo: TipoSolicitudExterna, Version: 1, Datos: json.RawMessage(`{"cosa": "no es un objeto"}`)},
	} {
		if err := manejar(context.Background(), s); !mensajeria.EsPermanente(err) {
			t.Fatalf("mensaje %s: error = %v, se esperaba permanente (va a la DLQ)", s.ID, err)
		}
	}
}
