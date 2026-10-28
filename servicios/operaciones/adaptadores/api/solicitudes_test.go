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
	err     error
	org     string
	extremo *dominio.Extremo
	idem    *puertos.Idempotencia
	hora    time.Time
	pagina  [2]int
	datos   dominio.DatosSolicitud
	fecha   string
	peso    float64
}

func (f *casosFalsos) vista() (puertos.Vista, error) {
	if f.err != nil {
		return puertos.Vista{}, f.err
	}
	return puertos.Vista{ID: "sol-1", Estado: "ACEPTADA"}, nil
}

func (f *casosFalsos) Obtener(_ context.Context, org, _ string) (puertos.Vista, error) {
	f.org = org
	return f.vista()
}

func (f *casosFalsos) Aceptar(_ context.Context, _ string, _ string, e *dominio.Extremo) (puertos.Vista, error) {
	f.extremo = e
	return f.vista()
}

func (f *casosFalsos) Contraproponer(_ context.Context, _ string, h time.Time) (puertos.Vista, error) {
	f.hora = h
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

func (f *casosFalsos) Listar(_ context.Context, _ string, pagina, tamanio int) (puertos.Pagina, error) {
	f.pagina = [2]int{pagina, tamanio}
	return puertos.Pagina{Resultados: []puertos.Vista{}, Pagina: pagina, Tamanio: tamanio}, f.err
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

var operador = map[string]string{"X-Rumbo-Roles": "operador"}

func TestRolRequerido(t *testing.T) {
	w, p := pedir(t, &casosFalsos{}, http.MethodGet, "/solicitudes/sol-1", "", map[string]string{"X-Rumbo-Roles": "cliente"})
	if w.Code != http.StatusForbidden || p["codigo"] != "SIN_PERMISO" {
		t.Fatalf("estado = %d, %v", w.Code, p)
	}
	f := &casosFalsos{}
	w, _ = pedir(t, f, http.MethodGet, "/solicitudes/sol-1", "", operador)
	if w.Code != http.StatusOK || f.org != "" {
		t.Fatalf("estado = %d, org = %q", w.Code, f.org)
	}
}

func TestMapeoDeErrores(t *testing.T) {
	casos := []struct {
		err    error
		estado int
		codigo string
	}{
		{puertos.ErrNoEncontrada, 404, "NO_ENCONTRADA"},
		{dominio.ErrEstadoNoPermite, 409, "ESTADO_NO_PERMITE_OPERACION"},
		{dominio.ErrContrapropuestaVencida, 409, "CONTRAPROPUESTA_VENCIDA"},
		{dominio.ErrSinHoraPedida, 409, "ESTADO_NO_PERMITE_OPERACION"},
		{dominio.ErrHorarioEnElPasado, 422, "HORARIO_EN_EL_PASADO"},
		{dominio.ErrDatosInvalidos, 422, "DATOS_INVALIDOS"},
		{puertos.ErrClaveReutilizada, 409, "CLAVE_IDEMPOTENCIA_REUTILIZADA"},
		{errors.New("conexión rechazada"), 503, "SERVICIO_NO_DISPONIBLE"},
	}
	for _, c := range casos {
		w, p := pedir(t, &casosFalsos{err: c.err}, http.MethodPost, "/solicitudes/sol-1/aceptacion", `{}`, operador)
		if w.Code != c.estado || p["codigo"] != c.codigo || w.Header().Get("Content-Type") != "application/problem+json" {
			t.Fatalf("%v: estado = %d, %v", c.err, w.Code, p)
		}
	}
	_, p := pedir(t, &casosFalsos{err: dominio.ErrSinHoraPedida}, http.MethodPost, "/solicitudes/sol-1/aceptacion", `{}`, operador)
	if p["detail"] != "la solicitud no tiene hora pedida: hay que contraproponer" {
		t.Fatalf("detalle = %v", p["detail"])
	}
}

func TestValidacionDelCuerpo(t *testing.T) {
	casos := []struct {
		ruta, cuerpo string
		estado       int
	}{
		{"/solicitudes/sol-1/aceptacion", `{"aeropuerto_extremo":`, 400},
		{"/solicitudes/sol-1/aceptacion", `{"aeropuerto_extremo":"PISTA"}`, 422},
		{"/solicitudes/sol-1/aceptacion", `{"aeropuerto_extremo":3}`, 422},
		{"/solicitudes/sol-1/contrapropuesta", `{}`, 422},
		{"/solicitudes/sol-1/contrapropuesta", `{"hora_llegada_propuesta":"mañana"}`, 422},
		{"/solicitudes/sol-1/rechazo", `{"motivo":"  "}`, 422},
		{"/solicitudes/sol-1/cancelacion-tardia", `{}`, 422},
	}
	for _, c := range casos {
		w, _ := pedir(t, &casosFalsos{}, http.MethodPost, c.ruta, c.cuerpo, operador)
		if w.Code != c.estado {
			t.Fatalf("%s %s: estado = %d, se esperaba %d", c.ruta, c.cuerpo, w.Code, c.estado)
		}
	}

	f := &casosFalsos{}
	w, _ := pedir(t, f, http.MethodPost, "/solicitudes/sol-1/aceptacion", `{"aeropuerto_extremo":"DESTINO"}`, operador)
	if w.Code != 200 || f.extremo == nil || *f.extremo != dominio.ExtremoDestino {
		t.Fatalf("estado = %d, extremo = %v", w.Code, f.extremo)
	}
	w, _ = pedir(t, f, http.MethodPost, "/solicitudes/sol-1/contrapropuesta", `{"hora_llegada_propuesta":"2026-10-16T14:00:00-03:00"}`, operador)
	if w.Code != 200 || !f.hora.Equal(time.Date(2026, 10, 16, 17, 0, 0, 0, time.UTC)) {
		t.Fatalf("estado = %d, hora = %v", w.Code, f.hora)
	}
}

func TestListarParametros(t *testing.T) {
	f := &casosFalsos{}
	w, _ := pedir(t, f, http.MethodGet, "/solicitudes", "", operador)
	if w.Code != 200 || f.pagina != [2]int{1, 20} {
		t.Fatalf("estado = %d, página = %v", w.Code, f.pagina)
	}
	w, _ = pedir(t, f, http.MethodGet, "/solicitudes?pagina=x", "", operador)
	if w.Code != 400 {
		t.Fatalf("estado = %d", w.Code)
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
