package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/francoroblesss/Rumbo/servicios/comun/observabilidad"
	"github.com/francoroblesss/Rumbo/servicios/comun/problema"
)

func leerProblema(t *testing.T, rec *httptest.ResponseRecorder) problema.Problema {
	t.Helper()
	if ct := rec.Header().Get("Content-Type"); ct != problema.TipoContenido {
		t.Fatalf("Content-Type = %q, se esperaba %q", ct, problema.TipoContenido)
	}
	var p problema.Problema
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("el cuerpo no es Problem Details: %v", err)
	}
	return p
}

func TestQuitaElPrefijoAntesDeReenviar(t *testing.T) {
	var rutaRecibida string
	servicio := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rutaRecibida = r.URL.Path
	}))
	defer servicio.Close()

	rec := httptest.NewRecorder()
	Nuevo(servicio.URL, "/api/v1/recursos").ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/recursos/choferes", nil))

	if rutaRecibida != "/choferes" {
		t.Fatalf("el servicio recibió %q, se esperaba /choferes", rutaRecibida)
	}
}

func TestElCorrelationIDNoSeDuplicaEnLaRespuesta(t *testing.T) {
	servicio := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// El servicio devuelve el mismo id que recibió, como hace observabilidad.Instrumentar.
		w.Header().Set(observabilidad.HeaderCorrelacion, r.Header.Get(observabilidad.HeaderCorrelacion))
		w.WriteHeader(http.StatusAccepted)
	}))
	defer servicio.Close()
	proxy := Nuevo(servicio.URL, "")
	// El middleware del gateway ya puso el id en la respuesta antes de reenviar.
	conMiddleware := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(observabilidad.HeaderCorrelacion, "id-1")
		r.Header.Set(observabilidad.HeaderCorrelacion, "id-1")
		proxy.ServeHTTP(w, r)
	})

	rec := httptest.NewRecorder()
	conMiddleware.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/traslados", nil))

	if valores := rec.Header().Values(observabilidad.HeaderCorrelacion); len(valores) != 1 || valores[0] != "id-1" {
		t.Fatalf("X-Correlation-ID = %v, se esperaba un solo valor id-1", valores)
	}
	if rec.Code != http.StatusAccepted {
		t.Fatalf("estado %d, se esperaba 202", rec.Code)
	}
}

func TestServicioCaidoResponde502ConProblemDetails(t *testing.T) {
	servicio := httptest.NewServer(http.NotFoundHandler())
	destino := servicio.URL
	servicio.Close() // nadie escucha en esa dirección

	rec := httptest.NewRecorder()
	Nuevo(destino, "").ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/traslados/x", nil))

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("estado %d, se esperaba 502", rec.Code)
	}
	if p := leerProblema(t, rec); p.Codigo != "SERVICIO_NO_DISPONIBLE" {
		t.Fatalf("codigo %q, se esperaba SERVICIO_NO_DISPONIBLE", p.Codigo)
	}
}

func TestServicioLentoResponde504ConProblemDetails(t *testing.T) {
	if testing.Short() {
		t.Skip("espera el deadline completo de 5 s")
	}
	liberar := make(chan struct{})
	servicio := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-liberar:
		case <-time.After(TiempoMaximo + 2*time.Second):
		}
	}))
	defer servicio.Close()
	defer close(liberar)

	rec := httptest.NewRecorder()
	Nuevo(servicio.URL, "").ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/traslados/x", nil))

	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("estado %d, se esperaba 504", rec.Code)
	}
	if p := leerProblema(t, rec); p.Codigo != "TIEMPO_AGOTADO" {
		t.Fatalf("codigo %q, se esperaba TIEMPO_AGOTADO", p.Codigo)
	}
}
