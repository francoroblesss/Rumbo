package entrada

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBloquearInternasResponde404(t *testing.T) {
	llegaron := 0
	h := BloquearInternas(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { llegaron++ }))

	for _, ruta := range []string{
		"/api/v1/operaciones/internas/v1/solicitudes/sol-1/cancelacion",
		"/api/v1/recursos/internas/x",
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, ruta, nil))
		if w.Code != http.StatusNotFound || w.Header().Get("Content-Type") != "application/problem+json" {
			t.Errorf("%s: estado = %d, content-type = %q", ruta, w.Code, w.Header().Get("Content-Type"))
		}
	}

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/operaciones/solicitudes", nil))
	if w.Code != http.StatusOK || llegaron != 1 {
		t.Errorf("una ruta pública tiene que pasar: estado = %d, llegaron = %d", w.Code, llegaron)
	}
}

func TestLimpiarIdentidadBorraLosHeadersDelCliente(t *testing.T) {
	var llegada http.Header
	h := LimpiarIdentidad(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { llegada = r.Header.Clone() }))

	req := httptest.NewRequest(http.MethodGet, "/v2/traslados", nil)
	req.Header.Set("X-Rumbo-Usuario", "usr-falso")
	req.Header.Set("X-Rumbo-Organizacion", "org-falsa")
	req.Header.Set("x-rumbo-roles", "administrador")
	req.Header.Set("X-Correlation-ID", "corr-1")
	h.ServeHTTP(httptest.NewRecorder(), req)

	for _, nombre := range []string{"X-Rumbo-Usuario", "X-Rumbo-Organizacion", "X-Rumbo-Roles"} {
		if _, hay := llegada[nombre]; hay {
			t.Errorf("%s no tendría que llegar", nombre)
		}
	}
	if llegada.Get("X-Correlation-ID") != "corr-1" {
		t.Error("el correlation id tiene que conservarse")
	}
}
