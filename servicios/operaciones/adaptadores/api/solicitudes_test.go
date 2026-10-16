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

func (f *casosFalsos) ResponderContrapropuesta(_ context.Context, org, _ string, _ bool, idem *puertos.Idempotencia) (puertos.Vista, error) {
	f.org, f.idem = org, idem
	return f.vista()
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

func TestEndpointsInternos(t *testing.T) {
	ruta := "/internas/solicitudes/sol-1/respuesta-contrapropuesta"
	completos := map[string]string{"X-Rumbo-Organizacion": "org-1", "Idempotency-Key": "clave-0001"}

	w, p := pedir(t, &casosFalsos{}, http.MethodPost, ruta, `{"acepta":true}`, map[string]string{"Idempotency-Key": "clave-0001"})
	if w.Code != 422 || p["codigo"] != "DATOS_INVALIDOS" {
		t.Fatalf("sin organización: %d %v", w.Code, p)
	}
	w, _ = pedir(t, &casosFalsos{}, http.MethodPost, ruta, `{"acepta":true}`, map[string]string{"X-Rumbo-Organizacion": "org-1"})
	if w.Code != 422 {
		t.Fatalf("sin Idempotency-Key: %d", w.Code)
	}

	f := &casosFalsos{}
	w, _ = pedir(t, f, http.MethodPost, ruta, `{"acepta":true}`, completos)
	if w.Code != 200 || f.org != "org-1" || f.idem.Clave != "clave-0001" || f.idem.Endpoint != "respuesta-contrapropuesta" {
		t.Fatalf("estado = %d, org = %s, idem = %+v", w.Code, f.org, f.idem)
	}
	hashAcepta := f.idem.Hash
	pedir(t, f, http.MethodPost, ruta, `{"acepta":false}`, completos)
	if f.idem.Hash == hashAcepta || len(hashAcepta) != 64 {
		t.Fatal("cuerpos distintos tienen que dar hashes distintos (SHA-256 en hexadecimal)")
	}

	w, _ = pedir(t, f, http.MethodPost, "/internas/solicitudes/sol-1/cancelacion", `{"motivo":"cambio de planes"}`, completos)
	if w.Code != 200 || f.idem.Endpoint != "cancelacion" {
		t.Fatalf("estado = %d, idem = %+v", w.Code, f.idem)
	}
}
