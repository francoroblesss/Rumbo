package services

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/francoroblesss/Rumbo/servicios/comun/mensajeria"
	"github.com/francoroblesss/Rumbo/servicios/comun/problema"
	"github.com/francoroblesss/Rumbo/servicios/integracion/clients"
	"github.com/francoroblesss/Rumbo/servicios/integracion/models"
	"github.com/francoroblesss/Rumbo/servicios/integracion/repositories"
)

const org = "org-almacenamiento"

var ahoraFija = time.Date(2026, 10, 15, 11, 30, 0, 0, time.UTC)

func nuevoServicio() (*SolicitudesService, *repositories.SolicitudesMock, *clients.OperacionesMock) {
	repo := repositories.NewSolicitudesMock()
	cliente := &clients.OperacionesMock{}
	return NewSolicitudesService(repo, cliente, func() time.Time { return ahoraFija }), repo, cliente
}

const cuerpoMinimo = `{"cosa":{"descripcion":"Cajas","cantidad":3},"origen":{"nombre":"A","direccion":"x"},` +
	`"destino":{"nombre":"B","direccion":"y"},"franja_inicio":"2026-10-16T10:00:00-03:00"}`

func errorDe(t *testing.T, err error) *ErrorServicio {
	t.Helper()
	var e *ErrorServicio
	if !errors.As(err, &e) {
		t.Fatalf("se esperaba un ErrorServicio, se obtuvo %v", err)
	}
	return e
}

func camposConError(e *ErrorServicio) map[string]bool {
	campos := map[string]bool{}
	for _, ec := range e.Errores {
		campos[ec.Campo] = true
	}
	return campos
}

func TestCrearValidoReservaYGuardaLaCopia(t *testing.T) {
	s, repo, cliente := nuevoServicio()
	vista, estado, err := s.Crear(context.Background(), org, "clave-0001", []byte(cuerpoMinimo))
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if estado != http.StatusCreated || vista.Traslado == nil || vista.Traslado.Estado != models.EstadoProgramado {
		t.Fatalf("respuesta inesperada: %d %+v", estado, vista)
	}
	l := cliente.Llamadas[0]
	if l.Operacion != "reserva" || l.Org != org || l.ClaveIdem != "clave-0001" ||
		!l.Pedido.FranjaInicio.Equal(time.Date(2026, 10, 16, 13, 0, 0, 0, time.UTC)) {
		t.Fatalf("llamada a Operaciones incorrecta: %+v", l)
	}
	guardada, err := repo.BuscarPorID(context.Background(), org, vista.ID)
	if err != nil || guardada.Version != 1 || guardada.Idempotencia.HashCuerpo == "" || guardada.Idempotencia.Clave != "clave-0001" {
		t.Fatalf("copia mal guardada: %+v %v", guardada, err)
	}
}

func TestCrearJSONMalFormado(t *testing.T) {
	s, _, _ := nuevoServicio()
	for _, cuerpo := range []string{`{`, ``, `{"cosa":{"cantidad":"tres"}}`, `{"franja_inicio":"mañana"}`} {
		_, _, err := s.Crear(context.Background(), org, "clave-0001", []byte(cuerpo))
		if e := errorDe(t, err); e.Estado != http.StatusBadRequest || e.Codigo != "JSON_MAL_FORMADO" {
			t.Fatalf("cuerpo %q: se esperaba 400 JSON_MAL_FORMADO, se obtuvo %+v", cuerpo, e)
		}
	}
}

