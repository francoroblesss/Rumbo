package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/francoroblesss/Rumbo/servicios/gateway/config"
	"github.com/francoroblesss/Rumbo/servicios/gateway/limite"
)

const secretoPrueba = "secreto-de-prueba-bastante-largo"

// servicioFalso es un servicio detrás del gateway: guarda la última request que recibió.
type servicioFalso struct {
	*httptest.Server
	recibida *http.Request
	llamadas int
}

func nuevoServicioFalso(t *testing.T) *servicioFalso {
	t.Helper()
	s := &servicioFalso{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.recibida = r
		s.llamadas++
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(s.Close)
	return s
}

type entorno struct {
	gateway                         http.Handler
	identidad, operaciones, integra *servicioFalso
}

func nuevoEntorno(t *testing.T) *entorno {
	t.Helper()
	e := &entorno{
		identidad:   nuevoServicioFalso(t),
		operaciones: nuevoServicioFalso(t),
		integra:     nuevoServicioFalso(t),
	}
	otro := nuevoServicioFalso(t)
	e.gateway = armar(config.Config{
		Puerto:            "0",
		TokenSecreto:      secretoPrueba,
		IdentidadURL:      e.identidad.URL,
		OperacionesURL:    e.operaciones.URL,
		RecursosURL:       otro.URL,
		NotificacionesURL: otro.URL,
		IntegracionURL:    e.integra.URL,
	}, limite.Nuevo())
	return e
}

func token(t *testing.T, org string, roles ...string) string {
	t.Helper()
	ahora := time.Now()
	firmado, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"iss": "rumbo-identidad", "aud": []string{"rumbo"}, "sub": "usr-7", "org": org, "roles": roles,
		"iat": ahora.Unix(), "exp": ahora.Add(time.Hour).Unix(),
	}).SignedString([]byte(secretoPrueba))
	if err != nil {
		t.Fatal(err)
	}
	return firmado
}

func (e *entorno) pedir(metodo, ruta, bearer string, headers map[string]string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(metodo, ruta, strings.NewReader(""))
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	e.gateway.ServeHTTP(w, req)
	return w
}

func TestSinTokenNoLlegaAlServicio(t *testing.T) {
	e := nuevoEntorno(t)
	w := e.pedir(http.MethodGet, "/v2/traslados", "", nil)
	if w.Code != http.StatusUnauthorized || e.integra.llamadas != 0 {
		t.Fatalf("estado = %d, llamadas = %d", w.Code, e.integra.llamadas)
	}
	if w.Header().Get("X-Correlation-ID") == "" {
		t.Error("hasta un rechazo tiene que llevar correlation id")
	}
}

func TestTokenValidoLlegaConLaIdentidadDelToken(t *testing.T) {
	e := nuevoEntorno(t)
	w := e.pedir(http.MethodGet, "/v2/traslados/sol-1", token(t, "org-1", "sistema_externo"), nil)
	if w.Code != http.StatusNoContent || e.integra.llamadas != 1 {
		t.Fatalf("estado = %d, llamadas = %d", w.Code, e.integra.llamadas)
	}
	recibida := e.integra.recibida
	if recibida.Header.Get("X-Rumbo-Usuario") != "usr-7" ||
		recibida.Header.Get("X-Rumbo-Organizacion") != "org-1" ||
		recibida.Header.Get("X-Rumbo-Roles") != "sistema_externo" {
		t.Errorf("identidad recibida por el servicio: %v", recibida.Header)
	}
	if recibida.URL.Path != "/v2/traslados/sol-1" {
		t.Errorf("path reenviado = %q", recibida.URL.Path)
	}
	if recibida.Header.Get("X-Correlation-ID") == "" || recibida.Header.Get("X-Correlation-ID") != w.Header().Get("X-Correlation-ID") {
		t.Errorf("correlation id: servicio=%q respuesta=%q",
			recibida.Header.Get("X-Correlation-ID"), w.Header().Get("X-Correlation-ID"))
	}
}

