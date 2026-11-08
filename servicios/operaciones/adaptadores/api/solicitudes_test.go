package api

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

	"github.com/francoroblesss/Rumbo/servicios/operaciones/dominio"
	"github.com/francoroblesss/Rumbo/servicios/operaciones/puertos"
)

// casosFalsos responde lo configurado y guarda los argumentos de la última llamada.
type casosFalsos struct {
	puertos.Solicitudes
	err      error
	org      string
	idem     *puertos.Idempotencia
	hora     time.Time
	filtro   puertos.FiltroListado
	datos    dominio.DatosSolicitud
	fecha    string
	peso     float64
	destino  dominio.EstadoTraslado
	llamadas int
	web      bool // se llamó a CancelarDesdeWeb (y no al Cancelar interno)
}

func (f *casosFalsos) vista() (puertos.Vista, error) {
	f.llamadas++
	if f.err != nil {
		return puertos.Vista{}, f.err
	}
	return puertos.Vista{ID: "sol-1", Estado: "ACEPTADA"}, nil
}

func (f *casosFalsos) Obtener(_ context.Context, org, _ string) (puertos.Vista, error) {
	f.org = org
	return f.vista()
}

func (f *casosFalsos) Reservar(_ context.Context, org string, d dominio.DatosSolicitud, inicio time.Time, idem *puertos.Idempotencia) (puertos.Vista, error) {
	f.org, f.idem, f.hora, f.datos = org, idem, inicio, d
	return f.vista()
}

func (f *casosFalsos) Disponibilidad(_ context.Context, fecha string, peso float64) (puertos.Disponibilidad, error) {
	f.fecha, f.peso = fecha, peso
	return puertos.Disponibilidad{Fecha: fecha, Franjas: []puertos.FranjaDisponible{}}, f.err
}

func (f *casosFalsos) Cancelar(_ context.Context, org, _, _ string, idem *puertos.Idempotencia) (puertos.Vista, error) {
	f.org, f.idem = org, idem
	return f.vista()
}

func (f *casosFalsos) CancelarDesdeWeb(_ context.Context, org, _, _ string, idem *puertos.Idempotencia) (puertos.Vista, error) {
	f.org, f.idem, f.web = org, idem, true
	return f.vista()
}

func (f *casosFalsos) AvanzarEstado(_ context.Context, org, _ string, destino dominio.EstadoTraslado) (puertos.Vista, error) {
	f.org, f.destino = org, destino
	return f.vista()
}

func (f *casosFalsos) Listar(_ context.Context, filtro puertos.FiltroListado) (puertos.Pagina, error) {
	f.filtro = filtro
	f.llamadas++
	return puertos.Pagina{Resultados: []puertos.Vista{}, Pagina: filtro.Pagina, Tamanio: filtro.Tamanio}, f.err
}

func (f *casosFalsos) Franjas(_ context.Context, fecha string) (puertos.FranjasDelDia, error) {
	f.fecha = fecha
	f.llamadas++
	return puertos.FranjasDelDia{Fecha: fecha, Franjas: []puertos.FranjaOcupada{}}, f.err
}

func pedir(t *testing.T, f *casosFalsos, metodo, ruta, cuerpo string, headers map[string]string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	NuevoHandler(f).Registrar(r)
	req := httptest.NewRequest(metodo, ruta, strings.NewReader(cuerpo))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var json_ map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &json_)
	return w, json_
}

var (
	operador      = map[string]string{"X-Rumbo-Roles": "operador"}
	administrador = map[string]string{"X-Rumbo-Roles": "administrador"}
	cliente       = map[string]string{"X-Rumbo-Roles": "cliente", "X-Rumbo-Organizacion": "org-1"}
)

func con(base map[string]string, extra ...string) map[string]string {
	h := map[string]string{}
	for k, v := range base {
		h[k] = v
	}
	for i := 0; i+1 < len(extra); i += 2 {
		h[extra[i]] = extra[i+1]
	}
	return h
}

