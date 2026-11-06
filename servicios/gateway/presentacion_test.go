package main

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/francoroblesss/Rumbo/servicios/comun/identidad"
	"github.com/francoroblesss/Rumbo/servicios/gateway/config"
	"github.com/francoroblesss/Rumbo/servicios/gateway/limite"
)

// Tests de lo que agrega la base de la Presentación (docs/interfaces-internas.md §11).

func TestElServicioRecibeElSecretoDelGatewayYNoElDelCliente(t *testing.T) {
	e := nuevoEntorno(t)
	w := e.pedir(http.MethodGet, "/traslados/x", token(t, "org-1", identidad.RolSistemaExterno),
		map[string]string{identidad.HeaderInterno: "falsificado"})
	if w.Code != http.StatusNoContent {
		t.Fatalf("estado %d", w.Code)
	}
	if got := e.integra.recibida.Header.Get(identidad.HeaderInterno); got != secretoInternoPrueba {
		t.Fatalf("el servicio recibió %q como secreto interno, se esperaba el del gateway", got)
	}
}

func TestSaludDelGatewaySinToken(t *testing.T) {
	e := nuevoEntorno(t)
	w := e.pedir(http.MethodGet, "/salud", "", nil)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"estado":"ok"`) {
		t.Fatalf("/salud: %d %s", w.Code, w.Body.String())
	}
}

func TestPreflightDelOrigenDeLaWeb(t *testing.T) {
	e := nuevoEntorno(t)
	preflight := func(origen string) *httptest.ResponseRecorder {
		return e.pedir(http.MethodOptions, "/api/operaciones/franjas", "", map[string]string{
			"Origin":                         origen,
			"Access-Control-Request-Method":  http.MethodPost,
			"Access-Control-Request-Headers": "authorization,idempotency-key",
		})
	}

	permitido := preflight(origenWebPrueba)
	if got := permitido.Header().Get("Access-Control-Allow-Origin"); got != origenWebPrueba {
		t.Fatalf("origen permitido: Access-Control-Allow-Origin = %q", got)
	}
	if permitido.Code == http.StatusUnauthorized {
		t.Fatal("el preflight no lleva token: no tiene que pasar por la autenticación")
	}
	if e.operaciones.llamadas != 0 {
		t.Fatal("el preflight lo responde el gateway, no llega al servicio")
	}

	if got := preflight("https://otro.example").Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("origen no permitido: Access-Control-Allow-Origin = %q", got)
	}
}

func TestLaRespuestaExponeElCorrelationID(t *testing.T) {
	e := nuevoEntorno(t)
	w := e.pedir(http.MethodGet, "/traslados/x", token(t, "org-1", identidad.RolSistemaExterno),
		map[string]string{"Origin": origenWebPrueba})
	if got := w.Header().Get("Access-Control-Expose-Headers"); !strings.Contains(strings.ToLower(got), "x-correlation-id") {
		t.Fatalf("Access-Control-Expose-Headers = %q", got)
	}
}

// El stream de avisos dura más que el deadline de 5 s y cada evento llega apenas se escribe.
func TestElStreamNoSeCortaALosCincoSegundos(t *testing.T) {
	if testing.Short() {
		t.Skip("espera más que el deadline de 5 s")
	}
	notificaciones := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/avisos/stream" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		time.Sleep(6 * time.Second) // más que proxy.TiempoMaximo
		_, _ = w.Write([]byte("event: aviso\nid: 1\ndata: {}\n\n"))
		w.(http.Flusher).Flush()
	}))
	defer notificaciones.Close()

	gateway := httptest.NewServer(armar(config.Config{
		TokenSecreto: secretoPrueba, NotificacionesURL: notificaciones.URL, IdentidadURL: notificaciones.URL,
		OperacionesURL: notificaciones.URL, RecursosURL: notificaciones.URL, IntegracionURL: notificaciones.URL,
		WebOrigen: origenWebPrueba,
	}, limite.Nuevo()))
	defer gateway.Close()

	req, _ := http.NewRequest(http.MethodGet, gateway.URL+"/api/notificaciones/avisos/stream", nil)
	req.Header.Set("Authorization", "Bearer "+token(t, "", identidad.RolOperador))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("estado %d", resp.StatusCode)
	}
	linea, err := bufio.NewReader(resp.Body).ReadString('\n')
	if err != nil || linea != "event: aviso\n" {
		t.Fatalf("se esperaba el evento después de 6 s, se leyó %q (%v)", linea, err)
	}
}
