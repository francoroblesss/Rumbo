package despertar

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestReintentaHastaQueElServicioDespierta(t *testing.T) {
	var llamadas atomic.Int32
	servicio := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/salud" {
			t.Errorf("ruta %q, se esperaba /salud", r.URL.Path)
		}
		if llamadas.Add(1) < 3 { // "dormido" las dos primeras veces
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer servicio.Close()

	Cadena(context.Background(), servicio.Client(), []string{servicio.URL}, 10*time.Millisecond, 2*time.Second)

	if got := llamadas.Load(); got != 3 {
		t.Fatalf("se esperaban 3 llamadas (dos fallidas y la que despierta), hubo %d", got)
	}
}

func TestSeRindeAlVencerElPlazo(t *testing.T) {
	servicio := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer servicio.Close()

	inicio := time.Now()
	Cadena(context.Background(), servicio.Client(), []string{servicio.URL}, 10*time.Millisecond, 100*time.Millisecond)
	if transcurrido := time.Since(inicio); transcurrido > time.Second {
		t.Fatalf("tardó %s: tenía que rendirse a los 100 ms", transcurrido)
	}
}
