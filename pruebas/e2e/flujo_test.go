//go:build e2e

package e2e

import (
	"fmt"
	"net/http"
	"slices"
	"testing"
)

// TestFlujoPrincipal recorre el flujo de la Presentación (ADR-024) de punta a punta: el sistema
// externo reserva por el contrato público, el operador lo ve y lo avanza, el sistema externo ve el
// resultado y el operador y el cliente tienen sus avisos. Cruza gateway, Identidad, Integración,
// Operaciones (outbox), RabbitMQ y Notificaciones.
func TestFlujoPrincipal(t *testing.T) {
	externo, operador, cliente := pedirToken(t), login(t, "OPERADOR"), login(t, "CLIENTE")
	f := franjaLibre(t, externo, operador, 1)

	// 1. Reserva por el contrato público: 201 PROGRAMADO.
	var reserva traslado
	esperar(t, reservar(t, externo, f, "Flujo principal"), http.StatusCreated, "POST /traslados").leer(t, &reserva)
	if reserva.ID == "" || reserva.Estado != "PROGRAMADO" {
		t.Fatalf("la reserva nace PROGRAMADO con id: %+v", reserva)
	}
	t.Logf("reservado %s en la franja %s", reserva.ID, f.inicio)

	// 2. El operador ve la franja con 1 reservado y el traslado en el listado del día.
	if n := reservadosEn(t, operador, f); n != 1 {
		t.Fatalf("la franja tiene %d reservados, se esperaba 1", n)
	}
	var listado struct {
		Resultados []traslado `json:"resultados"` // vistas de solicitud: solo se mira el id
	}
	esperar(t, hacer(t, http.MethodGet, "/api/operaciones/solicitudes?tamanio=100&fecha="+f.fecha, operador, nil, nil),
		http.StatusOK, "listado del operador").leer(t, &listado)
	if !slices.ContainsFunc(listado.Resultados, func(s traslado) bool { return s.ID == reserva.ID }) {
		t.Fatalf("el traslado %s no está en el listado del %s", reserva.ID, f.fecha)
	}

	// 3. El operador lo avanza: EN_CURSO → COMPLETADO.
	for _, estado := range []string{"EN_CURSO", "COMPLETADO"} {
		var vista struct {
			Traslado traslado `json:"traslado"`
		}
		esperar(t, hacer(t, http.MethodPost, "/api/operaciones/solicitudes/"+reserva.ID+"/estado", operador,
			map[string]string{"estado": estado}, nil), http.StatusOK, "avanzar a "+estado).leer(t, &vista)
		if vista.Traslado.Estado != estado {
			t.Fatalf("después de pedir %s el traslado quedó %s", estado, vista.Traslado.Estado)
		}
	}

	// 4. El sistema externo ve COMPLETADO (con finalizado_en) por la API pública: llega por el
	// evento de Operaciones a la copia de Integración, así que se espera con sondeo (CA-37).
	esperarHasta(t, plazoAsincrono, func() (bool, string) {
		tr := estadoPublico(t, externo, reserva.ID)
		return tr.Estado == "COMPLETADO" && tr.FinalizadoEn != nil, fmt.Sprintf("%+v", tr)
	})

	// 5. Avisos (ADR-027): el cliente, de su organización; el operador, la reserva nueva.
	esperarHasta(t, plazoAsincrono, func() (bool, string) {
		return tieneAvisos(t, cliente, reserva.ID, "Traslado en curso", "Traslado completado")
	})
	esperarHasta(t, plazoAsincrono, func() (bool, string) {
		return tieneAvisos(t, operador, reserva.ID, "Reserva nueva")
	})
}
