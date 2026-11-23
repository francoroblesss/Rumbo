//go:build e2e

package e2e

import (
	"net/http"
	"testing"
)

// TestClimaDelAeropuerto: la web ve las condiciones actuales del aeropuerto (§13.5, ADR-034). En
// el CI responde el mock de Prism con el ejemplo "operable" del contrato de clima.
func TestClimaDelAeropuerto(t *testing.T) {
	externo, operador := pedirToken(t), login(t, "OPERADOR")

	var clima struct {
		Aeropuerto struct {
			ICAO string `json:"icao"`
		} `json:"aeropuerto"`
		Operable    bool `json:"operable"`
		Condiciones struct {
			TemperaturaC float64 `json:"temperatura_c"`
		} `json:"condiciones"`
	}
	esperar(t, hacer(t, http.MethodGet, "/api/operaciones/clima", operador, nil, nil), http.StatusOK, "clima del operador").leer(t, &clima)
	if clima.Aeropuerto.ICAO != "SACO" || !clima.Operable || clima.Condiciones.TemperaturaC != 22.4 {
		t.Fatalf("clima inesperado: %+v", clima)
	}

	// El gateway no filtra roles: lo responde identidad.RequiereRol en Operaciones.
	r := esperar(t, hacer(t, http.MethodGet, "/api/operaciones/clima", externo, nil, nil), http.StatusForbidden, "clima del sistema externo")
	if c := r.codigo(); c != "SIN_PERMISO" {
		t.Fatalf("codigo %q, se esperaba SIN_PERMISO", c)
	}
}
