package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/francoroblesss/Rumbo/servicios/operaciones/dominio"
	"github.com/francoroblesss/Rumbo/servicios/operaciones/puertos"
)

// busquedaFalsa responde lo configurado y guarda la última consulta.
type busquedaFalsa struct {
	err         error
	consulta    puertos.ConsultaBusqueda
	llamadas    int
	reindexadas int
}

func (f *busquedaFalsa) Buscar(_ context.Context, c puertos.ConsultaBusqueda) (puertos.PaginaBusqueda, error) {
	f.consulta = c
	f.llamadas++
	if f.err != nil {
		return puertos.PaginaBusqueda{}, f.err
	}
	return puertos.PaginaBusqueda{Resultados: []puertos.ResultadoBusqueda{{SolicitudID: "sol-1"}}, Pagina: c.Pagina, Tamanio: c.Tamanio, Total: 1}, nil
}

func (f *busquedaFalsa) IndexarEvento(context.Context, string, puertos.Vista, time.Time) error {
	return nil
}

func (f *busquedaFalsa) Reindexar(context.Context) error {
	f.reindexadas++
	return f.err
}

func pedirBusqueda(t *testing.T, f *busquedaFalsa, metodo, ruta string, headers map[string]string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	NuevoHandlerBusqueda(f).Registrar(r)
	req := httptest.NewRequest(metodo, ruta, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var cuerpo map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &cuerpo)
	return w, cuerpo
}

func TestBusquedaRoles(t *testing.T) {
	casos := []struct {
		nombre, metodo, ruta string
		headers              map[string]string
		estado               int
	}{
		{"el sistema externo no busca", http.MethodGet, "/traslados/busqueda", map[string]string{"X-Rumbo-Roles": "sistema_externo", "X-Rumbo-Organizacion": "org-1"}, 403},
		{"un cliente sin organización", http.MethodGet, "/traslados/busqueda", map[string]string{"X-Rumbo-Roles": "cliente"}, 403},
		{"sin roles", http.MethodGet, "/traslados/busqueda", nil, 403},
		{"el operador no reindexa", http.MethodPost, "/admin/reindexacion", operador, 403},
		{"el cliente no reindexa", http.MethodPost, "/admin/reindexacion", cliente, 403},
		{"el operador busca", http.MethodGet, "/traslados/busqueda", operador, 200},
		{"el administrador busca", http.MethodGet, "/traslados/busqueda", administrador, 200},
		{"el cliente busca", http.MethodGet, "/traslados/busqueda", cliente, 200},
		{"el administrador reindexa", http.MethodPost, "/admin/reindexacion", administrador, 202},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			f := &busquedaFalsa{}
			w, p := pedirBusqueda(t, f, c.metodo, c.ruta, c.headers)
			if w.Code != c.estado {
				t.Fatalf("estado = %d, se esperaba %d: %v", w.Code, c.estado, p)
			}
			if c.estado == 403 && (p["codigo"] != "SIN_PERMISO" || f.llamadas+f.reindexadas != 0) {
				t.Fatalf("codigo = %v, llamadas = %d", p["codigo"], f.llamadas+f.reindexadas)
			}
			if c.estado == 202 && p["estado"] != "en_curso" {
				t.Fatalf("cuerpo = %v", p)
			}
		})
	}
}

func TestBusquedaOrganizacion(t *testing.T) {
	t.Run("al cliente se le fuerza la suya e ignora organizacion_id", func(t *testing.T) {
		f := &busquedaFalsa{}
		if w, _ := pedirBusqueda(t, f, http.MethodGet, "/traslados/busqueda?organizacion_id=org-2", cliente); w.Code != 200 || f.consulta.Organizacion != "org-1" {
			t.Fatalf("estado = %d, organización = %q", w.Code, f.consulta.Organizacion)
		}
	})
	t.Run("el operador elige la organización", func(t *testing.T) {
		f := &busquedaFalsa{}
		if w, _ := pedirBusqueda(t, f, http.MethodGet, "/traslados/busqueda?organizacion_id=org-2", operador); w.Code != 200 || f.consulta.Organizacion != "org-2" {
			t.Fatalf("estado = %d, organización = %q", w.Code, f.consulta.Organizacion)
		}
	})
	t.Run("el operador sin filtro ve todas", func(t *testing.T) {
		f := &busquedaFalsa{}
		if w, _ := pedirBusqueda(t, f, http.MethodGet, "/traslados/busqueda", con(operador, "X-Rumbo-Organizacion", "org-9")); w.Code != 200 || f.consulta.Organizacion != "" {
			t.Fatalf("estado = %d, organización = %q", w.Code, f.consulta.Organizacion)
		}
	})
}

