package services

import (
	"encoding/json"
	"testing"

	"github.com/francoroblesss/Rumbo/servicios/comun/mensajeria"
	"github.com/francoroblesss/Rumbo/servicios/notificaciones/models"
)

// Tests de la tabla de reglas de docs/interfaces-internas.md §14: un caso por fila, con
// destinatario, título, mensaje e id exactos.

const idMensaje = "6f1c2a8e-0000-4000-8000-000000000001"

var (
	alOperador = models.Destinatario{Tipo: "ROL", Valor: "operador"}
	aLaOrg     = models.Destinatario{Tipo: "ORGANIZACION", Valor: "org-almacenamiento"}
)

// vista arma la vista de §6 tal como la publica Operaciones. La franja va en UTC (13:00Z =
// 10:00 de Argentina) para verificar que el mensaje la pasa a hora local.
func vista(estado string, extra map[string]any) map[string]any {
	traslado := map[string]any{
		"id": "tra-1", "estado": estado, "hora_llegada_comprometida": "2026-11-20T15:00:00Z",
		"franja": map[string]any{"inicio": "2026-11-20T13:00:00Z", "fin": "2026-11-20T15:00:00Z"},
	}
	v := map[string]any{
		"id": "sol-1", "estado": "ACEPTADA",
		"cosa":     map[string]any{"descripcion": "Cajas de repuestos", "cantidad": 20, "peso_kg": 350},
		"origen":   map[string]any{"nombre": "Depósito central", "direccion": "Ruta 9 km 5"},
		"destino":  map[string]any{"nombre": "Aeropuerto de Córdoba", "direccion": "Av. La Voz del Interior 8500"},
		"traslado": traslado, "version": 2,
	}
	for k, valor := range extra {
		if k == "alerta_climatica" {
			traslado[k] = valor
			continue
		}
		v[k] = valor
	}
	return v
}

func sobre(t *testing.T, tipo string, solicitud map[string]any) mensajeria.Sobre {
	t.Helper()
	datos, err := json.Marshal(map[string]any{"organizacion_id": "org-almacenamiento", "origen": "externo", "solicitud": solicitud})
	if err != nil {
		t.Fatal(err)
	}
	return mensajeria.Sobre{ID: idMensaje, Tipo: tipo, Version: 1, AgregadoID: "sol-1", AgregadoVersion: 2, Datos: datos}
}

type esperado struct {
	id           string
	destinatario models.Destinatario
	titulo       string
	mensaje      string
}

func verificar(t *testing.T, avisos []models.Aviso, esperados ...esperado) {
	t.Helper()
	if len(avisos) != len(esperados) {
		t.Fatalf("se esperaban %d avisos, salieron %d: %+v", len(esperados), len(avisos), avisos)
	}
	for i, e := range esperados {
		a := avisos[i]
		if a.ID != e.id || a.Destinatario != e.destinatario || a.Titulo != e.titulo || a.Mensaje != e.mensaje {
			t.Errorf("aviso %d:\n obtenido %+v\n esperado %+v", i, a, e)
		}
		if a.Entidad != (models.Entidad{Tipo: "solicitud", ID: "sol-1"}) || a.Leido {
			t.Errorf("aviso %d: entidad o leído incorrectos: %+v", i, a)
		}
	}
}

func TestAvisosParaReservaNueva(t *testing.T) {
	avisos, err := avisosPara(sobre(t, "operaciones.solicitud.aceptada", vista("PROGRAMADO", nil)))
	if err != nil {
		t.Fatal(err)
	}
	verificar(t, avisos, esperado{idMensaje, alOperador, "Reserva nueva",
		"Se reservó «Cajas de repuestos» para la franja del 20/11 de 10:00 a 12:00."})
}

func TestAvisosParaCancelacionTardia(t *testing.T) {
	v := vista("CANCELADO", map[string]any{"cancelacion": map[string]any{"estado": "APROBADA", "tardia": true}})
	avisos, err := avisosPara(sobre(t, "operaciones.cancelacion.registrada", v))
	if err != nil {
		t.Fatal(err)
	}
	verificar(t, avisos,
		esperado{idMensaje + "-1", alOperador, "Cancelación tardía",
			"Se canceló «Cajas de repuestos» con menos de 24 h: era para la franja del 20/11 de 10:00 a 12:00."},
		esperado{idMensaje + "-2", aLaOrg, "Traslado cancelado", "Se canceló el traslado de «Cajas de repuestos» del 20/11."})
}

