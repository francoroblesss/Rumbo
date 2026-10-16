package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/gin-gonic/gin"

	"github.com/francoroblesss/Rumbo/servicios/integracion/clients"
	"github.com/francoroblesss/Rumbo/servicios/integracion/models"
	"github.com/francoroblesss/Rumbo/servicios/integracion/repositories"
	"github.com/francoroblesss/Rumbo/servicios/integracion/services"
)

// Test de contrato: cada respuesta del controlador (incluidos los errores) se valida contra
// docs/contracts/traslados.yaml. Si este test falla, el código está mal, no el contrato.

const (
	rutaContrato = "../../../docs/contracts/traslados.yaml"
	servidorDoc  = "http://localhost:4010" // servers[0] del contrato, para encontrar la ruta
	orgPrueba    = "org-almacenamiento"
)

var ahoraContrato = time.Date(2026, 10, 15, 11, 30, 0, 0, time.UTC)

const cuerpoCompleto = `{
  "referencia_externa": "TRF-2026-00123",
  "origen": {"nombre": "Sucursal Centro", "direccion": "Av. Colón 1234, Córdoba", "contacto": "Depósito — 351 555-0101"},
  "destino": {"nombre": "Sucursal Cerro", "direccion": "Av. Rafael Núñez 4500, Córdoba"},
  "hora_llegada_solicitada": "2026-10-16T14:00:00-03:00",
  "tolerancia_minutos": 120,
  "cosa": {"descripcion": "Cajas de mercadería seca", "cantidad": 20, "peso_kg": 350, "volumen_m3": 2.5},
  "observaciones": "Entrar por el portón de carga."
}`

const cuerpoMinimoContrato = `{"cosa":{"descripcion":"Cajas","cantidad":3},"origen":{"nombre":"A","direccion":"x"},"destino":{"nombre":"B","direccion":"y"}}`

type entorno struct {
	t       *testing.T
	router  *gin.Engine
	rutas   routers.Router
	repo    *repositories.SolicitudesMock
	cliente *clients.OperacionesMock
}

func nuevoEntorno(t *testing.T) *entorno {
	t.Helper()
	cargador := openapi3.NewLoader()
	doc, err := cargador.LoadFromFile(rutaContrato)
	if err != nil {
		t.Fatalf("no se pudo cargar el contrato: %v", err)
	}
	if err := doc.Validate(cargador.Context); err != nil {
		t.Fatalf("el contrato no es válido: %v", err)
	}
	rutas, err := legacy.NewRouter(doc)
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	repo := repositories.NewSolicitudesMock()
	cliente := &clients.OperacionesMock{}
	router := gin.New()
	NewTrasladosController(services.NewSolicitudesService(repo, cliente, func() time.Time { return ahoraContrato })).Registrar(router)
	return &entorno{t: t, router: router, rutas: rutas, repo: repo, cliente: cliente}
}

type pedido struct {
	metodo, ruta, cuerpo string
	clave                string // Idempotency-Key ("" = no se envía)
	sinOrganizacion      bool
	valido               bool // si true, también se valida el pedido contra el contrato
}

type respuesta struct {
	estado int
	header http.Header
	cuerpo []byte
}

// ejecutar manda el pedido al router y valida la respuesta contra el contrato.
func (e *entorno) ejecutar(p pedido) respuesta {
	e.t.Helper()
	nuevoPedido := func(base string) *http.Request {
		var cuerpo io.Reader
		if p.cuerpo != "" {
			cuerpo = strings.NewReader(p.cuerpo)
		}
		req := httptest.NewRequest(p.metodo, base+p.ruta, cuerpo)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer token-de-prueba")
		req.Header.Set("X-Rumbo-Roles", "sistema_externo")
		if !p.sinOrganizacion {
			req.Header.Set("X-Rumbo-Organizacion", orgPrueba)
		}
		if p.clave != "" {
			req.Header.Set("Idempotency-Key", p.clave)
		}
		return req
	}

	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, nuevoPedido(""))
	r := respuesta{estado: w.Code, header: w.Header(), cuerpo: w.Body.Bytes()}

	reqContrato := nuevoPedido(servidorDoc)
	ruta, parametros, err := e.rutas.FindRoute(reqContrato)
	if err != nil {
		e.t.Fatalf("%s %s no está en el contrato: %v", p.metodo, p.ruta, err)
	}
	opciones := &openapi3filter.Options{IncludeResponseStatus: true, AuthenticationFunc: openapi3filter.NoopAuthenticationFunc}
	entrada := &openapi3filter.RequestValidationInput{Request: reqContrato, PathParams: parametros, Route: ruta, Options: opciones}
	if p.valido {
		if err := openapi3filter.ValidateRequest(context.Background(), entrada); err != nil {
			e.t.Fatalf("el pedido de prueba %s %s no cumple el contrato: %v", p.metodo, p.ruta, err)
		}
	}
	err = openapi3filter.ValidateResponse(context.Background(), &openapi3filter.ResponseValidationInput{
		RequestValidationInput: entrada,
		Status:                 r.estado,
		Header:                 r.header,
		Body:                   io.NopCloser(bytes.NewReader(r.cuerpo)),
		Options:                opciones,
	})
	if err != nil {
		e.t.Fatalf("%s %s → %d no cumple el contrato: %v\ncuerpo: %s", p.metodo, p.ruta, r.estado, err, r.cuerpo)
	}
	return r
}

