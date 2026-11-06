package identidad

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func routerConSecreto(t *testing.T, secreto string) *gin.Engine {
	t.Helper()
	t.Setenv(VariableSecretoInterno, secreto)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(ExigirSecretoInterno())
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.GET("/salud", func(c *gin.Context) { c.Status(http.StatusOK) })
	return r
}

func pedir(r http.Handler, ruta, secreto string) int {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, ruta, nil)
	if secreto != "" {
		req.Header.Set(HeaderInterno, secreto)
	}
	r.ServeHTTP(w, req)
	return w.Code
}

func TestSecretoInternoCorrectoPasa(t *testing.T) {
	r := routerConSecreto(t, "secreto-de-prueba")
	if got := pedir(r, "/x", "secreto-de-prueba"); got != http.StatusOK {
		t.Fatalf("con el secreto correcto se esperaba 200, se obtuvo %d", got)
	}
}

func TestSecretoInternoIncorrectoOFaltanteDa401(t *testing.T) {
	r := routerConSecreto(t, "secreto-de-prueba")
	for nombre, secreto := range map[string]string{"incorrecto": "otro", "faltante": "", "prefijo": "secreto"} {
		if got := pedir(r, "/x", secreto); got != http.StatusUnauthorized {
			t.Errorf("secreto %s: se esperaba 401, se obtuvo %d", nombre, got)
		}
	}
}

func TestSaludNoPideSecretoInterno(t *testing.T) {
	r := routerConSecreto(t, "secreto-de-prueba")
	if got := pedir(r, "/salud", ""); got != http.StatusOK {
		t.Fatalf("/salud sin secreto: se esperaba 200, se obtuvo %d", got)
	}
}

func TestSinVariableNoSeExigeElSecreto(t *testing.T) {
	r := routerConSecreto(t, "")
	if got := pedir(r, "/x", ""); got != http.StatusOK {
		t.Fatalf("con SECRETO_INTERNO vacío se esperaba 200, se obtuvo %d", got)
	}
}
