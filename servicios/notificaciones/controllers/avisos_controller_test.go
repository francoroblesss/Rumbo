package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/francoroblesss/Rumbo/servicios/notificaciones/models"
	"github.com/francoroblesss/Rumbo/servicios/notificaciones/repositories"
	"github.com/francoroblesss/Rumbo/servicios/notificaciones/services"
)

var (
	alOperador = models.Destinatario{Tipo: "ROL", Valor: "operador"}
	aLaOrg     = models.Destinatario{Tipo: "ORGANIZACION", Valor: "org-almacenamiento"}
)

// routerDePrueba arma el router como main.go (sin observabilidad ni secreto, que se prueban en
// comun), con intervalos cortos para que los tests del stream no tarden.
func routerDePrueba(repo *repositories.AvisosMock, extra ...gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	ac := NewAvisosController(services.NewAvisosService(repo, time.Now))
	ac.intervaloConsulta = 20 * time.Millisecond
	ac.intervaloPing = 50 * time.Millisecond
	r := gin.New()
	r.Use(extra...)
	ac.RegistrarStream(r)
	ac.Registrar(r)
	return r
}

func pedido(metodo, ruta, roles, org string) *http.Request {
	req := httptest.NewRequest(metodo, ruta, nil)
	if roles != "" {
		req.Header.Set("X-Rumbo-Roles", roles)
	}
	if org != "" {
		req.Header.Set("X-Rumbo-Organizacion", org)
	}
	return req
}

func atender(r *gin.Engine, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func repoConAvisos(t *testing.T) *repositories.AvisosMock {
	t.Helper()
	repo := repositories.NewAvisosMock()
	base := time.Date(2026, 11, 20, 13, 0, 0, 0, time.UTC)
	for i, a := range []models.Aviso{
		{ID: "a-op", Destinatario: alOperador, Titulo: "Reserva nueva"},
		{ID: "a-org", Destinatario: aLaOrg, Titulo: "Traslado en curso"},
	} {
		a.CreadoEn = base.Add(time.Duration(i) * time.Second)
		if err := repo.Insertar(context.Background(), a); err != nil {
			t.Fatal(err)
		}
	}
	return repo
}

func TestRolesDeLaBandeja(t *testing.T) {
	r := routerDePrueba(repoConAvisos(t))
	// Sin un rol con bandeja: 403 en las tres rutas (el stream con un rol válido se prueba en
	// stream_test.go).
	for _, roles := range []string{"", "sistema_externo", "chofer"} {
		for _, req := range []*http.Request{
			pedido(http.MethodGet, "/avisos", roles, ""),
			pedido(http.MethodGet, "/avisos/stream", roles, ""),
			pedido(http.MethodPost, "/avisos/a-op/lectura", roles, ""),
		} {
			if rec := atender(r, req); rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "SIN_PERMISO") {
				t.Errorf("%s %s con roles %q: %d", req.Method, req.URL.Path, roles, rec.Code)
			}
		}
	}
	for _, roles := range []string{"operador", "administrador", "cliente"} {
		if rec := atender(r, pedido(http.MethodGet, "/avisos", roles, "")); rec.Code != http.StatusOK {
			t.Errorf("GET /avisos con roles %q: %d", roles, rec.Code)
		}
	}
}

func TestListarAvisos(t *testing.T) {
	r := routerDePrueba(repoConAvisos(t))
	rec := atender(r, pedido(http.MethodGet, "/avisos", "cliente", "org-almacenamiento"))
	if rec.Code != http.StatusOK {
		t.Fatalf("estado %d: %s", rec.Code, rec.Body)
	}
	var p struct {
		Resultados []map[string]any `json:"resultados"`
		Pagina     int              `json:"pagina"`
		Tamanio    int              `json:"tamanio"`
		Total      int              `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	if p.Total != 1 || p.Pagina != 1 || p.Tamanio != 20 || p.Resultados[0]["id"] != "a-org" {
		t.Fatalf("el cliente ve solo el de su organización: %s", rec.Body)
	}
	if creado := p.Resultados[0]["creado_en"]; creado != "2026-11-20T10:00:01-03:00" {
		t.Errorf("creado_en se informa en -03:00: %v", creado)
	}
}

func TestListarParametrosInvalidos(t *testing.T) {
	r := routerDePrueba(repoConAvisos(t))
	for _, q := range []string{"?pagina=0", "?tamanio=101", "?tamanio=x", "?no_leidos=quizas"} {
		if rec := atender(r, pedido(http.MethodGet, "/avisos"+q, "operador", "")); rec.Code != http.StatusUnprocessableEntity ||
			!strings.Contains(rec.Body.String(), "DATOS_INVALIDOS") {
			t.Errorf("%s: %d %s", q, rec.Code, rec.Body)
		}
	}
}

func TestMarcarLeido(t *testing.T) {
	r := routerDePrueba(repoConAvisos(t))
	if rec := atender(r, pedido(http.MethodPost, "/avisos/a-op/lectura", "operador", "")); rec.Code != http.StatusNoContent {
		t.Fatalf("el operador marca su aviso: %d %s", rec.Code, rec.Body)
	}
	if rec := atender(r, pedido(http.MethodPost, "/avisos/a-op/lectura", "cliente", "org-almacenamiento")); rec.Code != http.StatusNotFound {
		t.Errorf("el cliente no puede marcar uno del operador: %d", rec.Code)
	}
	if rec := atender(r, pedido(http.MethodPost, "/avisos/no-existe/lectura", "operador", "")); rec.Code != http.StatusNotFound ||
		!strings.Contains(rec.Body.String(), "NO_ENCONTRADA") {
		t.Errorf("uno que no existe: %d %s", rec.Code, rec.Body)
	}
}

func TestMongoCaidoResponde503(t *testing.T) {
	repo := repoConAvisos(t)
	repo.Error = errors.New("mongo caído")
	r := routerDePrueba(repo)
	for _, req := range []*http.Request{
		pedido(http.MethodGet, "/avisos", "operador", ""),
		pedido(http.MethodPost, "/avisos/a-op/lectura", "operador", ""),
		pedido(http.MethodGet, "/avisos/stream", "operador", ""),
	} {
		req.Header.Set("Last-Event-ID", "a-op") // el stream consulta MongoDB al abrir
		if rec := atender(r, req); rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "SERVICIO_NO_DISPONIBLE") {
			t.Errorf("%s %s: %d %s", req.Method, req.URL.Path, rec.Code, rec.Body)
		}
	}
}