func TestRoles(t *testing.T) {
	casos := []struct {
		nombre, metodo, ruta, cuerpo string
		headers                      map[string]string
		estado                       int
	}{
		{"el cliente no ve las franjas", http.MethodGet, "/franjas?fecha=2026-10-16", "", cliente, 403},
		{"el cliente no avanza estados", http.MethodPost, "/solicitudes/sol-1/estado", `{"estado":"EN_CURSO"}`, cliente, 403},
		{"el sistema externo no usa la API de la web", http.MethodGet, "/solicitudes", "", map[string]string{"X-Rumbo-Roles": "sistema_externo", "X-Rumbo-Organizacion": "org-1"}, 403},
		{"sin roles", http.MethodGet, "/solicitudes/sol-1", "", nil, 403},
		{"un cliente sin organización", http.MethodGet, "/solicitudes", "", map[string]string{"X-Rumbo-Roles": "cliente"}, 403},
		{"el administrador ve las franjas", http.MethodGet, "/franjas?fecha=2026-10-16", "", administrador, 200},
		{"el operador avanza estados", http.MethodPost, "/solicitudes/sol-1/estado", `{"estado":"EN_CURSO"}`, operador, 200},
		{"el cliente ve el detalle", http.MethodGet, "/solicitudes/sol-1", "", cliente, 200},
		{"el cliente cancela", http.MethodPost, "/solicitudes/sol-1/cancelacion", `{"motivo":"no"}`, con(cliente, "Idempotency-Key", "clave-1"), 200},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			f := &casosFalsos{}
			w, p := pedir(t, f, c.metodo, c.ruta, c.cuerpo, c.headers)
			if w.Code != c.estado {
				t.Fatalf("estado = %d, se esperaba %d: %v", w.Code, c.estado, p)
			}
			if c.estado == 403 && (p["codigo"] != "SIN_PERMISO" || f.llamadas != 0) {
				t.Fatalf("codigo = %v, llamadas = %d", p["codigo"], f.llamadas)
			}
		})
	}
}

func TestOrganizacionDelCliente(t *testing.T) {
	t.Run("detalle de otra organización: 404 sin revelar que existe", func(t *testing.T) {
		f := &casosFalsos{err: puertos.ErrNoEncontrada}
		w, p := pedir(t, f, http.MethodGet, "/solicitudes/sol-1", "", cliente)
		if w.Code != 404 || p["codigo"] != "NO_ENCONTRADA" || f.org != "org-1" {
			t.Fatalf("estado = %d, org = %q, %v", w.Code, f.org, p)
		}
	})
	t.Run("el personal de Rumbo ve todas", func(t *testing.T) {
		f := &casosFalsos{}
		if w, _ := pedir(t, f, http.MethodGet, "/solicitudes/sol-1", "", con(operador, "X-Rumbo-Organizacion", "org-9")); w.Code != 200 || f.org != "" {
			t.Fatalf("estado = %d, org = %q", w.Code, f.org)
		}
	})
	t.Run("en el listado se le fuerza la suya e ignora organizacion_id", func(t *testing.T) {
		f := &casosFalsos{}
		w, _ := pedir(t, f, http.MethodGet, "/solicitudes?organizacion_id=org-2&fecha=2026-10-16&estado_traslado=EN_CURSO", "", cliente)
		if w.Code != 200 || f.filtro.Organizacion != "org-1" || f.filtro.Fecha != "2026-10-16" || f.filtro.EstadoTraslado != "EN_CURSO" {
			t.Fatalf("estado = %d, filtro = %+v", w.Code, f.filtro)
		}
	})
	t.Run("el operador filtra por organizacion_id", func(t *testing.T) {
		f := &casosFalsos{}
		if w, _ := pedir(t, f, http.MethodGet, "/solicitudes?organizacion_id=org-2", "", operador); w.Code != 200 || f.filtro.Organizacion != "org-2" {
			t.Fatalf("estado = %d, filtro = %+v", w.Code, f.filtro)
		}
	})
	t.Run("cancelar: con la organización del cliente y el caso de uso de la web", func(t *testing.T) {
		f := &casosFalsos{}
		w, _ := pedir(t, f, http.MethodPost, "/solicitudes/sol-1/cancelacion", `{"motivo":"no"}`, con(cliente, "Idempotency-Key", "clave-1"))
		if w.Code != 200 || !f.web || f.org != "org-1" || f.idem.Endpoint != "cancelacion" || f.idem.Clave != "clave-1" {
			t.Fatalf("estado = %d, web = %v, org = %q, idem = %+v", w.Code, f.web, f.org, f.idem)
		}
		f = &casosFalsos{}
		pedir(t, f, http.MethodPost, "/solicitudes/sol-1/cancelacion", `{"motivo":"no"}`, con(operador, "Idempotency-Key", "clave-1"))
		if !f.web || f.org != "" {
			t.Fatalf("el operador cancela cualquiera: org = %q", f.org)
		}
	})
}