func (e *entorno) esperar(r respuesta, estado int, codigo string) {
	e.t.Helper()
	if r.estado != estado {
		e.t.Fatalf("se esperaba %d, se obtuvo %d: %s", estado, r.estado, r.cuerpo)
	}
	if codigo != "" {
		var p struct {
			Codigo string `json:"codigo"`
		}
		_ = json.Unmarshal(r.cuerpo, &p)
		if p.Codigo != codigo {
			e.t.Fatalf("se esperaba el código %s, se obtuvo %s", codigo, r.cuerpo)
		}
		if ct := r.header.Get("Content-Type"); !strings.HasPrefix(ct, "application/problem+json") {
			e.t.Fatalf("los errores van como Problem Details, Content-Type: %s", ct)
		}
	}
}

func (e *entorno) crear(clave string) string {
	e.t.Helper()
	r := e.ejecutar(pedido{metodo: http.MethodPost, ruta: "/traslados", cuerpo: cuerpoMinimoContrato, clave: clave, valido: true})
	e.esperar(r, http.StatusAccepted, "")
	var v struct{ ID string }
	_ = json.Unmarshal(r.cuerpo, &v)
	return v.ID
}

// vistaCompleta es una vista como la que manda Operaciones, con campos internos incluidos.
func vistaCompleta(id string, version int64) models.VistaSolicitud {
	t := func(s string) time.Time { v, _ := time.Parse(time.RFC3339, s); return v }
	observado := t("2026-10-15T12:00:00Z")
	demora := 15
	motivo := "La mercadería se despachó por otro medio."
	return models.VistaSolicitud{
		ID: id, Estado: models.EstadoAceptada, Version: version,
		Cosa:              models.Cosa{Descripcion: "Cajas", Cantidad: 3},
		Origen:            models.Ubicacion{Nombre: "A", Direccion: "x"},
		Destino:           models.Ubicacion{Nombre: "B", Direccion: "y"},
		ToleranciaMinutos: 0,
		Contrapropuesta:   &models.Contrapropuesta{HoraLlegadaPropuesta: t("2026-10-16T20:00:00Z"), VenceEn: t("2026-10-16T12:10:00Z")},
		Traslado: &models.Traslado{ID: "tra-1", Estado: "ASIGNADO", HoraLlegadaComprometida: t("2026-10-16T20:00:00Z"), DemoraMinutos: &demora,
			AlertaClimatica: &models.AlertaClimatica{Estado: "NO_OPERABLE", Motivos: []string{"HIGH_WIND"}, ObservadoEn: &observado}},
		Cancelacion: &models.Cancelacion{Estado: "PENDIENTE_APROBACION", Motivo: &motivo, SolicitadaEn: t("2026-10-16T10:30:00Z")},
		CreadoEn:    ahoraContrato, ActualizadoEn: ahoraContrato.Add(time.Hour),
	}
}

