package clients

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/francoroblesss/Rumbo/servicios/comun/observabilidad"
	"github.com/francoroblesss/Rumbo/servicios/integracion/models"
)

func servidorQueResponde(t *testing.T, estado int, cuerpo any) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var llamadas atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		llamadas.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(estado)
		_ = json.NewEncoder(w).Encode(cuerpo)
	}))
	t.Cleanup(srv.Close)
	return srv, &llamadas
}

func TestClienteMapeaLasRespuestas(t *testing.T) {
	casos := []struct {
		estado   int
		cuerpo   any
		esperado func(error) bool
	}{
		{http.StatusCreated, models.VistaSolicitud{ID: "sol-1", Estado: "ACEPTADA", Version: 1}, func(err error) bool { return err == nil }},
		{http.StatusNotFound, map[string]string{"codigo": "NO_ENCONTRADA"}, func(err error) bool { return errors.Is(err, ErrNoEncontrada) }},
		{http.StatusConflict, map[string]string{"codigo": "SIN_DISPONIBILIDAD", "detail": "La franja ya no tiene lugar."}, func(err error) bool {
			var r ErrRechazo
			return errors.As(err, &r) && r.Estado == 409 && r.Codigo == "SIN_DISPONIBILIDAD" && r.Detalle == "La franja ya no tiene lugar."
		}},
		{http.StatusUnprocessableEntity, map[string]any{"codigo": "DATOS_INVALIDOS", "errores": []map[string]string{{"campo": "franja_inicio", "mensaje": "no"}}},
			func(err error) bool {
				var r ErrRechazo
				return errors.As(err, &r) && r.Estado == 422 && len(r.Errores) == 1 && r.Errores[0].Campo == "franja_inicio"
			}},
		{http.StatusInternalServerError, map[string]string{}, func(err error) bool { return errors.Is(err, ErrNoDisponible) }},
	}
	for _, c := range casos {
		srv, _ := servidorQueResponde(t, c.estado, c.cuerpo)
		vista, err := NewOperacionesClient(srv.URL).Reservar(context.Background(), "org-1", "clave-0001", models.PedidoTraslado{})
		if !c.esperado(err) {
			t.Fatalf("estado %d: error inesperado %v", c.estado, err)
		}
		if c.estado == http.StatusCreated && (vista.Version != 1 || vista.Estado != "ACEPTADA") {
			t.Fatalf("vista mal decodificada: %+v", vista)
		}
	}
}

func TestClienteDisponibilidad(t *testing.T) {
	var recibida *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recibida = r.Clone(context.Background())
		_ = json.NewEncoder(w).Encode(models.Disponibilidad{Fecha: "2026-11-20", Franjas: []models.FranjaDisponible{{Disponible: true}}})
	}))
	defer srv.Close()
	peso := 350.5
	d, err := NewOperacionesClient(srv.URL).Disponibilidad(context.Background(), "2026-11-20", &peso)
	if err != nil || len(d.Franjas) != 1 || !d.Franjas[0].Disponible {
		t.Fatalf("d = %+v, err = %v", d, err)
	}
	if recibida.Method != http.MethodGet || recibida.URL.Path != "/internas/disponibilidad" ||
		recibida.URL.Query().Get("fecha") != "2026-11-20" || recibida.URL.Query().Get("peso_kg") != "350.5" {
		t.Fatalf("pedido incorrecto: %s %s", recibida.Method, recibida.URL)
	}
}

func TestClienteEnviaHeadersYCuerpo(t *testing.T) {
	// Con un TracerProvider real, el cliente de comun/observabilidad propaga traceparent.
	otel.SetTracerProvider(sdktrace.NewTracerProvider())
	otel.SetTextMapPropagator(propagation.TraceContext{})

	var recibida *http.Request
	var cuerpo map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&cuerpo)
		recibida = r.Clone(context.Background())
		_ = json.NewEncoder(w).Encode(models.VistaSolicitud{})
	}))
	defer srv.Close()

	ctx := observabilidad.ConCorrelacion(context.Background(), "corr-1")
	if _, err := NewOperacionesClient(srv.URL).Cancelar(ctx, "org-1", "clave-0001", "sol-1", "ya no hace falta"); err != nil {
		t.Fatal(err)
	}
	if recibida.Method != http.MethodPost || recibida.URL.Path != "/internas/solicitudes/sol-1/cancelacion" {
		t.Fatalf("ruta incorrecta: %s %s", recibida.Method, recibida.URL.Path)
	}
	h := recibida.Header
	if h.Get("X-Rumbo-Organizacion") != "org-1" || h.Get("Idempotency-Key") != "clave-0001" || h.Get("Content-Type") != "application/json" {
		t.Fatalf("headers incorrectos: %v", h)
	}
	if h.Get("X-Correlation-ID") != "corr-1" || h.Get("traceparent") == "" {
		t.Fatalf("faltan la correlación o la traza: %v", h)
	}
	if cuerpo["motivo"] != "ya no hace falta" {
		t.Fatalf("cuerpo incorrecto: %v", cuerpo)
	}
}

