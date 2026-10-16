package controllers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/francoroblesss/Rumbo/servicios/identidad/repositories"
	"github.com/francoroblesss/Rumbo/servicios/identidad/services"
)

func routerDePrueba(t *testing.T) *gin.Engine {
	t.Helper()
	servicio := services.NuevoAuthService(repositories.NuevoUsuariosMock(), repositories.NuevoOrganizacionesMock(),
		"secreto-de-prueba", services.Siembra{
			AdminEmail: "admin@rumbo.local", AdminPassword: "clave-admin",
			ExternoClientID: "externo", ExternoClientSecret: "secreto-externo",
		})
	if err := servicio.Sembrar(context.Background()); err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	NuevoAuthController(servicio).Registrar(r)
	return r
}

func publicar(r *gin.Engine, ruta, cuerpo string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, ruta, strings.NewReader(cuerpo))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

func codigoDe(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	if w.Header().Get("Content-Type") != "application/problem+json" {
		t.Errorf("Content-Type = %q, esperaba application/problem+json", w.Header().Get("Content-Type"))
	}
	var p struct {
		Codigo string `json:"codigo"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	return p.Codigo
}

func TestLoginExitosoResponde200ConToken(t *testing.T) {
	w := publicar(routerDePrueba(t), "/login", `{"email":"admin@rumbo.local","password":"clave-admin"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("estado = %d, cuerpo = %s", w.Code, w.Body)
	}
	var token services.Token
	if err := json.Unmarshal(w.Body.Bytes(), &token); err != nil {
		t.Fatal(err)
	}
	if token.AccessToken == "" || token.TokenType != "Bearer" || token.ExpiraEnSegundos != 3600 {
		t.Fatalf("respuesta inesperada: %+v", token)
	}
}

func TestTokenDeClienteResponde200ConToken(t *testing.T) {
	w := publicar(routerDePrueba(t), "/auth/token", `{"client_id":"externo","client_secret":"secreto-externo"}`)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"access_token"`) {
		t.Fatalf("estado = %d, cuerpo = %s", w.Code, w.Body)
	}
}

func TestCredencialesInvalidasResponden401NoAutenticado(t *testing.T) {
	r := routerDePrueba(t)
	casos := map[string][2]string{
		"login con clave equivocada":    {"/login", `{"email":"admin@rumbo.local","password":"mala"}`},
		"login con usuario inexistente": {"/login", `{"email":"nadie@rumbo.local","password":"x"}`},
		"token con secreto equivocado":  {"/auth/token", `{"client_id":"externo","client_secret":"malo"}`},
	}
	for nombre, caso := range casos {
		w := publicar(r, caso[0], caso[1])
		if w.Code != http.StatusUnauthorized || codigoDe(t, w) != "NO_AUTENTICADO" {
			t.Errorf("%s: estado = %d, cuerpo = %s", nombre, w.Code, w.Body)
		}
	}
}

func TestJSONMalFormadoResponde400(t *testing.T) {
	w := publicar(routerDePrueba(t), "/login", `{"email":`)
	if w.Code != http.StatusBadRequest || codigoDe(t, w) != "JSON_MAL_FORMADO" {
		t.Fatalf("estado = %d, cuerpo = %s", w.Code, w.Body)
	}
}

func TestCamposVaciosResponden422(t *testing.T) {
	r := routerDePrueba(t)
	for ruta, cuerpo := range map[string]string{
		"/login":      `{"email":"","password":""}`,
		"/auth/token": `{}`,
	} {
		w := publicar(r, ruta, cuerpo)
		if w.Code != http.StatusUnprocessableEntity || codigoDe(t, w) != "DATOS_INVALIDOS" {
			t.Errorf("%s: estado = %d, cuerpo = %s", ruta, w.Code, w.Body)
		}
	}
}

func TestLaRespuestaNoFiltraSecretos(t *testing.T) {
	w := publicar(routerDePrueba(t), "/login", `{"email":"admin@rumbo.local","password":"clave-admin"}`)
	for _, prohibido := range []string{"clave-admin", "hash", "$2a$"} {
		if strings.Contains(w.Body.String(), prohibido) {
			t.Errorf("la respuesta contiene %q", prohibido)
		}
	}
}