func TestContratoSolicitarTraslado(t *testing.T) {
	e := nuevoEntorno(t)
	r := e.ejecutar(pedido{metodo: http.MethodPost, ruta: "/traslados", cuerpo: cuerpoCompleto, clave: "clave-completa-1", valido: true})
	e.esperar(r, http.StatusAccepted, "")
	var creada map[string]any
	_ = json.Unmarshal(r.cuerpo, &creada)
	if creada["estado"] != "RECIBIDA" || r.header.Get("Location") != "/traslados/"+creada["id"].(string) {
		t.Fatalf("respuesta 202 incorrecta: %s (Location %s)", r.cuerpo, r.header.Get("Location"))
	}
	if creada["hora_llegada_solicitada"] != "2026-10-16T14:00:00-03:00" || !strings.HasSuffix(creada["creado_en"].(string), "-03:00") {
		t.Fatalf("las horas se responden con zona -03:00: %s", r.cuerpo)
	}
	if _, ok := creada["version"]; ok {
		t.Fatal("version es interno: no se responde")
	}

	// Reintento idempotente: 202 y la misma solicitud.
	r = e.ejecutar(pedido{metodo: http.MethodPost, ruta: "/traslados", cuerpo: cuerpoCompleto, clave: "clave-completa-1", valido: true})
	e.esperar(r, http.StatusAccepted, "")
	var repetida map[string]any
	_ = json.Unmarshal(r.cuerpo, &repetida)
	if repetida["id"] != creada["id"] {
		t.Fatalf("el reintento tiene que devolver la misma solicitud: %v vs %v", repetida["id"], creada["id"])
	}

	casos := []struct {
		nombre string
		p      pedido
		estado int
		codigo string
	}{
		{"mínimo", pedido{cuerpo: cuerpoMinimoContrato, clave: "clave-minima-1", valido: true}, http.StatusAccepted, ""},
		{"misma clave, otro cuerpo", pedido{cuerpo: cuerpoMinimoContrato, clave: "clave-completa-1"}, http.StatusConflict, "CLAVE_IDEMPOTENCIA_REUTILIZADA"},
		{"datos inválidos", pedido{cuerpo: `{"cosa":{"descripcion":"","cantidad":0},"origen":{"nombre":"A","direccion":"x"},"destino":{"nombre":"B","direccion":"y"}}`, clave: "clave-invalida"}, http.StatusUnprocessableEntity, "DATOS_INVALIDOS"},
		{"JSON mal formado", pedido{cuerpo: `{"cosa":`, clave: "clave-mal-formado"}, http.StatusBadRequest, "JSON_MAL_FORMADO"},
		{"sin Idempotency-Key", pedido{cuerpo: cuerpoMinimoContrato}, http.StatusUnprocessableEntity, "DATOS_INVALIDOS"},
		{"Idempotency-Key corta", pedido{cuerpo: cuerpoMinimoContrato, clave: "corta"}, http.StatusUnprocessableEntity, "DATOS_INVALIDOS"},
		{"horario en el pasado", pedido{cuerpo: strings.Replace(cuerpoMinimoContrato, `{"cosa"`, `{"hora_llegada_solicitada":"2026-10-14T10:00:00-03:00","cosa"`, 1), clave: "clave-pasado-1"}, http.StatusUnprocessableEntity, "HORARIO_EN_EL_PASADO"},
		{"sin organización", pedido{cuerpo: cuerpoMinimoContrato, clave: "clave-sin-org-1", sinOrganizacion: true}, http.StatusForbidden, "SIN_PERMISO"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			e.t = t
			c.p.metodo, c.p.ruta = http.MethodPost, "/traslados"
			e.esperar(e.ejecutar(c.p), c.estado, c.codigo)
		})
	}

	t.Run("MongoDB caído", func(t *testing.T) {
		e.t = t
		e.repo.Error = errors.New("mongo caído")
		defer func() { e.repo.Error = nil }()
		e.esperar(e.ejecutar(pedido{metodo: http.MethodPost, ruta: "/traslados", cuerpo: cuerpoMinimoContrato, clave: "clave-mongo-1", valido: true}),
			http.StatusServiceUnavailable, "SERVICIO_NO_DISPONIBLE")
	})

	// Los datos inválidos traen el detalle por campo.
	e.t = t
	r = e.ejecutar(pedido{metodo: http.MethodPost, ruta: "/traslados", cuerpo: `{}`, clave: "clave-vacia-1"})
	var p struct {
		Errores []struct{ Campo string } `json:"errores"`
	}
	_ = json.Unmarshal(r.cuerpo, &p)
	if len(p.Errores) < 6 {
		t.Fatalf("se esperaba el detalle de cada campo obligatorio: %s", r.cuerpo)
	}
}

