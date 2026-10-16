package autenticacion

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const secreto = "secreto-de-prueba-bastante-largo"

// firmar arma un token como el de Identidad; las opciones permiten romper una cosa por vez.
func firmar(t *testing.T, metodo jwt.SigningMethod, clave string, modificar func(*reclamos)) string {
	t.Helper()
	ahora := time.Now()
	r := reclamos{
		Org:   "org-1",
		Roles: []string{"operador", "administrador"},
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "rumbo-identidad",
			Audience:  jwt.ClaimStrings{"rumbo"},
			Subject:   "usr-7",
			IssuedAt:  jwt.NewNumericDate(ahora),
			ExpiresAt: jwt.NewNumericDate(ahora.Add(time.Hour)),
		},
	}
	if modificar != nil {
		modificar(&r)
	}
	token, err := jwt.NewWithClaims(metodo, r).SignedString([]byte(clave))
	if err != nil {
		t.Fatal(err)
	}
	return token
}

// backendFalso registra la request que le llega después del middleware.
func backendFalso(llegada **http.Request) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*llegada = r
		w.WriteHeader(http.StatusNoContent)
	})
}

func pedir(t *testing.T, metodo, ruta, autorizacion string) (*httptest.ResponseRecorder, *http.Request) {
	t.Helper()
	var llegada *http.Request
	h := Middleware(secreto)(backendFalso(&llegada))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(metodo, ruta, nil)
	if autorizacion != "" {
		req.Header.Set("Authorization", autorizacion)
	}
	h.ServeHTTP(w, req)
	return w, llegada
}

func verificar401(t *testing.T, nombre string, w *httptest.ResponseRecorder, llegada *http.Request) {
	t.Helper()
	if w.Code != http.StatusUnauthorized {
		t.Errorf("%s: estado = %d, esperaba 401", nombre, w.Code)
	}
	if llegada != nil {
		t.Errorf("%s: la request no tendría que haber llegado al servicio", nombre)
	}
	if w.Header().Get("WWW-Authenticate") != "Bearer" {
		t.Errorf("%s: falta WWW-Authenticate: Bearer", nombre)
	}
	var p struct{ Codigo string }
	_ = json.Unmarshal(w.Body.Bytes(), &p)
	if p.Codigo != "NO_AUTENTICADO" || w.Header().Get("Content-Type") != "application/problem+json" {
		t.Errorf("%s: cuerpo = %s", nombre, w.Body)
	}
}

func TestSinTokenResponde401(t *testing.T) {
	w, llegada := pedir(t, http.MethodGet, "/traslados", "")
	verificar401(t, "sin Authorization", w, llegada)
}

func TestTokensInvalidosResponden401(t *testing.T) {
	casos := map[string]string{
		"vencido": firmar(t, jwt.SigningMethodHS256, secreto, func(r *reclamos) {
			r.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Minute))
		}),
		"firmado con otra clave": firmar(t, jwt.SigningMethodHS256, "otra-clave-distinta", nil),
		"con otra audiencia": firmar(t, jwt.SigningMethodHS256, secreto, func(r *reclamos) {
			r.Audience = jwt.ClaimStrings{"otro-sistema"}
		}),
		"con otro emisor": firmar(t, jwt.SigningMethodHS256, secreto, func(r *reclamos) {
			r.Issuer = "otro-emisor"
		}),
		"sin vencimiento": firmar(t, jwt.SigningMethodHS256, secreto, func(r *reclamos) {
			r.ExpiresAt = nil
		}),
		"sin sub": firmar(t, jwt.SigningMethodHS256, secreto, func(r *reclamos) {
			r.Subject = ""
		}),
		"con otro algoritmo (HS384)": firmar(t, jwt.SigningMethodHS384, secreto, nil),
	}
	for nombre, token := range casos {
		w, llegada := pedir(t, http.MethodGet, "/traslados", "Bearer "+token)
		verificar401(t, nombre, w, llegada)
	}

	for nombre, cabecera := range map[string]string{
		"esquema distinto": "Basic abc",
		"sin token":        "Bearer ",
		"basura":           "Bearer no-es-un-jwt",
	} {
		w, llegada := pedir(t, http.MethodGet, "/traslados", cabecera)
		verificar401(t, nombre, w, llegada)
	}
}

func TestTokenValidoPropagaLaIdentidad(t *testing.T) {
	token := firmar(t, jwt.SigningMethodHS256, secreto, nil)
	w, llegada := pedir(t, http.MethodGet, "/traslados", "Bearer "+token)
	if w.Code != http.StatusNoContent || llegada == nil {
		t.Fatalf("estado = %d, la request tendría que llegar al servicio", w.Code)
	}
	if llegada.Header.Get("X-Rumbo-Usuario") != "usr-7" ||
		llegada.Header.Get("X-Rumbo-Organizacion") != "org-1" ||
		llegada.Header.Get("X-Rumbo-Roles") != "operador,administrador" {
		t.Errorf("identidad propagada = %v", llegada.Header)
	}
}

func TestTokenSinOrganizacionNoPropagaOrganizacion(t *testing.T) {
	token := firmar(t, jwt.SigningMethodHS256, secreto, func(r *reclamos) { r.Org = "" })
	_, llegada := pedir(t, http.MethodGet, "/api/operaciones/solicitudes", "Bearer "+token)
	if llegada == nil {
		t.Fatal("la request tendría que llegar")
	}
	if _, hay := llegada.Header["X-Rumbo-Organizacion"]; hay {
		t.Error("el personal de Rumbo no tiene organización: no se propaga el header")
	}
}

func TestRutasPublicasNoPidenToken(t *testing.T) {
	for _, ruta := range []string{"/auth/token", "/api/identidad/login"} {
		w, llegada := pedir(t, http.MethodPost, ruta, "")
		if w.Code != http.StatusNoContent || llegada == nil {
			t.Errorf("POST %s sin token: estado = %d, esperaba que pase", ruta, w.Code)
		}
	}
	// Solo son públicas con POST: el resto de los métodos y rutas sigue pidiendo token.
	w, llegada := pedir(t, http.MethodGet, "/api/identidad/login", "")
	verificar401(t, "GET login", w, llegada)
	w, llegada = pedir(t, http.MethodPost, "/traslados", "")
	verificar401(t, "POST traslados sin token", w, llegada)
}