func TestMapeoDeErrores(t *testing.T) {
	casos := []struct {
		err    error
		estado int
		codigo string
	}{
		{puertos.ErrNoEncontrada, 404, "NO_ENCONTRADA"},
		{dominio.ErrEstadoNoPermite, 409, "ESTADO_NO_PERMITE_OPERACION"},
		{dominio.ErrHorarioEnElPasado, 422, "HORARIO_EN_EL_PASADO"},
		{dominio.ErrDatosInvalidos, 422, "DATOS_INVALIDOS"},
		{puertos.ErrClaveReutilizada, 409, "CLAVE_IDEMPOTENCIA_REUTILIZADA"},
		{errors.New("conexión rechazada"), 503, "SERVICIO_NO_DISPONIBLE"},
	}
	for _, c := range casos {
		w, p := pedir(t, &casosFalsos{err: c.err}, http.MethodPost, "/solicitudes/sol-1/estado", `{"estado":"COMPLETADO"}`, operador)
		if w.Code != c.estado || p["codigo"] != c.codigo || w.Header().Get("Content-Type") != "application/problem+json" {
			t.Fatalf("%v: estado = %d, %v", c.err, w.Code, p)
		}
	}
}

func TestAvanzarEstado(t *testing.T) {
	f := &casosFalsos{}
	w, _ := pedir(t, f, http.MethodPost, "/solicitudes/sol-1/estado", `{"estado":"EN_CURSO"}`, operador)
	if w.Code != 200 || f.destino != dominio.TrasladoEnCurso || f.org != "" {
		t.Fatalf("estado = %d, destino = %s", w.Code, f.destino)
	}
	casos := []struct {
		cuerpo string
		estado int
	}{
		{`{"estado":"PROGRAMADO"}`, 422},
		{`{"estado":"CANCELADO"}`, 422},
		{`{"estado":"VOLANDO"}`, 422},
		{`{}`, 422},
		{`{"estado":1}`, 422},
		{`{"estado":`, 400},
	}
	for _, c := range casos {
		f := &casosFalsos{}
		w, p := pedir(t, f, http.MethodPost, "/solicitudes/sol-1/estado", c.cuerpo, operador)
		if w.Code != c.estado || f.llamadas != 0 {
			t.Fatalf("%s: estado = %d, %v", c.cuerpo, w.Code, p)
		}
	}
	w, p := pedir(t, &casosFalsos{err: dominio.ErrEstadoNoPermite}, http.MethodPost, "/solicitudes/sol-1/estado", `{"estado":"COMPLETADO"}`, operador)
	if w.Code != 409 || p["codigo"] != "ESTADO_NO_PERMITE_OPERACION" {
		t.Fatalf("estado = %d, %v", w.Code, p)
	}
}

