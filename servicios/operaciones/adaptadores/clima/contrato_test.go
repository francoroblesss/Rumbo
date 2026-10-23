package clima

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/sony/gobreaker/v2"

	"github.com/francoroblesss/Rumbo/servicios/operaciones/puertos"
)

// Test de contrato del consumidor (ADR-012): el adaptador se prueba contra la copia fijada del
// contrato del otro grupo. El servidor de prueba responde con los ejemplos del propio contrato
// y valida que el pedido que arma el adaptador lo cumple. Si el proveedor publica un cambio
// incompatible, al actualizar la copia este test falla.

// Desde servicios/operaciones/adaptadores/clima hasta la raíz del repo hay cuatro niveles.
var rutaContrato = filepath.Join("..", "..", "..", "..", "docs", "contracts", "consumidos", "clima.yaml")

const rutaOperacion = "/airports/{icao}/conditions"

func cargarContrato(t *testing.T) *openapi3.T {
	t.Helper()
	if _, err := os.Stat(rutaContrato); err != nil {
		t.Fatalf("no se encontró el contrato fijado en %s: %v", rutaContrato, err)
	}
	doc, err := openapi3.NewLoader().LoadFromFile(rutaContrato)
	if err != nil {
		t.Fatalf("cargar contrato: %v", err)
	}
	return doc
}

// ejemplo devuelve, en JSON, un ejemplo de respuesta del contrato.
func ejemplo(t *testing.T, doc *openapi3.T, estado int, nombre string) []byte {
	t.Helper()
	op := doc.Paths.Find(rutaOperacion).Get
	r := op.Responses.Status(estado).Value
	var contenido *openapi3.MediaType
	for _, tipo := range []string{"application/json", "application/problem+json"} {
		if c := r.Content.Get(tipo); c != nil {
			contenido = c
		}
	}
	if contenido == nil {
		t.Fatalf("el contrato no tiene contenido para %d", estado)
	}
	var valor any
	if nombre != "" {
		e := contenido.Examples[nombre]
		if e == nil || e.Value == nil {
			t.Fatalf("el contrato no tiene el ejemplo %q para %d", nombre, estado)
		}
		valor = e.Value.Value
	} else {
		valor = contenido.Example
	}
	crudo, err := json.Marshal(valor)
	if err != nil {
		t.Fatal(err)
	}
	return crudo
}

// proveedor es el servidor de prueba: valida cada pedido contra el contrato y responde lo
// configurado.
type proveedor struct {
	mu       sync.Mutex
	pedidos  int
	errores  []error
	responde func(w http.ResponseWriter, intento int)
}

func nuevoProveedor(t *testing.T, doc *openapi3.T) (*proveedor, *httptest.Server) {
	t.Helper()
	p := &proveedor{}
	var servidor *httptest.Server
	servidor = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		p.pedidos++
		intento := p.pedidos
		p.mu.Unlock()
		if err := validarPedido(r, doc, servidor.URL); err != nil {
			p.mu.Lock()
			p.errores = append(p.errores, err)
			p.mu.Unlock()
		}
		p.responde(w, intento)
	}))
	t.Cleanup(servidor.Close)
	t.Cleanup(func() {
		for _, err := range p.errores {
			t.Errorf("el pedido no cumple el contrato: %v", err)
		}
	})
	return p, servidor
}

// validarPedido verifica ruta, método, parámetro y header X-API-Key con openapi3filter.
func validarPedido(r *http.Request, doc *openapi3.T, base string) error {
	copia := *doc
	copia.Servers = openapi3.Servers{{URL: base}}
	router, err := legacy.NewRouter(&copia)
	if err != nil {
		return err
	}
	// Del lado del servidor la URL llega sin esquema ni host; el router los necesita para
	// compararla con servers.
	r.URL.Scheme, r.URL.Host = "http", r.Host
	ruta, parametros, err := router.FindRoute(r)
	if err != nil {
		return err
	}
	return openapi3filter.ValidateRequest(r.Context(), &openapi3filter.RequestValidationInput{
		Request: r, PathParams: parametros, Route: ruta,
		Options: &openapi3filter.Options{
			AuthenticationFunc: func(_ context.Context, in *openapi3filter.AuthenticationInput) error {
				esquema := in.SecurityScheme
				if esquema.Type != "apiKey" || esquema.In != "header" || in.RequestValidationInput.Request.Header.Get(esquema.Name) == "" {
					return errors.New("falta la clave " + esquema.Name)
				}
				return nil
			},
		},
	})
}

