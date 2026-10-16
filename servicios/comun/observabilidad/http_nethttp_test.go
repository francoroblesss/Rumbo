package observabilidad

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestInstrumentarHTTPPropagaCorrelacion(t *testing.T) {
	if _, err := Iniciar(context.Background(), "prueba"); err != nil {
		t.Fatal(err)
	}
	var enContexto, enHeader string
	h := InstrumentarHTTP(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		enContexto = Correlacion(r.Context())
		enHeader = r.Header.Get(HeaderCorrelacion)
		w.WriteHeader(http.StatusAccepted)
	}), "prueba")

	conHeader := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/traslados/sol-1", nil)
	req.Header.Set(HeaderCorrelacion, "corr-1")
	h.ServeHTTP(conHeader, req)
	if enContexto != "corr-1" || enHeader != "corr-1" || conHeader.Header().Get(HeaderCorrelacion) != "corr-1" {
		t.Fatalf("correlación no propagada: contexto=%q request=%q respuesta=%q",
			enContexto, enHeader, conHeader.Header().Get(HeaderCorrelacion))
	}
	if conHeader.Code != http.StatusAccepted {
		t.Fatalf("estado alterado: %d", conHeader.Code)
	}

	sinHeader := httptest.NewRecorder()
	h.ServeHTTP(sinHeader, httptest.NewRequest(http.MethodGet, "/traslados", nil))
	generado := sinHeader.Header().Get(HeaderCorrelacion)
	if generado == "" {
		t.Fatal("sin header de entrada se tiene que generar un correlation id")
	}
	if enContexto != generado || enHeader != generado {
		t.Fatalf("el id generado no llegó al handler: contexto=%q request=%q respuesta=%q", enContexto, enHeader, generado)
	}
}

func TestRutaAgrupada(t *testing.T) {
	casos := map[string]string{
		"/traslados":                      "/traslados",
		"/traslados/sol-7f3a":             "/traslados",
		"/traslados/sol-7f3a/cancelacion": "/traslados",
		"/auth/token":                     "/auth",
		"/api/operaciones/solicitudes":    "/api/operaciones",
		"/api/operaciones/solicitudes/42": "/api/operaciones",
		"/api/identidad/login":            "/api/identidad",
		"/api":                            "/api",
		"/":                               "/",
	}
	for path, esperada := range casos {
		if obtenida := rutaAgrupada(path); obtenida != esperada {
			t.Errorf("rutaAgrupada(%q) = %q, esperaba %q", path, obtenida, esperada)
		}
	}
}