func TestBusquedaParametros(t *testing.T) {
	f := &busquedaFalsa{}
	w, p := pedirBusqueda(t, f, http.MethodGet,
		"/traslados/busqueda?q=cajas+rojas&estado=EN_CURSO&desde=2026-10-16&hasta=2026-10-17&alerta=true&orden=relevancia&pagina=2&tamanio=5", operador)
	if w.Code != 200 || p["total"] != 1.0 {
		t.Fatalf("estado = %d: %v", w.Code, p)
	}
	esperada := puertos.ConsultaBusqueda{Texto: "cajas rojas", Estado: "EN_CURSO", Desde: "2026-10-16", Hasta: "2026-10-17",
		SoloAlerta: true, Orden: "relevancia", Pagina: 2, Tamanio: 5}
	if f.consulta != esperada {
		t.Fatalf("consulta = %+v", f.consulta)
	}

	if w, _ := pedirBusqueda(t, f, http.MethodGet, "/traslados/busqueda", operador); w.Code != 200 || f.consulta.Pagina != 1 || f.consulta.Tamanio != 20 || f.consulta.SoloAlerta {
		t.Fatalf("por defecto: estado = %d, consulta = %+v", w.Code, f.consulta)
	}
	if w, _ := pedirBusqueda(t, f, http.MethodGet, "/traslados/busqueda?alerta=false", operador); w.Code != 200 || f.consulta.SoloAlerta {
		t.Fatalf("alerta=false: estado = %d", w.Code)
	}

	t.Run("alerta=si: 422 sin llamar al caso de uso", func(t *testing.T) {
		f := &busquedaFalsa{}
		w, p := pedirBusqueda(t, f, http.MethodGet, "/traslados/busqueda?alerta=si", operador)
		if w.Code != 422 || p["codigo"] != "DATOS_INVALIDOS" || f.llamadas != 0 {
			t.Fatalf("estado = %d, llamadas = %d: %v", w.Code, f.llamadas, p)
		}
	})
	t.Run("pagina no numérica: 400", func(t *testing.T) {
		if w, _ := pedirBusqueda(t, &busquedaFalsa{}, http.MethodGet, "/traslados/busqueda?pagina=uno", operador); w.Code != 400 {
			t.Fatalf("estado = %d", w.Code)
		}
	})
}

func TestBusquedaErrores(t *testing.T) {
	casos := []struct {
		nombre, metodo, ruta string
		headers              map[string]string
		err                  error
		estado               int
		codigo               string
	}{
		{"filtro inválido", http.MethodGet, "/traslados/busqueda?estado=ASIGNADO", operador, dominio.ErrDatosInvalidos, 422, "DATOS_INVALIDOS"},
		{"Solr caído", http.MethodGet, "/traslados/busqueda", cliente, fmt.Errorf("%w: timeout", puertos.ErrBusquedaNoDisponible), 503, "SERVICIO_NO_DISPONIBLE"},
		{"reindexar sin motor", http.MethodPost, "/admin/reindexacion", administrador, puertos.ErrBusquedaNoDisponible, 503, "SERVICIO_NO_DISPONIBLE"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			w, p := pedirBusqueda(t, &busquedaFalsa{err: c.err}, c.metodo, c.ruta, c.headers)
			if w.Code != c.estado || p["codigo"] != c.codigo {
				t.Fatalf("estado = %d: %v", w.Code, p)
			}
		})
	}
}
