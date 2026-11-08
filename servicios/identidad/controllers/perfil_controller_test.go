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

// perfilDePrueba siembra los tres usuarios de §15 y devuelve el router con GET /yo y los
// repositorios (para conocer los ids generados).
func perfilDePrueba(t *testing.T) (*gin.Engine, *repositories.UsuariosMock) {
	t.Helper()
	usuarios := repositories.NuevoUsuariosMock()
	organizaciones := repositories.NuevoOrganizacionesMock()
	siembra := services.NuevoAuthService(usuarios, organizaciones, "secreto-de-prueba", services.Siembra{
		AdminEmail: "admin@rumbo.local", AdminPassword: "clave-admin",
		ExternoClientID: "externo", ExternoClientSecret: "secreto-externo",
		ExternoOrganizacionNombre: "Gestión de almacenamiento",
		OperadorEmail:             "operador@rumbo.local", OperadorPassword: "clave-operador",
		ClienteEmail: "cliente@almacenamiento.local", ClientePassword: "clave-cliente",
	})
	if err := siembra.Sembrar(context.Background()); err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	NuevoPerfilController(services.NuevoPerfilService(usuarios, organizaciones)).Registrar(r)
	return r, usuarios
}

// pedirYo hace GET /yo con los headers que pondría el gateway.
func pedirYo(r *gin.Engine, usuario, organizacion, roles string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/yo", nil)
	req.Header.Set("X-Rumbo-Usuario", usuario)
	req.Header.Set("X-Rumbo-Organizacion", organizacion)
	req.Header.Set("X-Rumbo-Roles", roles)
	r.ServeHTTP(w, req)
	return w
}

type respuestaYo struct {
	ID           string   `json:"id"`
	Email        string   `json:"email"`
	Nombre       string   `json:"nombre"`
	Roles        []string `json:"roles"`
	Organizacion *struct {
		ID     string `json:"id"`
		Nombre string `json:"nombre"`
	} `json:"organizacion"`
}

func TestYoDevuelveElPerfilDeCadaRol(t *testing.T) {
	r, usuarios := perfilDePrueba(t)
	casos := []struct {
		email, roles, organizacion, nombre string
		conOrganizacion                    bool
	}{
		{"admin@rumbo.local", "administrador,operador", "", "Administrador", false},
		{"operador@rumbo.local", "operador", "", "Operador de Rumbo", false},
		{"cliente@almacenamiento.local", "cliente", "org-almacenamiento", "Usuario de almacenamiento", true},
	}
	for _, caso := range casos {
		usuario := usuarios.PorEmail[caso.email]
		w := pedirYo(r, usuario.ID, caso.organizacion, caso.roles)
		if w.Code != http.StatusOK {
			t.Errorf("%s: estado = %d, cuerpo = %s", caso.email, w.Code, w.Body)
			continue
		}
		var perfil respuestaYo
		if err := json.Unmarshal(w.Body.Bytes(), &perfil); err != nil {
			t.Fatal(err)
		}
		if perfil.ID != usuario.ID || perfil.Email != caso.email || perfil.Nombre != caso.nombre || len(perfil.Roles) == 0 {
			t.Errorf("%s: perfil = %+v", caso.email, perfil)
		}
		if caso.conOrganizacion {
			if perfil.Organizacion == nil || perfil.Organizacion.ID != "org-almacenamiento" ||
				perfil.Organizacion.Nombre != "Gestión de almacenamiento" {
				t.Errorf("%s: organización = %+v", caso.email, perfil.Organizacion)
			}
		} else if perfil.Organizacion != nil {
			t.Errorf("%s: el personal de Rumbo tiene organizacion null, vino %+v", caso.email, perfil.Organizacion)
		}
	}
}

func TestYoDelPersonalDeRumboTraeOrganizacionNull(t *testing.T) {
	r, usuarios := perfilDePrueba(t)
	w := pedirYo(r, usuarios.PorEmail["operador@rumbo.local"].ID, "", "operador")
	var crudo map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &crudo); err != nil {
		t.Fatal(err)
	}
	if v, ok := crudo["organizacion"]; !ok || string(v) != "null" {
		t.Errorf("organizacion = %s, esperaba el campo con null", v)
	}
}

func TestYoDeUnUsuarioInexistenteResponde404(t *testing.T) {
	r, _ := perfilDePrueba(t)
	// El sub de un token de cliente API es el client_id, que no es un usuario.
	w := pedirYo(r, "externo", "org-almacenamiento", "sistema_externo")
	if w.Code != http.StatusNotFound || codigoDe(t, w) != "NO_ENCONTRADA" {
		t.Fatalf("estado = %d, cuerpo = %s", w.Code, w.Body)
	}
}

func TestYoSinRolPermitidoResponde403(t *testing.T) {
	r, usuarios := perfilDePrueba(t)
	for _, roles := range []string{"sistema_externo", ""} {
		w := pedirYo(r, usuarios.PorEmail["operador@rumbo.local"].ID, "org-almacenamiento", roles)
		if w.Code != http.StatusForbidden || codigoDe(t, w) != "SIN_PERMISO" {
			t.Errorf("roles %q: estado = %d, cuerpo = %s", roles, w.Code, w.Body)
		}
	}
}

func TestYoNoDevuelveLaContrasenia(t *testing.T) {
	r, usuarios := perfilDePrueba(t)
	w := pedirYo(r, usuarios.PorEmail["cliente@almacenamiento.local"].ID, "org-almacenamiento", "cliente")
	for _, prohibido := range []string{"clave-cliente", "hash", "$2a$"} {
		if strings.Contains(w.Body.String(), prohibido) {
			t.Errorf("la respuesta contiene %q", prohibido)
		}
	}
}