func TestLaIdentidadDelClienteNoLlegaAlServicio(t *testing.T) {
	e := nuevoEntorno(t)
	falsos := map[string]string{
		"X-Rumbo-Usuario": "usr-falso", "X-Rumbo-Organizacion": "org-falsa", "X-Rumbo-Roles": "administrador",
	}

	// Con token: llegan los valores del token, no los que mandó el cliente.
	e.pedir(http.MethodGet, "/api/v1/operaciones/solicitudes", token(t, "org-1", "cliente"), falsos)
	recibida := e.operaciones.recibida
	if recibida.Header.Get("X-Rumbo-Roles") != "cliente" || recibida.Header.Get("X-Rumbo-Organizacion") != "org-1" ||
		recibida.Header.Get("X-Rumbo-Usuario") != "usr-7" {
		t.Errorf("llegó identidad del cliente: %v", recibida.Header)
	}

	// En una ruta pública (sin token) no llega ninguna identidad.
	e.pedir(http.MethodPost, "/v2/auth/token", "", falsos)
	for _, nombre := range []string{"X-Rumbo-Usuario", "X-Rumbo-Organizacion", "X-Rumbo-Roles"} {
		if _, hay := e.identidad.recibida.Header[nombre]; hay {
			t.Errorf("%s llegó a Identidad por una ruta pública", nombre)
		}
	}
}

func TestLasRutasInternasNoSeExponen(t *testing.T) {
	e := nuevoEntorno(t)
	w := e.pedir(http.MethodPost, "/api/v1/operaciones/internas/v1/solicitudes/sol-1/cancelacion",
		token(t, "org-1", "sistema_externo"), nil)
	if w.Code != http.StatusNotFound || e.operaciones.llamadas != 0 {
		t.Fatalf("estado = %d, llamadas = %d", w.Code, e.operaciones.llamadas)
	}
}

func TestLasRutasPublicasPasanSinToken(t *testing.T) {
	e := nuevoEntorno(t)
	if w := e.pedir(http.MethodPost, "/v2/auth/token", "", nil); w.Code != http.StatusNoContent {
		t.Errorf("POST /v2/auth/token: estado = %d", w.Code)
	}
	if w := e.pedir(http.MethodPost, "/api/v1/identidad/login", "", nil); w.Code != http.StatusNoContent {
		t.Errorf("POST /api/v1/identidad/login: estado = %d", w.Code)
	}
	if got := e.identidad.recibida.URL.Path; got != "/login" {
		t.Errorf("el login se reenvía a Identidad como /login, llegó %q", got)
	}
}

func TestElPedidoOnceEnRafagaDeLaMismaOrganizacionRecibe429(t *testing.T) {
	e := nuevoEntorno(t)
	tokenOrg1 := token(t, "org-1", "sistema_externo")

	for i := 1; i <= 10; i++ {
		if w := e.pedir(http.MethodGet, "/v2/traslados", tokenOrg1, nil); w.Code != http.StatusNoContent {
			t.Fatalf("pedido %d: estado = %d", i, w.Code)
		}
	}
	w := e.pedir(http.MethodGet, "/v2/traslados", tokenOrg1, nil)
	if w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") == "" {
		t.Fatalf("pedido 11: estado = %d, Retry-After = %q", w.Code, w.Header().Get("Retry-After"))
	}
	if w := e.pedir(http.MethodGet, "/v2/traslados", token(t, "org-2", "sistema_externo"), nil); w.Code != http.StatusNoContent {
		t.Errorf("otra organización: estado = %d, esperaba que no se vea afectada", w.Code)
	}
}

func TestUnaRutaInexistenteResponde404ConProblemDetails(t *testing.T) {
	e := nuevoEntorno(t)
	w := e.pedir(http.MethodGet, "/no-existe", token(t, "org-1", "cliente"), nil)
	if w.Code != http.StatusNotFound || w.Header().Get("Content-Type") != "application/problem+json" {
		t.Fatalf("estado = %d, content-type = %q", w.Code, w.Header().Get("Content-Type"))
	}
}