func responder(estado int, cuerpo []byte, headers map[string]string) func(http.ResponseWriter, int) {
	return func(w http.ResponseWriter, _ int) {
		tipo := "application/problem+json"
		if estado == http.StatusOK {
			tipo = "application/json"
		}
		w.Header().Set("Content-Type", tipo)
		for k, v := range headers {
			w.Header().Set(k, v)
		}
		w.WriteHeader(estado)
		_, _ = w.Write(cuerpo)
	}
}

func TestContratoEjemplosDeRespuesta(t *testing.T) {
	doc := cargarContrato(t)
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatalf("el contrato fijado no es válido: %v", err)
	}
	ctx := context.Background()

	t.Run("operable", func(t *testing.T) {
		p, srv := nuevoProveedor(t, doc)
		p.responde = responder(200, ejemplo(t, doc, 200, "operable"), nil)
		o, err := NuevoCliente(srv.URL, "clave-de-prueba").Operabilidad(ctx, "SACO")
		if err != nil {
			t.Fatal(err)
		}
		if !o.Operable || len(o.Motivos) != 0 || !o.ObservadoEn.Equal(time.Date(2026, 10, 9, 14, 0, 0, 0, time.UTC)) {
			t.Fatalf("operabilidad inesperada: %+v", o)
		}
	})

	t.Run("not_operable", func(t *testing.T) {
		p, srv := nuevoProveedor(t, doc)
		p.responde = responder(200, ejemplo(t, doc, 200, "not_operable"), nil)
		o, err := NuevoCliente(srv.URL, "clave-de-prueba").Operabilidad(ctx, "SACO")
		if err != nil {
			t.Fatal(err)
		}
		if o.Operable || len(o.Motivos) != 2 || o.Motivos[0] != "HIGH_WIND" || o.Motivos[1] != "HIGH_GUSTS" {
			t.Fatalf("operabilidad inesperada: %+v", o)
		}
	})

	t.Run("caché de 5 min: la segunda consulta no llega al proveedor", func(t *testing.T) {
		p, srv := nuevoProveedor(t, doc)
		p.responde = responder(200, ejemplo(t, doc, 200, "operable"), nil)
		c := NuevoCliente(srv.URL, "clave-de-prueba")
		for range 3 {
			if _, err := c.Operabilidad(ctx, "SACO"); err != nil {
				t.Fatal(err)
			}
		}
		if p.pedidos != 1 {
			t.Fatalf("pedidos = %d, se esperaba 1", p.pedidos)
		}
	})
}