func TestAvisosParaCancelacionATiempo(t *testing.T) {
	v := vista("CANCELADO", map[string]any{"cancelacion": map[string]any{"estado": "APROBADA", "tardia": false}})
	avisos, err := avisosPara(sobre(t, "operaciones.cancelacion.registrada", v))
	if err != nil {
		t.Fatal(err)
	}
	verificar(t, avisos, esperado{idMensaje, aLaOrg, "Traslado cancelado", "Se canceló el traslado de «Cajas de repuestos» del 20/11."})
}

func TestAvisosParaAlertaNoOperable(t *testing.T) {
	v := vista("PROGRAMADO", map[string]any{"alerta_climatica": map[string]any{"estado": "NO_OPERABLE", "motivos": []string{"HIGH_WIND", "LOW_VISIBILITY"}}})
	avisos, err := avisosPara(sobre(t, "operaciones.traslado.alerta_climatica", v))
	if err != nil {
		t.Fatal(err)
	}
	mensaje := "El aeropuerto no está operable para «Cajas de repuestos» (HIGH_WIND, LOW_VISIBILITY)."
	verificar(t, avisos,
		esperado{idMensaje + "-1", alOperador, "Alerta climática", mensaje},
		esperado{idMensaje + "-2", aLaOrg, "Alerta climática", mensaje})
}

func TestAvisosParaAlertaNoVerificada(t *testing.T) {
	v := vista("PROGRAMADO", map[string]any{"alerta_climatica": map[string]any{"estado": "NO_VERIFICADO"}})
	avisos, err := avisosPara(sobre(t, "operaciones.traslado.alerta_climatica", v))
	if err != nil {
		t.Fatal(err)
	}
	mensaje := "No se pudo verificar el clima del aeropuerto para «Cajas de repuestos»."
	verificar(t, avisos,
		esperado{idMensaje + "-1", alOperador, "Alerta climática", mensaje},
		esperado{idMensaje + "-2", aLaOrg, "Alerta climática", mensaje})
}

func TestAvisosParaTrasladoEnCurso(t *testing.T) {
	avisos, err := avisosPara(sobre(t, "operaciones.traslado.estado_cambiado", vista("EN_CURSO", nil)))
	if err != nil {
		t.Fatal(err)
	}
	verificar(t, avisos, esperado{idMensaje, aLaOrg, "Traslado en curso", "Salió el traslado de «Cajas de repuestos»."})
}

func TestAvisosParaTrasladoCompletado(t *testing.T) {
	avisos, err := avisosPara(sobre(t, "operaciones.traslado.estado_cambiado", vista("COMPLETADO", nil)))
	if err != nil {
		t.Fatal(err)
	}
	verificar(t, avisos, esperado{idMensaje, aLaOrg, "Traslado completado", "Llegó «Cajas de repuestos» a Aeropuerto de Córdoba."})
}

func TestAvisosParaEstadoSinAviso(t *testing.T) {
	for _, estado := range []string{"CANCELADO", "PROGRAMADO"} {
		avisos, err := avisosPara(sobre(t, "operaciones.traslado.estado_cambiado", vista(estado, nil)))
		if err != nil || len(avisos) != 0 {
			t.Errorf("%s: se esperaba ningún aviso y sin error: %+v %v", estado, avisos, err)
		}
	}
}

func TestAvisosParaEventoSinRegla(t *testing.T) {
	avisos, err := avisosPara(sobre(t, "operaciones.solicitud.recibida", vista("PROGRAMADO", nil)))
	if err != nil || len(avisos) != 0 {
		t.Fatalf("un evento sin regla no genera avisos ni error: %+v %v", avisos, err)
	}
}

func TestAvisosParaDatosIlegibles(t *testing.T) {
	casos := map[string]mensajeria.Sobre{
		"no es JSON":   {ID: idMensaje, Tipo: "operaciones.solicitud.aceptada", Datos: json.RawMessage(`"texto"`)},
		"sin traslado": sobre(t, "operaciones.solicitud.aceptada", map[string]any{"id": "sol-1", "cosa": map[string]any{"descripcion": "Cajas"}}),
		"sin id":       {Tipo: "operaciones.solicitud.aceptada", Datos: sobre(t, "operaciones.solicitud.aceptada", vista("PROGRAMADO", nil)).Datos},
		"alerta desconocida": sobre(t, "operaciones.traslado.alerta_climatica",
			vista("PROGRAMADO", map[string]any{"alerta_climatica": map[string]any{"estado": "OTRO"}})),
	}
	for nombre, s := range casos {
		if _, err := avisosPara(s); !mensajeria.EsPermanente(err) {
			t.Errorf("%s: se esperaba un error permanente (DLQ), salió %v", nombre, err)
		}
	}
}
