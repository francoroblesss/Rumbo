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

const cuerpoMinimo = `{"cosa":{"descripcion":"Cajas","cantidad":3},"origen":{"nombre":"A","direccion":"x"},"destino":{"nombre":"B","direccion":"y"}}`

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

func TestCrearValidoQuedaRecibidaYPendiente(t *testing.T) {
	s, repo, _ := nuevoServicio()
	vista, estado, err := s.Crear(context.Background(), org, "clave-0001", []byte(cuerpoMinimo))
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if estado != http.StatusAccepted || vista.Estado != models.EstadoRecibida || !strings.HasPrefix(vista.ID, "sol-") {
		t.Fatalf("respuesta inesperada: %d %+v", estado, vista)
	}
	if vista.ToleranciaMinutos != 0 || vista.Version != 0 || !vista.CreadoEn.Equal(ahoraFija) || !vista.ActualizadoEn.Equal(ahoraFija) {
		t.Fatalf("vista inicial incorrecta: %+v", vista)
	}
	guardada, err := repo.BuscarPorID(context.Background(), org, vista.ID)
	if err != nil || !guardada.PendienteEnvio || guardada.Version != 0 || guardada.Idempotencia.HashCuerpo == "" {
		t.Fatalf("solicitud mal guardada: %+v %v", guardada, err)
	}
}

func TestCrearJSONMalFormado(t *testing.T) {
	s, _, _ := nuevoServicio()
	for _, cuerpo := range []string{`{`, ``, `{"cosa":{"cantidad":"tres"}}`} {
		_, _, err := s.Crear(context.Background(), org, "clave-0001", []byte(cuerpo))
		if e := errorDe(t, err); e.Estado != http.StatusBadRequest || e.Codigo != "JSON_MAL_FORMADO" {
			t.Fatalf("cuerpo %q: se esperaba 400 JSON_MAL_FORMADO, se obtuvo %+v", cuerpo, e)
		}
	}
}

