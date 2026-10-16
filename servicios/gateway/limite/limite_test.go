package limite

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func TestPermiteLaRafagaYRechazaLaSiguiente(t *testing.T) {
	l := Nuevo()
	for i := 1; i <= rafaga; i++ {
		if ok, _ := l.Permitir("org-1"); !ok {
			t.Fatalf("el pedido %d de la ráfaga tendría que pasar", i)
		}
	}
	ok, espera := l.Permitir("org-1")
	if ok || espera <= 0 {
		t.Fatalf("el pedido %d tendría que rechazarse con una espera, ok=%v espera=%v", rafaga+1, ok, espera)
	}
	if ok, _ := l.Permitir("org-2"); !ok {
		t.Fatal("otra organización no tiene que verse afectada")
	}
}

func TestMiddlewareResponde429ConRetryAfter(t *testing.T) {
	llegaron := 0
	h := Middleware(Nuevo())(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		llegaron++
		w.WriteHeader(http.StatusNoContent)
	}))
	pedir := func(org string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/traslados", nil)
		req.Header.Set("X-Rumbo-Organizacion", org)
		h.ServeHTTP(w, req)
		return w
	}

	for i := 1; i <= rafaga; i++ {
		if w := pedir("org-1"); w.Code != http.StatusNoContent {
			t.Fatalf("pedido %d: estado = %d", i, w.Code)
		}
	}
	w := pedir("org-1")
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("el pedido %d: estado = %d, esperaba 429", rafaga+1, w.Code)
	}
	segundos, err := strconv.Atoi(w.Header().Get("Retry-After"))
	if err != nil || segundos < 1 {
		t.Errorf("Retry-After = %q, esperaba un entero >= 1", w.Header().Get("Retry-After"))
	}
	if w.Header().Get("Content-Type") != "application/problem+json" {
		t.Errorf("Content-Type = %q", w.Header().Get("Content-Type"))
	}
	if llegaron != rafaga {
		t.Errorf("llegaron %d pedidos al servicio, esperaba %d", llegaron, rafaga)
	}
	if w := pedir("org-2"); w.Code != http.StatusNoContent {
		t.Errorf("otra organización: estado = %d, esperaba que pase", w.Code)
	}
}

func TestLaClaveSaleDeLaOrganizacionDelUsuarioOLaIP(t *testing.T) {
	casos := []struct {
		nombre   string
		headers  map[string]string
		remota   string
		esperada string
	}{
		{"organización", map[string]string{"X-Rumbo-Organizacion": "org-1", "X-Rumbo-Usuario": "usr-1"}, "10.0.0.1:5000", "org-1"},
		{"usuario sin organización", map[string]string{"X-Rumbo-Usuario": "usr-1"}, "10.0.0.1:5000", "usr-1"},
		{"ruta pública", nil, "10.0.0.1:5000", "ip:10.0.0.1"},
	}
	for _, caso := range casos {
		req := httptest.NewRequest(http.MethodPost, "/login", nil)
		req.RemoteAddr = caso.remota
		for k, v := range caso.headers {
			req.Header.Set(k, v)
		}
		if obtenida := clave(req); obtenida != caso.esperada {
			t.Errorf("%s: clave = %q, esperaba %q", caso.nombre, obtenida, caso.esperada)
		}
	}
}

func TestDescartaLasClavesSinUso(t *testing.T) {
	l := Nuevo()
	l.Permitir("vieja")
	l.Permitir("reciente")
	l.cubos["vieja"].ultimoUso = time.Now().Add(-inactividadMaxima - time.Minute)

	l.descartarInactivas(time.Now())

	if _, hay := l.cubos["vieja"]; hay {
		t.Error("la clave sin uso hace más de 10 min tendría que descartarse")
	}
	if _, hay := l.cubos["reciente"]; !hay {
		t.Error("la clave reciente no tendría que descartarse")
	}
}