func TestBreakerSeAbreALasCincoFallasConsecutivas(t *testing.T) {
	srv, llamadas := servidorQueResponde(t, http.StatusInternalServerError, map[string]string{})
	cliente := NewOperacionesClient(srv.URL)
	for i := 0; i < 5; i++ {
		if _, err := cliente.Cancelar(context.Background(), "org-1", "clave-0001", "sol-1", "m"); !errors.Is(err, ErrNoDisponible) {
			t.Fatalf("llamada %d: se esperaba ErrNoDisponible, se obtuvo %v", i+1, err)
		}
	}
	if _, err := cliente.Cancelar(context.Background(), "org-1", "clave-0001", "sol-1", "m"); !errors.Is(err, ErrNoDisponible) {
		t.Fatalf("con el breaker abierto se esperaba ErrNoDisponible: %v", err)
	}
	if n := llamadas.Load(); n != 5 {
		t.Fatalf("la sexta llamada no tenía que llegar al servidor (llegaron %d)", n)
	}
}

func TestUnConflictoNoCuentaComoFalla(t *testing.T) {
	srv, llamadas := servidorQueResponde(t, http.StatusConflict, map[string]string{"codigo": "ESTADO_NO_PERMITE_OPERACION"})
	cliente := NewOperacionesClient(srv.URL)
	for i := 0; i < 8; i++ {
		var c ErrRechazo
		if _, err := cliente.Cancelar(context.Background(), "org-1", "clave-0001", "sol-1", "m"); !errors.As(err, &c) {
			t.Fatalf("llamada %d: se esperaba el conflicto, se obtuvo %v", i+1, err)
		}
	}
	if n := llamadas.Load(); n != 8 {
		t.Fatalf("un 409 no abre el breaker: llegaron %d de 8 llamadas", n)
	}
}

func TestErrorDeRedEsNoDisponible(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close() // nadie escucha en esa dirección
	if _, err := NewOperacionesClient(url).Cancelar(context.Background(), "org-1", "clave-0001", "sol-1", "m"); !errors.Is(err, ErrNoDisponible) {
		t.Fatalf("un error de red tiene que ser ErrNoDisponible: %v", err)
	}
}

// servidorConSecuencia responde con la i-ésima función en el pedido i (la última se repite) y
// guarda los headers y el cuerpo de cada pedido recibido.
type pedidoRecibido struct {
	claveIdem string
	cuerpo    string
}

func servidorConSecuencia(t *testing.T, respuestas ...http.HandlerFunc) (*httptest.Server, *[]pedidoRecibido, *sync.Mutex) {
	t.Helper()
	var mu sync.Mutex
	var recibidos []pedidoRecibido
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		crudo, _ := io.ReadAll(r.Body)
		mu.Lock()
		i := len(recibidos)
		recibidos = append(recibidos, pedidoRecibido{claveIdem: r.Header.Get("Idempotency-Key"), cuerpo: string(crudo)})
		mu.Unlock()
		respuestas[min(i, len(respuestas)-1)](w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, &recibidos, &mu
}

func responder(estado int, cuerpo any) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(estado)
		_ = json.NewEncoder(w).Encode(cuerpo)
	}
}

