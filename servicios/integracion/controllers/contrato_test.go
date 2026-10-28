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

	"github.com/francoroblesss/Rumbo/servicios/comun/problema"
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
  "cosa": {"descripcion": "Cajas de mercadería seca", "cantidad": 20, "peso_kg": 350, "volumen_m3": 2.5},
  "origen": {"nombre": "Sucursal Centro", "direccion": "Av. Colón 1234, Córdoba", "contacto": "Depósito — 351 555-0101"},
  "destino": {"nombre": "Sucursal Cerro", "direccion": "Av. Rafael Núñez 4500, Córdoba"},
  "franja_inicio": "2026-10-16T10:00:00-03:00",
  "observaciones": "Entrar por el portón de carga."
}`

const cuerpoMinimoContrato = `{"cosa":{"descripcion":"Cajas","cantidad":3},"origen":{"nombre":"A","direccion":"x"},` +
	`"destino":{"nombre":"B","direccion":"y"},"franja_inicio":"2026-10-16T10:00:00-03:00"}`

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
	e.esperar(r, http.StatusCreated, "")
	var v struct{ ID string }
	_ = json.Unmarshal(r.cuerpo, &v)
	return v.ID
}

func hora(s string) time.Time {
	v, _ := time.Parse(time.RFC3339, s)
	return v
}

// vistaCompleta es una vista como la que manda Operaciones, con campos internos incluidos.
func vistaCompleta(id string, version int64) models.VistaSolicitud {
	observado := hora("2026-10-15T12:00:00Z")
	demora := 15
	motivo := "La mercadería se despachó por otro medio."
	return models.VistaSolicitud{
		ID: id, Estado: "ACEPTADA", Version: version,
		Cosa:    models.Cosa{Descripcion: "Cajas", Cantidad: 3},
		Origen:  models.Ubicacion{Nombre: "A", Direccion: "x"},
		Destino: models.Ubicacion{Nombre: "B", Direccion: "y"},
		Traslado: &models.Traslado{ID: "tra-1", Estado: models.EstadoCancelado, HoraLlegadaComprometida: hora("2026-10-16T15:00:00Z"),
			Franja:          &models.Franja{Inicio: hora("2026-10-16T13:00:00Z"), Fin: hora("2026-10-16T15:00:00Z")},
			DemoraMinutos:   &demora,
			AlertaClimatica: &models.AlertaClimatica{Estado: "NO_OPERABLE", Motivos: []string{"HIGH_WIND"}, ObservadoEn: &observado}},
		Cancelacion: &models.Cancelacion{Estado: "APROBADA", Motivo: &motivo, SolicitadaEn: hora("2026-10-16T10:30:00Z"), Tardia: true},
		CreadoEn:    ahoraContrato, ActualizadoEn: ahoraContrato.Add(time.Hour),
	}
}

func TestContratoDisponibilidad(t *testing.T) {
	e := nuevoEntorno(t)
	e.cliente.Disponible = models.Disponibilidad{Fecha: "2026-10-16", Franjas: []models.FranjaDisponible{
		{Inicio: hora("2026-10-16T11:00:00Z"), Fin: hora("2026-10-16T13:00:00Z"), Disponible: true, TrasladosDisponibles: 4, KgDisponibles: 2000},
		{Inicio: hora("2026-10-16T13:00:00Z"), Fin: hora("2026-10-16T15:00:00Z"), Disponible: false, TrasladosDisponibles: 0, KgDisponibles: 600},
	}}
	r := e.ejecutar(pedido{metodo: http.MethodGet, ruta: "/disponibilidad?fecha=2026-10-16&peso_kg=350", valido: true})
	e.esperar(r, http.StatusOK, "")
	if !strings.Contains(string(r.cuerpo), `"inicio":"2026-10-16T08:00:00-03:00"`) {
		t.Fatalf("las franjas se responden en hora de Argentina: %s", r.cuerpo)
	}

	e.cliente.Disponible = models.Disponibilidad{Fecha: "2026-10-16"}
	r = e.ejecutar(pedido{metodo: http.MethodGet, ruta: "/disponibilidad?fecha=2026-10-16", valido: true})
	e.esperar(r, http.StatusOK, "")
	if !strings.Contains(string(r.cuerpo), `"franjas":[]`) {
		t.Fatalf("sin franjas: lista vacía, no null: %s", r.cuerpo)
	}

	for _, consulta := range []string{"", "?fecha=16/10/2026", "?fecha=2026-10-16&peso_kg=0", "?fecha=2026-10-16&peso_kg=mucho"} {
		t.Run("consulta "+consulta, func(t *testing.T) {
			e.t = t
			e.esperar(e.ejecutar(pedido{metodo: http.MethodGet, ruta: "/disponibilidad" + consulta}), http.StatusBadRequest, "JSON_MAL_FORMADO")
		})
	}
	t.Run("fecha fuera del período", func(t *testing.T) {
		e.t = t
		e.cliente.Error = clients.ErrRechazo{Estado: 422, Codigo: "DATOS_INVALIDOS", Titulo: "Datos inválidos"}
		defer func() { e.cliente.Error = nil }()
		e.esperar(e.ejecutar(pedido{metodo: http.MethodGet, ruta: "/disponibilidad?fecha=2030-01-01", valido: true}),
			http.StatusUnprocessableEntity, "DATOS_INVALIDOS")
	})
	t.Run("Operaciones caída", func(t *testing.T) {
		e.t = t
		e.cliente.Error = clients.ErrNoDisponible
		defer func() { e.cliente.Error = nil }()
		r := e.ejecutar(pedido{metodo: http.MethodGet, ruta: "/disponibilidad?fecha=2026-10-16", valido: true})
		e.esperar(r, http.StatusServiceUnavailable, "SERVICIO_NO_DISPONIBLE")
		if r.header.Get("Retry-After") != "30" {
			t.Fatalf("Retry-After = %q", r.header.Get("Retry-After"))
		}
	})
	t.Run("sin organización", func(t *testing.T) {
		e.t = t
		e.esperar(e.ejecutar(pedido{metodo: http.MethodGet, ruta: "/disponibilidad?fecha=2026-10-16", sinOrganizacion: true}),
			http.StatusForbidden, "SIN_PERMISO")
	})
}

func TestContratoReservarTraslado(t *testing.T) {
	e := nuevoEntorno(t)
	r := e.ejecutar(pedido{metodo: http.MethodPost, ruta: "/traslados", cuerpo: cuerpoCompleto, clave: "clave-completa-1", valido: true})
	e.esperar(r, http.StatusCreated, "")
	var creada map[string]any
	_ = json.Unmarshal(r.cuerpo, &creada)
	if creada["estado"] != "PROGRAMADO" || r.header.Get("Location") != "/traslados/"+creada["id"].(string) {
		t.Fatalf("respuesta 201 incorrecta: %s (Location %s)", r.cuerpo, r.header.Get("Location"))
	}
	franja, _ := creada["franja"].(map[string]any)
	if franja["inicio"] != "2026-10-16T10:00:00-03:00" || franja["fin"] != "2026-10-16T12:00:00-03:00" ||
		!strings.HasSuffix(creada["creado_en"].(string), "-03:00") {
		t.Fatalf("las horas se responden con zona -03:00: %s", r.cuerpo)
	}
	if _, ok := creada["version"]; ok {
		t.Fatal("version es interno: no se responde")
	}

	// Reintento idempotente: 201 y la misma reserva.
	r = e.ejecutar(pedido{metodo: http.MethodPost, ruta: "/traslados", cuerpo: cuerpoCompleto, clave: "clave-completa-1", valido: true})
	e.esperar(r, http.StatusCreated, "")
	var repetida map[string]any
	_ = json.Unmarshal(r.cuerpo, &repetida)
	if repetida["id"] != creada["id"] {
		t.Fatalf("el reintento tiene que devolver la misma reserva: %v vs %v", repetida["id"], creada["id"])
	}

	franjaInvalida := clients.ErrRechazo{Estado: 422, Codigo: "DATOS_INVALIDOS", Titulo: "Datos inválidos",
		Errores: []problema.ErrorCampo{{Campo: "franja_inicio", Mensaje: "no es el inicio de una franja reservable"}}}
	casos := []struct {
		nombre     string
		p          pedido
		errCliente error
		estado     int
		codigo     string
	}{
		{"mínimo", pedido{cuerpo: cuerpoMinimoContrato, clave: "clave-minima-1", valido: true}, nil, http.StatusCreated, ""},
		{"misma clave, otro cuerpo", pedido{cuerpo: cuerpoMinimoContrato, clave: "clave-completa-1"}, nil, http.StatusConflict, "CLAVE_IDEMPOTENCIA_REUTILIZADA"},
		{"sin disponibilidad", pedido{cuerpo: cuerpoMinimoContrato, clave: "clave-llena-1", valido: true},
			clients.ErrRechazo{Estado: 409, Codigo: "SIN_DISPONIBILIDAD", Titulo: "Sin disponibilidad", Detalle: "La franja ya no tiene lugar."},
			http.StatusConflict, "SIN_DISPONIBILIDAD"},
		{"anticipación insuficiente", pedido{cuerpo: cuerpoMinimoContrato, clave: "clave-pronto-1", valido: true},
			clients.ErrRechazo{Estado: 422, Codigo: "ANTICIPACION_INSUFICIENTE", Titulo: "Anticipación insuficiente"},
			http.StatusUnprocessableEntity, "ANTICIPACION_INSUFICIENTE"},
		{"franja inexistente", pedido{cuerpo: cuerpoMinimoContrato, clave: "clave-franja-1", valido: true}, franjaInvalida,
			http.StatusUnprocessableEntity, "DATOS_INVALIDOS"},
		{"Operaciones caída", pedido{cuerpo: cuerpoMinimoContrato, clave: "clave-caida-1", valido: true}, clients.ErrNoDisponible,
			http.StatusServiceUnavailable, "SERVICIO_NO_DISPONIBLE"},
		{"datos inválidos", pedido{cuerpo: `{"cosa":{"descripcion":"","cantidad":0},"origen":{"nombre":"A","direccion":"x"},"destino":{"nombre":"B","direccion":"y"}}`,
			clave: "clave-invalida"}, nil, http.StatusUnprocessableEntity, "DATOS_INVALIDOS"},
		{"JSON mal formado", pedido{cuerpo: `{"cosa":`, clave: "clave-mal-formado"}, nil, http.StatusBadRequest, "JSON_MAL_FORMADO"},
		{"sin Idempotency-Key", pedido{cuerpo: cuerpoMinimoContrato}, nil, http.StatusUnprocessableEntity, "DATOS_INVALIDOS"},
		{"Idempotency-Key corta", pedido{cuerpo: cuerpoMinimoContrato, clave: "corta"}, nil, http.StatusUnprocessableEntity, "DATOS_INVALIDOS"},
		{"sin organización", pedido{cuerpo: cuerpoMinimoContrato, clave: "clave-sin-org-1", sinOrganizacion: true}, nil, http.StatusForbidden, "SIN_PERMISO"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			e.t = t
			e.cliente.Error = c.errCliente
			defer func() { e.cliente.Error = nil }()
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
	if len(p.Errores) < 7 {
		t.Fatalf("se esperaba el detalle de cada campo obligatorio: %s", r.cuerpo)
	}
}

func TestContratoListarTraslados(t *testing.T) {
	e := nuevoEntorno(t)
	e.crear("clave-lista-1")
	e.crear("clave-lista-2")

	r := e.ejecutar(pedido{metodo: http.MethodGet, ruta: "/traslados?tamanio=1&pagina=2&estado=PROGRAMADO&actualizado_desde=2026-10-15T00:00:00-03:00", valido: true})
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
	r = e.ejecutar(pedido{metodo: http.MethodGet, ruta: "/traslados?estado=CANCELADO", valido: true})
	e.esperar(r, http.StatusOK, "")
	if !strings.Contains(string(r.cuerpo), `"resultados":[]`) {
		t.Fatalf("se esperaba una lista vacía: %s", r.cuerpo)
	}

	for _, consulta := range []string{"tamanio=0", "tamanio=101", "pagina=0", "pagina=uno", "estado=RECIBIDA", "actualizado_desde=ayer"} {
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
	if strings.Contains(cuerpo, "alerta_climatica") || strings.Contains(cuerpo, `"version"`) || strings.Contains(cuerpo, "ACEPTADA") {
		t.Fatalf("los campos internos no se responden: %s", cuerpo)
	}
	if !strings.Contains(cuerpo, `"fin":"2026-10-16T12:00:00-03:00"`) || !strings.Contains(cuerpo, `"tardia":true`) {
		t.Fatalf("franja sin zona -03:00 o cancelación incompleta: %s", cuerpo)
	}

	e.esperar(e.ejecutar(pedido{metodo: http.MethodGet, ruta: "/traslados/sol-no-existe", valido: true}), http.StatusNotFound, "NO_ENCONTRADA")
	t.Run("MongoDB caído", func(t *testing.T) {
		e.t = t
		e.repo.Error = errors.New("mongo caído")
		defer func() { e.repo.Error = nil }()
		e.esperar(e.ejecutar(pedido{metodo: http.MethodGet, ruta: "/traslados/" + id, valido: true}), http.StatusServiceUnavailable, "SERVICIO_NO_DISPONIBLE")
	})
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
		{"traslado en curso", motivo, clients.ErrRechazo{Estado: 409, Codigo: "ESTADO_NO_PERMITE_OPERACION", Titulo: "El estado actual no permite la operación"},
			true, http.StatusConflict, "ESTADO_NO_PERMITE_OPERACION"},
		{"Operaciones caída", motivo, clients.ErrNoDisponible, true, http.StatusServiceUnavailable, "SERVICIO_NO_DISPONIBLE"},
		{"motivo vacío", `{"motivo":"  "}`, nil, false, http.StatusUnprocessableEntity, "DATOS_INVALIDOS"},
		{"motivo largo", `{"motivo":"` + strings.Repeat("m", 501) + `"}`, nil, false, http.StatusUnprocessableEntity, "DATOS_INVALIDOS"},
		{"JSON mal formado", `motivo`, nil, false, http.StatusBadRequest, "JSON_MAL_FORMADO"},
		{"cancelado", motivo, nil, true, http.StatusOK, ""},
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
	e.cliente.Error = nil
	e.esperar(e.ejecutar(pedido{metodo: http.MethodPost, ruta: "/traslados/sol-no-existe/cancelacion", cuerpo: motivo, clave: "clave-canc-0100", valido: true}),
		http.StatusNotFound, "NO_ENCONTRADA")
}