func TestCrearJuntaTodosLosErroresPorCampo(t *testing.T) {
	s, repo, _ := nuevoServicio()
	largo := func(n int) string { return strings.Repeat("a", n) }
	cuerpo, _ := json.Marshal(map[string]any{
		"referencia_externa": largo(65),
		"cosa":               map[string]any{"descripcion": largo(251), "cantidad": 0, "peso_kg": 0, "volumen_m3": -1},
		"origen":             map[string]any{"nombre": largo(121), "direccion": largo(251), "contacto": largo(121)},
		"destino":            map[string]any{"nombre": " ", "direccion": ""},
		"tolerancia_minutos": 721,
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
		"tolerancia_minutos", "observaciones"} {
		if !campos[campo] {
			t.Errorf("falta el error del campo %s (errores: %+v)", campo, e.Errores)
		}
	}
	if n, _ := repo.ContarPendientes(context.Background()); n != 0 {
		t.Fatal("un pedido inválido no se tiene que registrar")
	}
}

func TestCrearSinCosaNiUbicaciones(t *testing.T) {
	s, _, _ := nuevoServicio()
	_, _, err := s.Crear(context.Background(), org, "clave-0001", []byte(`{}`))
	campos := camposConError(errorDe(t, err))
	for _, campo := range []string{"cosa.descripcion", "cosa.cantidad", "origen.nombre", "origen.direccion", "destino.nombre", "destino.direccion"} {
		if !campos[campo] {
			t.Errorf("falta el error del campo obligatorio %s", campo)
		}
	}
}

func TestCrearAceptaLosLimitesExactos(t *testing.T) {
	s, _, _ := nuevoServicio()
	cuerpo, _ := json.Marshal(map[string]any{
		"referencia_externa": strings.Repeat("r", 64),
		"cosa":               map[string]any{"descripcion": strings.Repeat("ñ", 250), "cantidad": 1, "peso_kg": 0.1, "volumen_m3": 0.1},
		"origen":             map[string]any{"nombre": strings.Repeat("n", 120), "direccion": strings.Repeat("d", 250), "contacto": strings.Repeat("c", 120)},
		"destino":            map[string]any{"nombre": "B", "direccion": "y"},
		"tolerancia_minutos": 720,
		"observaciones":      strings.Repeat("o", 500),
	})
	if _, _, err := s.Crear(context.Background(), org, "clave-0001", cuerpo); err != nil {
		t.Fatalf("los valores en el límite son válidos: %v", err)
	}
}

func TestCrearHorarioEnElPasado(t *testing.T) {
	s, _, _ := nuevoServicio()
	cuerpo := strings.Replace(cuerpoMinimo, `{"cosa"`, `{"hora_llegada_solicitada":"2026-10-15T08:00:00-03:00","cosa"`, 1)
	_, _, err := s.Crear(context.Background(), org, "clave-0001", []byte(cuerpo))
	if e := errorDe(t, err); e.Estado != http.StatusUnprocessableEntity || e.Codigo != "HORARIO_EN_EL_PASADO" {
		t.Fatalf("se esperaba 422 HORARIO_EN_EL_PASADO, se obtuvo %+v", e)
	}
	futuro := strings.Replace(cuerpoMinimo, `{"cosa"`, `{"hora_llegada_solicitada":"2026-10-16T14:00:00-03:00","tolerancia_minutos":120,"cosa"`, 1)
	vista, _, err := s.Crear(context.Background(), org, "clave-0002", []byte(futuro))
	if err != nil || vista.ToleranciaMinutos != 120 || vista.HoraLlegadaSolicitada == nil ||
		!vista.HoraLlegadaSolicitada.Equal(time.Date(2026, 10, 16, 17, 0, 0, 0, time.UTC)) {
		t.Fatalf("una hora futura es válida: %+v %v", vista, err)
	}
}

func TestCrearReintentoIdempotenteDevuelveLaMismaRespuesta(t *testing.T) {
	s, repo, _ := nuevoServicio()
	primera, _, err := s.Crear(context.Background(), org, "clave-0001", []byte(cuerpoMinimo))
	if err != nil {
		t.Fatal(err)
	}
	// Mismo pedido con otro orden de campos y espacios: el cuerpo normalizado es el mismo.
	reordenado := `{ "destino":{"direccion":"y","nombre":"B"}, "origen":{"nombre":"A","direccion":"x"}, "cosa":{"cantidad":3,"descripcion":"Cajas"} }`
	segunda, estado, err := s.Crear(context.Background(), org, "clave-0001", []byte(reordenado))
	if err != nil || estado != http.StatusAccepted {
		t.Fatalf("el reintento idempotente tiene que dar 202: %d %v", estado, err)
	}
	if segunda.ID != primera.ID || !segunda.CreadoEn.Equal(primera.CreadoEn) {
		t.Fatalf("el reintento tiene que devolver la misma solicitud: %s vs %s", segunda.ID, primera.ID)
	}
	if n, _ := repo.ContarPendientes(context.Background()); n != 1 {
		t.Fatalf("se tiene que registrar una sola solicitud, hay %d", n)
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

func TestObtenerYNoEncontrada(t *testing.T) {
	s, _, _ := nuevoServicio()
	creada, _, _ := s.Crear(context.Background(), org, "clave-0001", []byte(cuerpoMinimo))
	vista, err := s.Obtener(context.Background(), org, creada.ID)
	if err != nil || vista.ID != creada.ID {
		t.Fatalf("no se obtuvo la solicitud: %v", err)
	}
	_, err = s.Obtener(context.Background(), "org-otra", creada.ID)
	if e := errorDe(t, err); e.Estado != http.StatusNotFound || e.Codigo != "NO_ENCONTRADA" {
		t.Fatalf("otra organización no tiene que ver la solicitud: %+v", e)
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
	vacia, _ := s.Listar(context.Background(), repositories.FiltroListado{OrganizacionID: "org-otra", Pagina: 1, Tamanio: 20})
	if vacia.Resultados == nil || len(vacia.Resultados) != 0 {
		t.Fatal("una página vacía tiene que traer una lista vacía, no null")
	}
}

func crearSolicitud(t *testing.T, s *SolicitudesService) models.VistaSolicitud {
	t.Helper()
	v, _, err := s.Crear(context.Background(), org, "clave-0001", []byte(cuerpoMinimo))
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestResponderContrapropuestaExitosaAplicaLaVista(t *testing.T) {
	s, repo, cliente := nuevoServicio()
	creada := crearSolicitud(t, s)
	cliente.Vista = models.VistaSolicitud{ID: creada.ID, Estado: models.EstadoAceptada, Version: 4}
	vista, err := s.ResponderContrapropuesta(context.Background(), org, "clave-resp-1", creada.ID, true)
	if err != nil || vista.Estado != models.EstadoAceptada {
		t.Fatalf("respuesta inesperada: %+v %v", vista, err)
	}
	if l := cliente.Llamadas[0]; l.Org != org || l.ClaveIdem != "clave-resp-1" || l.ID != creada.ID || !l.Acepta {
		t.Fatalf("llamada a Operaciones incorrecta: %+v", l)
	}
	guardada, _ := repo.BuscarPorID(context.Background(), org, creada.ID)
	if guardada.Version != 4 || guardada.Vista.Estado != models.EstadoAceptada {
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
		{"no recibida todavía", clients.ErrNoEncontrada, http.StatusServiceUnavailable, "SERVICIO_NO_DISPONIBLE", ReintentarSinRecibir},
		{"conflicto", clients.ErrConflicto{Codigo: "ESTADO_NO_PERMITE_OPERACION"}, http.StatusConflict, "ESTADO_NO_PERMITE_OPERACION", 0},
		{"contrapropuesta vencida", clients.ErrConflicto{Codigo: "CONTRAPROPUESTA_VENCIDA"}, http.StatusConflict, "CONTRAPROPUESTA_VENCIDA", 0},
		{"datos inválidos", clients.ErrDatosInvalidos, http.StatusUnprocessableEntity, "DATOS_INVALIDOS", 0},
		{"no disponible", clients.ErrNoDisponible, http.StatusServiceUnavailable, "SERVICIO_NO_DISPONIBLE", ReintentarNoDisponible},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			s, _, cliente := nuevoServicio()
			creada := crearSolicitud(t, s)
			cliente.Error = c.err
			_, err := s.Cancelar(context.Background(), org, "clave-canc-1", creada.ID, "ya no hace falta")
			e := errorDe(t, err)
			if e.Estado != c.estadoHTTP || e.Codigo != c.codigo || e.ReintentoS != c.reintento {
				t.Fatalf("se esperaba %d %s (Retry-After %d), se obtuvo %+v", c.estadoHTTP, c.codigo, c.reintento, e)
			}
			if cliente.Llamadas[0].Motivo != "ya no hace falta" {
				t.Fatalf("no se envió el motivo: %+v", cliente.Llamadas[0])
			}
		})
	}
}

func TestOperacionesNoEncuentraUnaSolicitudYaProcesadaEs404(t *testing.T) {
	s, repo, cliente := nuevoServicio()
	creada := crearSolicitud(t, s)
	_, _, _ = repo.AplicarVista(context.Background(), creada.ID, 2, models.VistaSolicitud{ID: creada.ID, Estado: models.EstadoAceptada, Version: 2}, ahoraFija)
	cliente.Error = clients.ErrNoEncontrada
	_, err := s.ResponderContrapropuesta(context.Background(), org, "clave-resp-1", creada.ID, false)
	if e := errorDe(t, err); e.Estado != http.StatusNotFound {
		t.Fatalf("si la solicitud ya no está RECIBIDA, un 404 de Operaciones es 404: %+v", e)
	}
}

func TestAccionSobreSolicitudInexistenteNoLlamaAOperaciones(t *testing.T) {
	s, _, cliente := nuevoServicio()
	_, err := s.Cancelar(context.Background(), org, "clave-canc-1", "sol-no-existe", "motivo")
	if e := errorDe(t, err); e.Estado != http.StatusNotFound || len(cliente.Llamadas) != 0 {
		t.Fatalf("se esperaba 404 sin llamar a Operaciones: %+v, llamadas %d", e, len(cliente.Llamadas))
	}
}

func sobre(t *testing.T, id string, version int64, origen string, estado string) mensajeria.Sobre {
	t.Helper()
	s, err := mensajeria.NuevoSobre(context.Background(), "operaciones.solicitud.aceptada", id, version, map[string]any{
		"organizacion_id": org,
		"origen":          origen,
		"solicitud":       models.VistaSolicitud{ID: id, Estado: estado, Version: version},
	})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestAplicarEventoExternoActualizaLaCopia(t *testing.T) {
	s, repo, _ := nuevoServicio()
	creada := crearSolicitud(t, s)
	if err := s.AplicarEvento(context.Background(), sobre(t, creada.ID, 3, "externo", models.EstadoAceptada)); err != nil {
		t.Fatal(err)
	}
	// Un evento viejo que llega tarde no pisa el estado nuevo.
	if err := s.AplicarEvento(context.Background(), sobre(t, creada.ID, 1, "externo", models.EstadoRecibida)); err != nil {
		t.Fatal(err)
	}
	guardada, _ := repo.BuscarPorID(context.Background(), org, creada.ID)
	if guardada.Version != 3 || guardada.Vista.Estado != models.EstadoAceptada {
		t.Fatalf("copia incorrecta: versión %d estado %s", guardada.Version, guardada.Vista.Estado)
	}
}

func TestAplicarEventoInternoSeIgnora(t *testing.T) {
	s, repo, _ := nuevoServicio()
	creada := crearSolicitud(t, s)
	if err := s.AplicarEvento(context.Background(), sobre(t, creada.ID, 3, "interno", models.EstadoAceptada)); err != nil {
		t.Fatal(err)
	}
	guardada, _ := repo.BuscarPorID(context.Background(), org, creada.ID)
	if guardada.Version != 0 {
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
	err := s.AplicarEvento(context.Background(), sobre(t, "sol-1", 3, "externo", models.EstadoAceptada))
	if err == nil || mensajeria.EsPermanente(err) {
		t.Fatalf("un error de MongoDB es transitorio: %v", err)
	}
}
