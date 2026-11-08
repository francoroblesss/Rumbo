package observabilidad

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/francoroblesss/Rumbo/servicios/comun/identidad"
)

// headerRecibido levanta un servidor que devuelve el valor de un header de la request.
func headerRecibido(t *testing.T, cliente *http.Client, header string) string {
	t.Helper()
	var recibido string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recibido = r.Header.Get(header)
	}))
	defer srv.Close()
	req, _ := http.NewRequestWithContext(ConCorrelacion(context.Background(), "corr-1"), http.MethodGet, srv.URL, nil)
	resp, err := cliente.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return recibido
}

func TestClienteInternoMandaElSecretoYLaCorrelacion(t *testing.T) {
	t.Setenv(identidad.VariableSecretoInterno, "secreto-de-prueba")
	cliente := ClienteInterno(time.Second)
	if got := headerRecibido(t, cliente, identidad.HeaderInterno); got != "secreto-de-prueba" {
		t.Fatalf("ClienteInterno no mandó el secreto: %q", got)
	}
	if got := headerRecibido(t, cliente, HeaderCorrelacion); got != "corr-1" {
		t.Fatalf("ClienteInterno no propagó la correlación: %q", got)
	}
}

// ClienteHTTP es para sistemas de otros grupos: nunca les manda nuestro secreto.
func TestClienteHTTPNoMandaElSecreto(t *testing.T) {
	t.Setenv(identidad.VariableSecretoInterno, "secreto-de-prueba")
	if got := headerRecibido(t, ClienteHTTP(time.Second), identidad.HeaderInterno); got != "" {
		t.Fatalf("ClienteHTTP mandó el secreto interno: %q", got)
	}
}

func TestSaludRespondeSinPasarPorElMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	Salud(r, "prueba")
	r.Use(func(c *gin.Context) { c.AbortWithStatus(http.StatusTeapot) }) // como el secreto o las métricas
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/salud", nil))
	var cuerpo map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &cuerpo)
	if w.Code != http.StatusOK || cuerpo["estado"] != "ok" || cuerpo["servicio"] != "prueba" {
		t.Fatalf("/salud: %d %v", w.Code, cuerpo)
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
	if w.Code != http.StatusTeapot {
		t.Fatalf("una ruta registrada después sí tiene que pasar por el middleware: %d", w.Code)
	}
}

// Los monitores externos (UptimeRobot gratis) usan HEAD: /salud tiene que responder 200 también.
func TestSaludRespondeHEAD(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	Salud(r, "prueba")
	r.Use(func(c *gin.Context) { c.AbortWithStatus(http.StatusTeapot) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodHead, "/salud", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("HEAD /salud: %d, se esperaba 200", w.Code)
	}
}