func TestContratoErrores(t *testing.T) {
	doc := cargarContrato(t)
	ctx := context.Background()
	ejemplo401 := ejemplo(t, doc, 401, "invalid")

	casos := []struct {
		nombre   string
		estado   int
		cuerpo   []byte
		headers  map[string]string
		pedidos  int // cuántos pedidos tiene que hacer el adaptador
		cuentaEn bool
	}{
		{"401 sin reintento", 401, ejemplo401, nil, 1, false},
		{"404 sin reintento", 404, ejemplo(t, doc, 404, ""), nil, 1, false},
		{"400 sin reintento", 400, ejemplo(t, doc, 400, ""), nil, 1, false},
		{"429 con Retry-After 30: no se espera", 429, ejemplo(t, doc, 429, ""), map[string]string{"Retry-After": "30"}, 1, false},
		{"503 con Retry-After 30: no se espera", 503, ejemplo(t, doc, 503, ""), map[string]string{"Retry-After": "30"}, 1, true},
		{"503 con Retry-After 1: un reintento", 503, ejemplo(t, doc, 503, ""), map[string]string{"Retry-After": "1"}, 2, true},
		{"500: un reintento", 500, nil, nil, 2, true},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			p, srv := nuevoProveedor(t, doc)
			p.responde = responder(c.estado, c.cuerpo, c.headers)
			cliente := NuevoCliente(srv.URL, "clave-de-prueba")
			_, err := cliente.Operabilidad(ctx, "SACO")
			if !errors.Is(err, puertos.ErrClimaNoDisponible) {
				t.Fatalf("error = %v, se esperaba ErrClimaNoDisponible", err)
			}
			if p.pedidos != c.pedidos {
				t.Fatalf("pedidos = %d, se esperaban %d", p.pedidos, c.pedidos)
			}
			if fallas := cliente.breaker.Counts().TotalFailures; (fallas > 0) != c.cuentaEn {
				t.Fatalf("fallas del breaker = %d", fallas)
			}
			// Un error no se guarda en la caché: la próxima consulta vuelve a preguntar.
			_, _ = cliente.Operabilidad(ctx, "SACO")
			if p.pedidos <= c.pedidos {
				t.Fatal("un error no tiene que quedar en la caché")
			}
		})
	}

	t.Run("503 y después 200: el reintento recupera el dato", func(t *testing.T) {
		p, srv := nuevoProveedor(t, doc)
		error503 := responder(503, ejemplo(t, doc, 503, ""), map[string]string{"Retry-After": "1"})
		ok := responder(200, ejemplo(t, doc, 200, "operable"), nil)
		p.responde = func(w http.ResponseWriter, intento int) {
			if intento == 1 {
				error503(w, intento)
				return
			}
			ok(w, intento)
		}
		if _, err := NuevoCliente(srv.URL, "clave-de-prueba").Operabilidad(ctx, "SACO"); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("timeout: no verificado", func(t *testing.T) {
		p, srv := nuevoProveedor(t, doc)
		p.responde = func(w http.ResponseWriter, _ int) { time.Sleep(TimeoutIntento + 200*time.Millisecond) }
		inicio := time.Now()
		_, err := NuevoCliente(srv.URL, "clave-de-prueba").Operabilidad(ctx, "SACO")
		if !errors.Is(err, puertos.ErrClimaNoDisponible) {
			t.Fatalf("error = %v", err)
		}
		if p.pedidos != 2 || time.Since(inicio) > 3*time.Second {
			t.Fatalf("pedidos = %d en %v: un reintento, cada uno cortado a 1 s", p.pedidos, time.Since(inicio))
		}
	})
}

// El validador tiene que rechazar pedidos que no cumplen el contrato (si no, el test de
// contrato no probaría nada).
func TestValidadorDetectaPedidosInvalidos(t *testing.T) {
	doc := cargarContrato(t)
	base := "http://proveedor.test"
	sinClave := httptest.NewRequest(http.MethodGet, base+"/airports/SACO/conditions", nil)
	icaoInvalido := httptest.NewRequest(http.MethodGet, base+"/airports/cor/conditions", nil)
	icaoInvalido.Header.Set("X-API-Key", "clave")
	for nombre, r := range map[string]*http.Request{"sin X-API-Key": sinClave, "ICAO inválido": icaoInvalido} {
		r.Host = "proveedor.test"
		if err := validarPedido(r, doc, base); err == nil {
			t.Fatalf("%s: el validador tenía que rechazarlo", nombre)
		}
	}
}

func TestBreakerSeAbreConCincoFallas(t *testing.T) {
	doc := cargarContrato(t)
	ctx := context.Background()
	p, srv := nuevoProveedor(t, doc)
	p.responde = responder(500, nil, nil)
	cliente := NuevoCliente(srv.URL, "clave-de-prueba")

	// Cada consulta hace 2 intentos (con reintento): a la tercera consulta llega a 5 fallas.
	for range 3 {
		_, _ = cliente.Operabilidad(ctx, "SACO")
	}
	if p.pedidos != FallasParaAbrir {
		t.Fatalf("pedidos = %d, se esperaban %d", p.pedidos, FallasParaAbrir)
	}
	if estado := cliente.breaker.State(); estado != gobreaker.StateOpen {
		t.Fatalf("estado del breaker = %s, se esperaba abierto", estado)
	}
	// Abierto: falla rápido, sin llamar al proveedor.
	_, err := cliente.Operabilidad(ctx, "SACO")
	if !errors.Is(err, puertos.ErrClimaNoDisponible) || p.pedidos != FallasParaAbrir {
		t.Fatalf("error = %v, pedidos = %d", err, p.pedidos)
	}
}