func TestCancelacionDesdeLaWeb(t *testing.T) {
	ruta := "/solicitudes/sol-1/cancelacion"
	w, p := pedir(t, &casosFalsos{}, http.MethodPost, ruta, `{"motivo":"no"}`, cliente)
	if w.Code != 422 || p["codigo"] != "DATOS_INVALIDOS" {
		t.Fatalf("sin Idempotency-Key: %d %v", w.Code, p)
	}
	if w, _ := pedir(t, &casosFalsos{}, http.MethodPost, ruta, `{"motivo":" "}`, con(cliente, "Idempotency-Key", "clave-1")); w.Code != 422 {
		t.Fatalf("sin motivo: %d", w.Code)
	}
	for err, codigo := range map[error]string{
		dominio.ErrEstadoNoPermite: "ESTADO_NO_PERMITE_OPERACION", puertos.ErrClaveReutilizada: "CLAVE_IDEMPOTENCIA_REUTILIZADA",
		puertos.ErrNoEncontrada: "NO_ENCONTRADA",
	} {
		if _, p := pedir(t, &casosFalsos{err: err}, http.MethodPost, ruta, `{"motivo":"no"}`, con(cliente, "Idempotency-Key", "clave-1")); p["codigo"] != codigo {
			t.Fatalf("%v: %v", err, p)
		}
	}
}

func TestFranjas(t *testing.T) {
	f := &casosFalsos{}
	if w, _ := pedir(t, f, http.MethodGet, "/franjas?fecha=2026-10-16", "", operador); w.Code != 200 || f.fecha != "2026-10-16" {
		t.Fatalf("estado = %d, fecha = %s", w.Code, f.fecha)
	}
	if w, _ := pedir(t, &casosFalsos{}, http.MethodGet, "/franjas", "", operador); w.Code != 422 {
		t.Fatalf("sin fecha: %d", w.Code)
	}
	if w, p := pedir(t, &casosFalsos{err: dominio.ErrDatosInvalidos}, http.MethodGet, "/franjas?fecha=2030-01-01", "", operador); w.Code != 422 || p["codigo"] != "DATOS_INVALIDOS" {
		t.Fatalf("fuera del período: %d %v", w.Code, p)
	}
}

func TestListarParametros(t *testing.T) {
	f := &casosFalsos{}
	w, _ := pedir(t, f, http.MethodGet, "/solicitudes", "", operador)
	if w.Code != 200 || f.filtro.Pagina != 1 || f.filtro.Tamanio != 20 || f.filtro.Organizacion != "" {
		t.Fatalf("estado = %d, filtro = %+v", w.Code, f.filtro)
	}
	w, _ = pedir(t, f, http.MethodGet, "/solicitudes?pagina=x", "", operador)
	if w.Code != 400 {
		t.Fatalf("estado = %d", w.Code)
	}
	if w, _ := pedir(t, &casosFalsos{err: dominio.ErrDatosInvalidos}, http.MethodGet, "/solicitudes?estado_traslado=ASIGNADO", "", operador); w.Code != 422 {
		t.Fatalf("estado inválido: %d", w.Code)
	}
}

const reserva = `{"cosa":{"descripcion":"Cajas","cantidad":20,"peso_kg":350},
	"origen":{"nombre":"Sucursal Centro","direccion":"Av. Colón 1234"},
	"destino":{"nombre":"Sucursal Cerro","direccion":"Av. R. Núñez 4500"},
	"franja_inicio":"2026-10-16T10:00:00-03:00","referencia_externa":"TRF-1"}`

