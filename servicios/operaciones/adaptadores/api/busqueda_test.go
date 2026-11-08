package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/francoroblesss/Rumbo/servicios/operaciones/dominio"
	"github.com/francoroblesss/Rumbo/servicios/operaciones/puertos"
)

// busquedaFalsa guarda la última consulta y responde lo configurado.
type busquedaFalsa struct {
	puertos.Busqueda
	consulta    puertos.ConsultaBusqueda
	err         error
	reindexadas int
}

func (b *busquedaFalsa) Buscar(_ context.Context, c puertos.ConsultaBusqueda) (puertos.PaginaBusqueda, error) {
	b.consulta = c
	return puertos.PaginaBusqueda{Resultados: []puertos.ResultadoBusqueda{}, Pagina: c.Pagina, Tamanio: c.Tamanio}, b.err
}

func (b *busquedaFalsa) Reindexar(context.Context) error {
	b.reindexadas++
	return b.err
}

func pedirBusqueda(b *busquedaFalsa, metodo, ruta string, headers map[string]string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	NuevoHandlerBusqueda(b).Registrar(r)
	req := httptest.NewRequest(metodo, ruta, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestBusquedaFuerzaLaOrganizacionDelCliente(t *testing.T) {
	b := &busquedaFalsa{}
	// CA-25: el cliente pide otra organización y se le ignora; siempre ve la suya.
	w := pedirBusqueda(b, http.MethodGet, "/traslados/busqueda?q=cajas&organizacion_id=org-9&estado=EN_CURSO&desde=2026-11-20&hasta=2026-11-21&alerta=true&orden=relevancia&pagina=2&tamanio=5", cliente)
	c := b.consulta
	if w.Code != http.StatusOK || c.Organizacion != "org-1" || c.Texto != "cajas" || c.Estado != "EN_CURSO" ||
		c.Desde != "2026-11-20" || c.Hasta != "2026-11-21" || !c.SoloConAlerta || c.Orden != "relevancia" || c.Pagina != 2 || c.Tamanio != 5 {
		t.Fatalf("%d %+v", w.Code, c)
	}
	// El operador sí elige la organización (o ninguna).
	if w := pedirBusqueda(b, http.MethodGet, "/traslados/busqueda?organizacion_id=org-9", operador); w.Code != 200 || b.consulta.Organizacion != "org-9" {
		t.Fatalf("operador: %d %+v", w.Code, b.consulta)
	}
	if w := pedirBusqueda(b, http.MethodGet, "/traslados/busqueda", administrador); w.Code != 200 || b.consulta.Organizacion != "" ||
		b.consulta.Pagina != 1 || b.consulta.Tamanio != 20 {
		t.Fatalf("administrador: %d %+v", w.Code, b.consulta)
	}
}

func TestBusquedaRolesYErrores(t *testing.T) {
	casos := []struct {
		nombre, metodo, ruta string
		headers              map[string]string
		err                  error
		estado               int
	}{
		{"el sistema externo no busca", http.MethodGet, "/traslados/busqueda", map[string]string{"X-Rumbo-Roles": "sistema_externo", "X-Rumbo-Organizacion": "org-1"}, nil, 403},
		{"cliente sin organización", http.MethodGet, "/traslados/busqueda", map[string]string{"X-Rumbo-Roles": "cliente"}, nil, 403},
		{"alerta inválida", http.MethodGet, "/traslados/busqueda?alerta=si", operador, nil, 422},
		{"página no numérica", http.MethodGet, "/traslados/busqueda?pagina=x", operador, nil, 400},
		{"filtro inválido", http.MethodGet, "/traslados/busqueda", operador, dominio.ErrDatosInvalidos, 422},
		{"Solr caído", http.MethodGet, "/traslados/busqueda", cliente, puertos.ErrBusquedaNoDisponible, 503},
		{"reindexar: solo el administrador", http.MethodPost, "/admin/reindexacion", operador, nil, 403},
		{"reindexar", http.MethodPost, "/admin/reindexacion", administrador, nil, 202},
		{"reindexar sin Solr", http.MethodPost, "/admin/reindexacion", administrador, puertos.ErrBusquedaNoDisponible, 503},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if w := pedirBusqueda(&busquedaFalsa{err: c.err}, c.metodo, c.ruta, c.headers); w.Code != c.estado {
				t.Fatalf("%d, se esperaba %d: %s", w.Code, c.estado, w.Body.String())
			}
		})
	}
}