func TestContratoListarTraslados(t *testing.T) {
	e := nuevoEntorno(t)
	e.crear("clave-lista-1")
	e.crear("clave-lista-2")

	r := e.ejecutar(pedido{metodo: http.MethodGet, ruta: "/traslados?tamanio=1&pagina=2&estado=RECIBIDA&actualizado_desde=2026-10-15T00:00:00-03:00", valido: true})
	e.esperar(r, http.StatusOK, "")
	var pagina struct {
		Resultados []map[string]any
		Pagina     int
		Tamanio    int
		Total      int
	}
	_ = json.Unmarshal(r.cuerpo, &pagina)
	if pagina.Total != 2 || len(pagina.Resultados) != 1 || pagina.Pagina != 2 || pagina.Tamanio != 1 {
		t.Fatalf("página incorrecta: %s", r.cuerpo)
	}

	r = e.ejecutar(pedido{metodo: http.MethodGet, ruta: "/traslados", valido: true})
	e.esperar(r, http.StatusOK, "")
	_ = json.Unmarshal(r.cuerpo, &pagina)
	if pagina.Pagina != 1 || pagina.Tamanio != 20 {
		t.Fatalf("valores por defecto incorrectos: %s", r.cuerpo)
	}

	// Sin resultados: lista vacía, no null.
	r = e.ejecutar(pedido{metodo: http.MethodGet, ruta: "/traslados?estado=CANCELADA", valido: true})
	e.esperar(r, http.StatusOK, "")
	if !strings.Contains(string(r.cuerpo), `"resultados":[]`) {
		t.Fatalf("se esperaba una lista vacía: %s", r.cuerpo)
	}

	for _, consulta := range []string{"tamanio=0", "tamanio=101", "pagina=0", "pagina=uno", "estado=PERDIDA", "actualizado_desde=ayer"} {
		t.Run(consulta, func(t *testing.T) {
			e.t = t
			e.esperar(e.ejecutar(pedido{metodo: http.MethodGet, ruta: "/traslados?" + consulta}), http.StatusBadRequest, "JSON_MAL_FORMADO")
		})
	}
	t.Run("MongoDB caído", func(t *testing.T) {
		e.t = t
		e.repo.Error = errors.New("mongo caído")
		defer func() { e.repo.Error = nil }()
		e.esperar(e.ejecutar(pedido{metodo: http.MethodGet, ruta: "/traslados", valido: true}), http.StatusServiceUnavailable, "SERVICIO_NO_DISPONIBLE")
	})
}

func TestContratoObtenerTraslado(t *testing.T) {
	e := nuevoEntorno(t)
	id := e.crear("clave-obtener-1")
	e.esperar(e.ejecutar(pedido{metodo: http.MethodGet, ruta: "/traslados/" + id, valido: true}), http.StatusOK, "")

	// Con todos los campos (incluidos los internos que el contrato no informa).
	if _, _, err := e.repo.AplicarVista(context.Background(), id, 4, vistaCompleta(id, 4), ahoraContrato); err != nil {
		t.Fatal(err)
	}
	r := e.ejecutar(pedido{metodo: http.MethodGet, ruta: "/traslados/" + id, valido: true})
	e.esperar(r, http.StatusOK, "")
	cuerpo := string(r.cuerpo)
	if strings.Contains(cuerpo, "alerta_climatica") || strings.Contains(cuerpo, `"version"`) {
		t.Fatalf("los campos internos no se responden: %s", cuerpo)
	}
	if !strings.Contains(cuerpo, `"hora_llegada_comprometida":"2026-10-16T17:00:00-03:00"`) {
		t.Fatalf("hora del traslado sin zona -03:00: %s", cuerpo)
	}

	e.esperar(e.ejecutar(pedido{metodo: http.MethodGet, ruta: "/traslados/sol-no-existe", valido: true}), http.StatusNotFound, "NO_ENCONTRADA")
	t.Run("MongoDB caído", func(t *testing.T) {
		e.t = t
		e.repo.Error = errors.New("mongo caído")
		defer func() { e.repo.Error = nil }()
		e.esperar(e.ejecutar(pedido{metodo: http.MethodGet, ruta: "/traslados/" + id, valido: true}), http.StatusServiceUnavailable, "SERVICIO_NO_DISPONIBLE")
	})
}