func TestCrearJuntaTodosLosErroresPorCampo(t *testing.T) {
	s, _, cliente := nuevoServicio()
	largo := func(n int) string { return strings.Repeat("a", n) }
	cuerpo, _ := json.Marshal(map[string]any{
		"referencia_externa": largo(65),
		"cosa":               map[string]any{"descripcion": largo(251), "cantidad": 0, "peso_kg": 0, "volumen_m3": -1},
		"origen":             map[string]any{"nombre": largo(121), "direccion": largo(251), "contacto": largo(121)},
		"destino":            map[string]any{"nombre": " ", "direccion": ""},
		"observaciones":      largo(501),
	})
	_, _, err := s.Crear(context.Background(), org, "clave-0001", cuerpo)
	e := errorDe(t, err)
	if e.Estado != http.StatusUnprocessableEntity || e.Codigo != "DATOS_INVALIDOS" {
		t.Fatalf("se esperaba 422 DATOS_INVALIDOS, se obtuvo %+v", e)
	}
	campos := camposConError(e)
	for _, campo := range []string{"referencia_externa", "cosa.descripcion", "cosa.cantidad", "cosa.peso_kg", "cosa.volumen_m3",
		"origen.nombre", "origen.direccion", "origen.contacto", "destino.nombre", "destino.direccion",
		"franja_inicio", "observaciones"} {
		if !campos[campo] {
			t.Errorf("falta el error del campo %s (errores: %+v)", campo, e.Errores)
		}
	}
	if len(cliente.Llamadas) != 0 {
		t.Fatal("un pedido inválido no se tiene que mandar a Operaciones")
	}
}

func TestCrearAceptaLosLimitesExactos(t *testing.T) {
	s, _, _ := nuevoServicio()
	cuerpo, _ := json.Marshal(map[string]any{
		"referencia_externa": strings.Repeat("r", 64),
		"cosa":               map[string]any{"descripcion": strings.Repeat("ñ", 250), "cantidad": 1, "peso_kg": 0.1, "volumen_m3": 0.1},
		"origen":             map[string]any{"nombre": strings.Repeat("n", 120), "direccion": strings.Repeat("d", 250), "contacto": strings.Repeat("c", 120)},
		"destino":            map[string]any{"nombre": "B", "direccion": "y"},
		"franja_inicio":      "2026-10-16T10:00:00-03:00",
		"observaciones":      strings.Repeat("o", 500),
	})
	if _, _, err := s.Crear(context.Background(), org, "clave-0001", cuerpo); err != nil {
		t.Fatalf("los valores en el límite son válidos: %v", err)
	}
}

func TestCrearReenviaElRechazoDeOperaciones(t *testing.T) {
	casos := []struct {
		err       error
		estado    int
		codigo    string
		reintento int
	}{
		{clients.ErrRechazo{Estado: 409, Codigo: "SIN_DISPONIBILIDAD", Titulo: "Sin disponibilidad", Detalle: "La franja ya no tiene lugar."},
			http.StatusConflict, "SIN_DISPONIBILIDAD", 0},
		{clients.ErrRechazo{Estado: 422, Codigo: "DATOS_INVALIDOS", Errores: []problema.ErrorCampo{{Campo: "franja_inicio", Mensaje: "no"}}},
			http.StatusUnprocessableEntity, "DATOS_INVALIDOS", 0},
		{clients.ErrNoDisponible, http.StatusServiceUnavailable, "SERVICIO_NO_DISPONIBLE", ReintentarNoDisponible},
	}
	for _, c := range casos {
		s, repo, cliente := nuevoServicio()
		cliente.Error = c.err
		_, _, err := s.Crear(context.Background(), org, "clave-0001", []byte(cuerpoMinimo))
		e := errorDe(t, err)
		if e.Estado != c.estado || e.Codigo != c.codigo || e.ReintentoS != c.reintento || e.Titulo == "" {
			t.Fatalf("%v: se esperaba %d %s, se obtuvo %+v", c.err, c.estado, c.codigo, e)
		}
		if _, err := repo.BuscarPorClave(context.Background(), org, "clave-0001"); !errors.Is(err, repositories.ErrNoEncontrada) {
			t.Fatal("un rechazo no se tiene que guardar")
		}
	}
}

