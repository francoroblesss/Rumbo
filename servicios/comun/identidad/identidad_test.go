package identidad

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestDesdeHeaders(t *testing.T) {
	h := http.Header{}
	h.Set(HeaderUsuario, "usr-1")
	h.Set(HeaderOrganizacion, "org-1")
	h.Set(HeaderRoles, "operador, administrador")
	i := DesdeHeaders(h)
	if i.Usuario != "usr-1" || i.Organizacion != "org-1" || !i.TieneRol(RolAdministrador) || i.TieneRol(RolChofer) {
		t.Fatalf("identidad mal leída: %+v", i)
	}
}

func TestRequiereRolRechazaSinRol(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/x", RequiereRol(RolOperador), func(c *gin.Context) { c.Status(http.StatusOK) })

	sinRol := httptest.NewRecorder()
	r.ServeHTTP(sinRol, httptest.NewRequest(http.MethodGet, "/x", nil))
	if sinRol.Code != http.StatusForbidden {
		t.Fatalf("se esperaba 403, se obtuvo %d", sinRol.Code)
	}

	conRol := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set(HeaderRoles, RolOperador)
	r.ServeHTTP(conRol, req)
	if conRol.Code != http.StatusOK {
		t.Fatalf("se esperaba 200, se obtuvo %d", conRol.Code)
	}
}