func TestContratoResponderContrapropuesta(t *testing.T) {
	e := nuevoEntorno(t)
	id := e.crear("clave-contra-1")
	ruta := "/traslados/" + id + "/respuesta-contrapropuesta"

	casos := []struct {
		nombre     string
		cuerpo     string
		clave      string
		errCliente error
		valido     bool
		estado     int
		codigo     string
		retryAfter string
	}{
		{"Operaciones todavía no la recibió", `{"acepta":true}`, "clave-resp-0001", clients.ErrNoEncontrada, true, http.StatusServiceUnavailable, "SERVICIO_NO_DISPONIBLE", "5"},
		{"Operaciones caída", `{"acepta":true}`, "clave-resp-0002", clients.ErrNoDisponible, true, http.StatusServiceUnavailable, "SERVICIO_NO_DISPONIBLE", "30"},
		{"contrapropuesta vencida", `{"acepta":true}`, "clave-resp-0003", clients.ErrConflicto{Codigo: "CONTRAPROPUESTA_VENCIDA"}, true, http.StatusConflict, "CONTRAPROPUESTA_VENCIDA", ""},
		{"Operaciones rechaza los datos", `{"acepta":true}`, "clave-resp-0004", clients.ErrDatosInvalidos, true, http.StatusUnprocessableEntity, "DATOS_INVALIDOS", ""},
		{"sin acepta", `{}`, "clave-resp-0005", nil, false, http.StatusUnprocessableEntity, "DATOS_INVALIDOS", ""},
		{"JSON mal formado", `{"acepta":`, "clave-resp-0006", nil, false, http.StatusBadRequest, "JSON_MAL_FORMADO", ""},
		{"sin Idempotency-Key", `{"acepta":true}`, "", nil, false, http.StatusUnprocessableEntity, "DATOS_INVALIDOS", ""},
		{"aceptada", `{"acepta":true}`, "clave-resp-0007", nil, true, http.StatusOK, "", ""},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			e.t = t
			e.cliente.Error, e.cliente.Vista = c.errCliente, vistaCompleta(id, 5)
			r := e.ejecutar(pedido{metodo: http.MethodPost, ruta: ruta, cuerpo: c.cuerpo, clave: c.clave, valido: c.valido})
			e.esperar(r, c.estado, c.codigo)
			if got := r.header.Get("Retry-After"); got != c.retryAfter {
				t.Fatalf("Retry-After: se esperaba %q, se obtuvo %q", c.retryAfter, got)
			}
		})
	}
	e.t = t
	e.cliente.Error = nil
	e.esperar(e.ejecutar(pedido{metodo: http.MethodPost, ruta: "/traslados/sol-no-existe/respuesta-contrapropuesta", cuerpo: `{"acepta":false}`, clave: "clave-resp-0008", valido: true}),
		http.StatusNotFound, "NO_ENCONTRADA")
}

func TestContratoCancelarTraslado(t *testing.T) {
	e := nuevoEntorno(t)
	id := e.crear("clave-cancelar-1")
	ruta := "/traslados/" + id + "/cancelacion"
	motivo := `{"motivo":"La mercadería se despachó por otro medio."}`

	casos := []struct {
		nombre     string
		cuerpo     string
		errCliente error
		valido     bool
		estado     int
		codigo     string
	}{
		{"traslado en curso", motivo, clients.ErrConflicto{Codigo: "ESTADO_NO_PERMITE_OPERACION"}, true, http.StatusConflict, "ESTADO_NO_PERMITE_OPERACION"},
		{"Operaciones caída", motivo, clients.ErrNoDisponible, true, http.StatusServiceUnavailable, "SERVICIO_NO_DISPONIBLE"},
		{"motivo vacío", `{"motivo":"  "}`, nil, false, http.StatusUnprocessableEntity, "DATOS_INVALIDOS"},
		{"motivo largo", `{"motivo":"` + strings.Repeat("m", 501) + `"}`, nil, false, http.StatusUnprocessableEntity, "DATOS_INVALIDOS"},
		{"JSON mal formado", `motivo`, nil, false, http.StatusBadRequest, "JSON_MAL_FORMADO"},
		{"cancelación registrada", motivo, nil, true, http.StatusOK, ""},
	}
	for i, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			e.t = t
			e.cliente.Error, e.cliente.Vista = c.errCliente, vistaCompleta(id, int64(10+i))
			e.esperar(e.ejecutar(pedido{metodo: http.MethodPost, ruta: ruta, cuerpo: c.cuerpo, clave: "clave-canc-000" + string(rune('0'+i)), valido: c.valido}),
				c.estado, c.codigo)
		})
	}
	e.t = t
	e.esperar(e.ejecutar(pedido{metodo: http.MethodPost, ruta: "/traslados/sol-no-existe/cancelacion", cuerpo: motivo, clave: "clave-canc-0100", valido: true}),
		http.StatusNotFound, "NO_ENCONTRADA")
}