func TestCrearReintentoIdempotenteDevuelveLaMismaRespuesta(t *testing.T) {
	s, _, cliente := nuevoServicio()
	primera, _, err := s.Crear(context.Background(), org, "clave-0001", []byte(cuerpoMinimo))
	if err != nil {
		t.Fatal(err)
	}
	// Mismo pedido con otro orden de campos y espacios: el cuerpo normalizado es el mismo. Y
	// aunque Operaciones esté caída, el reintento se responde con la copia guardada.
	cliente.Error = clients.ErrNoDisponible
	reordenado := `{ "franja_inicio":"2026-10-16T10:00:00-03:00", "destino":{"direccion":"y","nombre":"B"},
		"origen":{"nombre":"A","direccion":"x"}, "cosa":{"cantidad":3,"descripcion":"Cajas"} }`
	segunda, estado, err := s.Crear(context.Background(), org, "clave-0001", []byte(reordenado))
	if err != nil || estado != http.StatusCreated {
		t.Fatalf("el reintento idempotente tiene que dar 201: %d %v", estado, err)
	}
	if segunda.ID != primera.ID || len(cliente.Llamadas) != 1 {
		t.Fatalf("el reintento tiene que devolver la misma reserva sin llamar a Operaciones: %s vs %s", segunda.ID, primera.ID)
	}
}

func TestCrearMismaClaveOtroCuerpoEsConflicto(t *testing.T) {
	s, _, _ := nuevoServicio()
	if _, _, err := s.Crear(context.Background(), org, "clave-0001", []byte(cuerpoMinimo)); err != nil {
		t.Fatal(err)
	}
	otro := strings.Replace(cuerpoMinimo, `"cantidad":3`, `"cantidad":4`, 1)
	_, _, err := s.Crear(context.Background(), org, "clave-0001", []byte(otro))
	if e := errorDe(t, err); e.Estado != http.StatusConflict || e.Codigo != "CLAVE_IDEMPOTENCIA_REUTILIZADA" {
		t.Fatalf("se esperaba 409 CLAVE_IDEMPOTENCIA_REUTILIZADA, se obtuvo %+v", e)
	}
	// La misma clave en otra organización es otra intención.
	if _, _, err := s.Crear(context.Background(), "org-otra", "clave-0001", []byte(otro)); err != nil {
		t.Fatalf("la clave es por organización: %v", err)
	}
}

func TestCrearConMongoCaidoEs503(t *testing.T) {
	s, repo, _ := nuevoServicio()
	repo.Error = errors.New("mongo caído")
	_, _, err := s.Crear(context.Background(), org, "clave-0001", []byte(cuerpoMinimo))
	if e := errorDe(t, err); e.Estado != http.StatusServiceUnavailable || e.Codigo != "SERVICIO_NO_DISPONIBLE" {
		t.Fatalf("se esperaba 503, se obtuvo %+v", e)
	}
}

func TestDisponibilidad(t *testing.T) {
	s, _, cliente := nuevoServicio()
	cliente.Disponible = models.Disponibilidad{Fecha: "2026-10-16", Franjas: []models.FranjaDisponible{{Disponible: true}}}
	d, err := s.Disponibilidad(context.Background(), "2026-10-16", nil)
	if err != nil || len(d.Franjas) != 1 || cliente.Llamadas[0].Fecha != "2026-10-16" {
		t.Fatalf("d = %+v, err = %v", d, err)
	}
	cliente.Error = clients.ErrNoDisponible
	if _, err := s.Disponibilidad(context.Background(), "2026-10-16", nil); errorDe(t, err).Estado != http.StatusServiceUnavailable {
		t.Fatalf("con Operaciones caída: %v", err)
	}
}

func TestObtenerYNoEncontrada(t *testing.T) {
	s, _, _ := nuevoServicio()
	creada, _, _ := s.Crear(context.Background(), org, "clave-0001", []byte(cuerpoMinimo))
	vista, err := s.Obtener(context.Background(), org, creada.ID)
	if err != nil || vista.ID != creada.ID {
		t.Fatalf("no se obtuvo el traslado: %v", err)
	}
	_, err = s.Obtener(context.Background(), "org-otra", creada.ID)
	if e := errorDe(t, err); e.Estado != http.StatusNotFound || e.Codigo != "NO_ENCONTRADA" {
		t.Fatalf("otra organización no tiene que ver el traslado: %+v", e)
	}
}