func TestReintentaUn503YDespuesResponde(t *testing.T) {
	srv, recibidos, mu := servidorConSecuencia(t,
		responder(http.StatusServiceUnavailable, map[string]string{}),
		responder(http.StatusCreated, models.VistaSolicitud{ID: "sol-1", Estado: "ACEPTADA", Version: 1}))
	pedido := models.PedidoTraslado{Cosa: models.Cosa{Descripcion: "cajas", Cantidad: 2}}
	vista, err := NewOperacionesClient(srv.URL).Reservar(context.Background(), "org-1", "clave-0001", pedido)
	if err != nil || vista.ID != "sol-1" {
		t.Fatalf("vista = %+v, err = %v", vista, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(*recibidos) != 2 {
		t.Fatalf("se esperaban 2 pedidos, llegaron %d", len(*recibidos))
	}
	primero, segundo := (*recibidos)[0], (*recibidos)[1]
	if primero.claveIdem != "clave-0001" || segundo.claveIdem != primero.claveIdem {
		t.Fatalf("el reintento tiene que llevar la misma Idempotency-Key: %q y %q", primero.claveIdem, segundo.claveIdem)
	}
	if primero.cuerpo == "" || segundo.cuerpo != primero.cuerpo {
		t.Fatalf("el reintento tiene que llevar el mismo cuerpo: %q y %q", primero.cuerpo, segundo.cuerpo)
	}
}

func TestReintentaUnaDisponibilidadQueTardaDemasiado(t *testing.T) {
	srv, recibidos, mu := servidorConSecuencia(t,
		func(w http.ResponseWriter, r *http.Request) {
			// Más que el timeout por intento (1 s); se corta antes si el cliente ya se fue.
			select {
			case <-time.After(1500 * time.Millisecond):
			case <-r.Context().Done():
			}
		},
		responder(http.StatusOK, models.Disponibilidad{Fecha: "2026-11-20", Franjas: []models.FranjaDisponible{{Disponible: true}}}))
	d, err := NewOperacionesClient(srv.URL).Disponibilidad(context.Background(), "2026-11-20", nil)
	if err != nil || len(d.Franjas) != 1 {
		t.Fatalf("d = %+v, err = %v", d, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(*recibidos) != 2 {
		t.Fatalf("se esperaban 2 pedidos, llegaron %d", len(*recibidos))
	}
}

func TestAgotaLosTresIntentos(t *testing.T) {
	srv, llamadas := servidorQueResponde(t, http.StatusServiceUnavailable, map[string]string{})
	inicio := time.Now()
	_, err := NewOperacionesClient(srv.URL).Reservar(context.Background(), "org-1", "clave-0001", models.PedidoTraslado{})
	duracion := time.Since(inicio)
	if !errors.Is(err, ErrNoDisponible) {
		t.Fatalf("se esperaba ErrNoDisponible, se obtuvo %v", err)
	}
	if n := llamadas.Load(); n != IntentosOperaciones {
		t.Fatalf("se esperaban %d pedidos, llegaron %d", IntentosOperaciones, n)
	}
	if duracion >= TopeReintentos {
		t.Fatalf("los intentos tienen que terminar antes del tope de %s: tardaron %s", TopeReintentos, duracion)
	}
}

func TestNoReintentaUn4xx(t *testing.T) {
	srv, llamadas := servidorQueResponde(t, http.StatusConflict, map[string]string{"codigo": "SIN_DISPONIBILIDAD"})
	_, err := NewOperacionesClient(srv.URL).Reservar(context.Background(), "org-1", "clave-0001", models.PedidoTraslado{})
	var r ErrRechazo
	if !errors.As(err, &r) {
		t.Fatalf("se esperaba ErrRechazo, se obtuvo %v", err)
	}
	if n := llamadas.Load(); n != 1 {
		t.Fatalf("un 4xx no se reintenta: llegaron %d pedidos", n)
	}
}

func TestNoReintentaUn500(t *testing.T) {
	srv, llamadas := servidorQueResponde(t, http.StatusInternalServerError, map[string]string{})
	_, err := NewOperacionesClient(srv.URL).Reservar(context.Background(), "org-1", "clave-0001", models.PedidoTraslado{})
	if !errors.Is(err, ErrNoDisponible) {
		t.Fatalf("se esperaba ErrNoDisponible, se obtuvo %v", err)
	}
	if n := llamadas.Load(); n != 1 {
		t.Fatalf("un 500 no se reintenta: llegaron %d pedidos", n)
	}
}

func TestNoReintentaConElContextoCancelado(t *testing.T) {
	srv, llamadas := servidorQueResponde(t, http.StatusServiceUnavailable, map[string]string{})
	ctx, cancelar := context.WithCancel(context.Background())
	cancelar()
	if _, err := NewOperacionesClient(srv.URL).Reservar(ctx, "org-1", "clave-0001", models.PedidoTraslado{}); err == nil {
		t.Fatal("con el contexto cancelado se esperaba un error")
	}
	if n := llamadas.Load(); n > 1 {
		t.Fatalf("con el contexto cancelado no se reintenta: llegaron %d pedidos", n)
	}
}
