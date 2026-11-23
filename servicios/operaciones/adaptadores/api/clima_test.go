package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/francoroblesss/Rumbo/servicios/operaciones/puertos"
)

// climaActualFalso responde lo configurado y cuenta las consultas.
type climaActualFalso struct {
	clima    puertos.ClimaAeropuerto
	err      error
	llamadas int
}

func (f *climaActualFalso) Consultar(context.Context) (puertos.ClimaAeropuerto, error) {
	f.llamadas++
	return f.clima, f.err
}

func pedirClima(t *testing.T, f *climaActualFalso, headers map[string]string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	NuevoHandlerClima(f).Registrar(r)
	req := httptest.NewRequest(http.MethodGet, "/clima", nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var cuerpo map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &cuerpo)
	return w, cuerpo
}

func TestClima(t *testing.T) {
	t.Run("200 con las condiciones", func(t *testing.T) {
		f := &climaActualFalso{clima: puertos.ClimaAeropuerto{
			Aeropuerto:  puertos.AeropuertoClima{ICAO: "SACO", Nombre: "Aeropuerto Taravella"},
			ObservadoEn: "2026-10-09T11:00:00-03:00", Operable: true, Motivos: []string{},
			Condiciones: puertos.CondicionesActual{TemperaturaC: 22.4, VientoKmh: 18, RafagasKmh: 27.5,
				DireccionVientoGrados: 340, VisibilidadM: 10000, PrecipitacionMm: 0, Tormenta: false},
		}}
		w, _ := pedirClima(t, f, operador)
		esperado := `{"aeropuerto":{"icao":"SACO","nombre":"Aeropuerto Taravella"},"observado_en":"2026-10-09T11:00:00-03:00",` +
			`"operable":true,"motivos":[],"condiciones":{"temperatura_c":22.4,"viento_kmh":18,"rafagas_kmh":27.5,` +
			`"direccion_viento_grados":340,"visibilidad_m":10000,"precipitacion_mm":0,"tormenta":false}}`
		if w.Code != 200 || w.Body.String() != esperado {
			t.Fatalf("estado = %d, cuerpo = %s", w.Code, w.Body.String())
		}
	})

	t.Run("503 con Retry-After si el clima no responde", func(t *testing.T) {
		f := &climaActualFalso{err: fmt.Errorf("%w: breaker abierto", puertos.ErrClimaNoDisponible)}
		w, p := pedirClima(t, f, cliente)
		if w.Code != 503 || p["codigo"] != "SERVICIO_NO_DISPONIBLE" || w.Header().Get("Retry-After") != "30" {
			t.Fatalf("estado = %d, codigo = %v, Retry-After = %q", w.Code, p["codigo"], w.Header().Get("Retry-After"))
		}
	})

	t.Run("roles", func(t *testing.T) {
		casos := []struct {
			nombre  string
			headers map[string]string
			estado  int
		}{
			{"el sistema externo no ve el clima", map[string]string{"X-Rumbo-Roles": "sistema_externo", "X-Rumbo-Organizacion": "org-1"}, 403},
			{"sin roles", nil, 403},
			{"el operador", operador, 200},
			{"el administrador", administrador, 200},
			{"el cliente", cliente, 200},
		}
		for _, c := range casos {
			t.Run(c.nombre, func(t *testing.T) {
				f := &climaActualFalso{clima: puertos.ClimaAeropuerto{Motivos: []string{}}}
				w, p := pedirClima(t, f, c.headers)
				if w.Code != c.estado {
					t.Fatalf("estado = %d, se esperaba %d: %v", w.Code, c.estado, p)
				}
				if c.estado == 403 && (p["codigo"] != "SIN_PERMISO" || f.llamadas != 0) {
					t.Fatalf("codigo = %v, llamadas = %d", p["codigo"], f.llamadas)
				}
			})
		}
	})
}
