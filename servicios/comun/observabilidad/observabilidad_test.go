package observabilidad

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestLogsIncluyenCorrelacion(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(manejadorContexto{slog.NewJSONHandler(&buf, nil)})
	log.InfoContext(ConCorrelacion(context.Background(), "corr-9"), "hola")

	var registro map[string]any
	if err := json.Unmarshal(buf.Bytes(), &registro); err != nil {
		t.Fatal(err)
	}
	if registro["correlation_id"] != "corr-9" || registro["msg"] != "hola" {
		t.Fatalf("log sin correlación: %v", registro)
	}
}

func TestInstrumentarPropagaCorrelacion(t *testing.T) {
	if _, err := Iniciar(context.Background(), "prueba"); err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	Instrumentar(r, "prueba")
	var vista string
	r.GET("/x", func(c *gin.Context) { vista = Correlacion(c.Request.Context()); c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set(HeaderCorrelacion, "corr-1")
	r.ServeHTTP(w, req)
	if vista != "corr-1" || w.Header().Get(HeaderCorrelacion) != "corr-1" {
		t.Fatalf("correlación no propagada: contexto=%q respuesta=%q", vista, w.Header().Get(HeaderCorrelacion))
	}

	sinHeader := httptest.NewRecorder()
	r.ServeHTTP(sinHeader, httptest.NewRequest(http.MethodGet, "/x", nil))
	if sinHeader.Header().Get(HeaderCorrelacion) == "" {
		t.Fatal("sin header de entrada se tiene que generar un correlation id")
	}
}

func TestClienteHTTPPropagaCorrelacion(t *testing.T) {
	var recibido string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recibido = r.Header.Get(HeaderCorrelacion)
	}))
	defer srv.Close()
	req, _ := http.NewRequestWithContext(ConCorrelacion(context.Background(), "corr-2"), http.MethodGet, srv.URL, nil)
	resp, err := ClienteHTTP(0).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if recibido != "corr-2" {
		t.Fatalf("el servidor no recibió la correlación: %q", recibido)
	}
}