func TestEndpointsInternos(t *testing.T) {
	ruta := "/internas/reservas"
	completos := map[string]string{"X-Rumbo-Organizacion": "org-1", "Idempotency-Key": "clave-0001"}

	w, p := pedir(t, &casosFalsos{}, http.MethodPost, ruta, reserva, map[string]string{"Idempotency-Key": "clave-0001"})
	if w.Code != 422 || p["codigo"] != "DATOS_INVALIDOS" {
		t.Fatalf("sin organización: %d %v", w.Code, p)
	}
	w, _ = pedir(t, &casosFalsos{}, http.MethodPost, ruta, reserva, map[string]string{"X-Rumbo-Organizacion": "org-1"})
	if w.Code != 422 {
		t.Fatalf("sin Idempotency-Key: %d", w.Code)
	}
	for _, mala := range []string{`{"cosa":{"descripcion":"Cajas","cantidad":1}}`, strings.Replace(reserva, "2026-10-16T10:00:00-03:00", "mañana", 1)} {
		if w, _ := pedir(t, &casosFalsos{}, http.MethodPost, ruta, mala, completos); w.Code != 422 {
			t.Fatalf("franja_inicio ausente o inválida: %d", w.Code)
		}
	}

	f := &casosFalsos{}
	w, _ = pedir(t, f, http.MethodPost, ruta, reserva, completos)
	if w.Code != 201 || f.org != "org-1" || f.idem.Clave != "clave-0001" || f.idem.Endpoint != "reserva" ||
		!f.hora.Equal(time.Date(2026, 10, 16, 13, 0, 0, 0, time.UTC)) || *f.datos.Cosa.PesoKg != 350 ||
		f.datos.Hasta.Nombre != "Sucursal Cerro" || *f.datos.ReferenciaExterna != "TRF-1" {
		t.Fatalf("estado = %d, org = %s, idem = %+v, datos = %+v", w.Code, f.org, f.idem, f.datos)
	}
	hashReserva := f.idem.Hash
	pedir(t, f, http.MethodPost, ruta, strings.Replace(reserva, "350", "351", 1), completos)
	if f.idem.Hash == hashReserva || len(hashReserva) != 64 {
		t.Fatal("cuerpos distintos tienen que dar hashes distintos (SHA-256 en hexadecimal)")
	}

	for err, codigo := range map[error]string{
		dominio.ErrSinDisponibilidad: "SIN_DISPONIBILIDAD", dominio.ErrExcedeCapacidad: "EXCEDE_CAPACIDAD",
		dominio.ErrAnticipacionInsuficiente: "ANTICIPACION_INSUFICIENTE", dominio.ErrFranjaInvalida: "DATOS_INVALIDOS",
	} {
		if _, p := pedir(t, &casosFalsos{err: err}, http.MethodPost, ruta, reserva, completos); p["codigo"] != codigo {
			t.Fatalf("%v: %v", err, p)
		}
	}

	w, _ = pedir(t, f, http.MethodGet, "/internas/disponibilidad?fecha=2026-10-16&peso_kg=350", "", nil)
	if w.Code != 200 || f.fecha != "2026-10-16" || f.peso != 350 {
		t.Fatalf("disponibilidad: %d, fecha = %s, peso = %v", w.Code, f.fecha, f.peso)
	}
	for _, q := range []string{"", "?fecha=2026-10-16&peso_kg=-1"} {
		if w, _ := pedir(t, f, http.MethodGet, "/internas/disponibilidad"+q, "", nil); w.Code != 422 {
			t.Fatalf("disponibilidad%s: %d", q, w.Code)
		}
	}
	if w, _ := pedir(t, &casosFalsos{err: dominio.ErrFranjaInvalida}, http.MethodGet, "/internas/disponibilidad?fecha=2030-01-01", "", nil); w.Code != 422 {
		t.Fatalf("fecha fuera del período: %d", w.Code)
	}

	w, _ = pedir(t, f, http.MethodPost, "/internas/solicitudes/sol-1/cancelacion", `{"motivo":"cambio de planes"}`, completos)
	if w.Code != 200 || f.idem.Endpoint != "cancelacion" {
		t.Fatalf("estado = %d, idem = %+v", w.Code, f.idem)
	}
}