func TestListarDevuelvePagina(t *testing.T) {
	s, _, _ := nuevoServicio()
	for _, clave := range []string{"clave-0001", "clave-0002", "clave-0003"} {
		if _, _, err := s.Crear(context.Background(), org, clave, []byte(cuerpoMinimo)); err != nil {
			t.Fatal(err)
		}
	}
	pagina, err := s.Listar(context.Background(), repositories.FiltroListado{OrganizacionID: org, Pagina: 2, Tamanio: 2})
	if err != nil || pagina.Total != 3 || len(pagina.Resultados) != 1 || pagina.Pagina != 2 || pagina.Tamanio != 2 {
		t.Fatalf("página incorrecta: %+v %v", pagina, err)
	}
	filtrada, _ := s.Listar(context.Background(), repositories.FiltroListado{OrganizacionID: org, Estado: models.EstadoCompletado, Pagina: 1, Tamanio: 20})
	if filtrada.Total != 0 {
		t.Fatalf("el filtro es por el estado del traslado: %+v", filtrada)
	}
	vacia, _ := s.Listar(context.Background(), repositories.FiltroListado{OrganizacionID: "org-otra", Pagina: 1, Tamanio: 20})
	if vacia.Resultados == nil || len(vacia.Resultados) != 0 {
		t.Fatal("una página vacía tiene que traer una lista vacía, no null")
	}
}

func crearReserva(t *testing.T, s *SolicitudesService) models.VistaSolicitud {
	t.Helper()
	v, _, err := s.Crear(context.Background(), org, "clave-0001", []byte(cuerpoMinimo))
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestCancelarAplicaLaVista(t *testing.T) {
	s, repo, cliente := nuevoServicio()
	creada := crearReserva(t, s)
	cliente.Vista = models.VistaSolicitud{ID: creada.ID, Estado: "ACEPTADA", Version: 2,
		Traslado: &models.Traslado{Estado: models.EstadoCancelado}, Cancelacion: &models.Cancelacion{Estado: "APROBADA", Tardia: true}}
	vista, err := s.Cancelar(context.Background(), org, "clave-canc-1", creada.ID, "ya no hace falta")
	if err != nil || vista.Traslado.Estado != models.EstadoCancelado || !vista.Cancelacion.Tardia {
		t.Fatalf("respuesta inesperada: %+v %v", vista, err)
	}
	if l := cliente.Llamadas[1]; l.Org != org || l.ClaveIdem != "clave-canc-1" || l.ID != creada.ID || l.Motivo != "ya no hace falta" {
		t.Fatalf("llamada a Operaciones incorrecta: %+v", l)
	}
	guardada, _ := repo.BuscarPorID(context.Background(), org, creada.ID)
	if guardada.Version != 2 || guardada.Vista.Traslado.Estado != models.EstadoCancelado {
		t.Fatalf("no se aplicó la vista devuelta: %+v", guardada)
	}
}

func TestCancelarMapeaLosErroresDeOperaciones(t *testing.T) {
	casos := []struct {
		nombre     string
		err        error
		estadoHTTP int
		codigo     string
		reintento  int
	}{
		{"ya empezó", clients.ErrRechazo{Estado: 409, Codigo: "ESTADO_NO_PERMITE_OPERACION"}, http.StatusConflict, "ESTADO_NO_PERMITE_OPERACION", 0},
		{"datos inválidos", clients.ErrRechazo{Estado: 422, Codigo: "DATOS_INVALIDOS"}, http.StatusUnprocessableEntity, "DATOS_INVALIDOS", 0},
		{"no la encuentra", clients.ErrNoEncontrada, http.StatusNotFound, "NO_ENCONTRADA", 0},
		{"no disponible", clients.ErrNoDisponible, http.StatusServiceUnavailable, "SERVICIO_NO_DISPONIBLE", ReintentarNoDisponible},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			s, _, cliente := nuevoServicio()
			creada := crearReserva(t, s)
			cliente.Error = c.err
			_, err := s.Cancelar(context.Background(), org, "clave-canc-1", creada.ID, "ya no hace falta")
			e := errorDe(t, err)
			if e.Estado != c.estadoHTTP || e.Codigo != c.codigo || e.ReintentoS != c.reintento {
				t.Fatalf("se esperaba %d %s (Retry-After %d), se obtuvo %+v", c.estadoHTTP, c.codigo, c.reintento, e)
			}
		})
	}
}

func TestAccionSobreTrasladoInexistenteNoLlamaAOperaciones(t *testing.T) {
	s, _, cliente := nuevoServicio()
	_, err := s.Cancelar(context.Background(), org, "clave-canc-1", "sol-no-existe", "motivo")
	if e := errorDe(t, err); e.Estado != http.StatusNotFound || len(cliente.Llamadas) != 0 {
		t.Fatalf("se esperaba 404 sin llamar a Operaciones: %+v, llamadas %d", e, len(cliente.Llamadas))
	}
}

func sobre(t *testing.T, id string, version int64, origen string, estado string) mensajeria.Sobre {
	t.Helper()
	s, err := mensajeria.NuevoSobre(context.Background(), "operaciones.traslado.estado_cambiado", id, version, map[string]any{
		"organizacion_id": org,
		"origen":          origen,
		"solicitud":       models.VistaSolicitud{ID: id, Estado: "ACEPTADA", Version: version, Traslado: &models.Traslado{Estado: estado}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestAplicarEventoExternoActualizaLaCopia(t *testing.T) {
	s, repo, _ := nuevoServicio()
	creada := crearReserva(t, s)
	if err := s.AplicarEvento(context.Background(), sobre(t, creada.ID, 3, "externo", models.EstadoAsignado)); err != nil {
		t.Fatal(err)
	}
	// Un evento viejo que llega tarde (o el de la creación, que ya se guardó) no pisa el estado nuevo.
	if err := s.AplicarEvento(context.Background(), sobre(t, creada.ID, 1, "externo", models.EstadoProgramado)); err != nil {
		t.Fatal(err)
	}
	guardada, _ := repo.BuscarPorID(context.Background(), org, creada.ID)
	if guardada.Version != 3 || guardada.Vista.Traslado.Estado != models.EstadoAsignado {
		t.Fatalf("copia incorrecta: versión %d estado %s", guardada.Version, guardada.Vista.Traslado.Estado)
	}
}

func TestAplicarEventoInternoSeIgnora(t *testing.T) {
	s, repo, _ := nuevoServicio()
	creada := crearReserva(t, s)
	if err := s.AplicarEvento(context.Background(), sobre(t, creada.ID, 3, "interno", models.EstadoAsignado)); err != nil {
		t.Fatal(err)
	}
	guardada, _ := repo.BuscarPorID(context.Background(), org, creada.ID)
	if guardada.Version != 1 {
		t.Fatal("un evento de una solicitud interna no tiene que aplicarse")
	}
}

func TestAplicarEventoConDatosInvalidosEsPermanente(t *testing.T) {
	s, _, _ := nuevoServicio()
	err := s.AplicarEvento(context.Background(), mensajeria.Sobre{Tipo: "operaciones.solicitud.aceptada", Datos: []byte(`"no es un objeto"`)})
	if !mensajeria.EsPermanente(err) {
		t.Fatalf("datos ilegibles tienen que ir a la DLQ: %v", err)
	}
}

func TestAplicarEventoConMongoCaidoSeReintenta(t *testing.T) {
	s, repo, _ := nuevoServicio()
	repo.Error = errors.New("mongo caído")
	err := s.AplicarEvento(context.Background(), sobre(t, "sol-1", 3, "externo", models.EstadoAsignado))
	if err == nil || mensajeria.EsPermanente(err) {
		t.Fatalf("un error de MongoDB es transitorio: %v", err)
	}
}
