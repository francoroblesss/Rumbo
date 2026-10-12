package problema

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestEscribirRespondeProblemDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	Escribir(c, http.StatusConflict, "ESTADO_NO_PERMITE_OPERACION", "Estado inválido", "venció", ErrorCampo{Campo: "x", Mensaje: "y"})

	if w.Code != http.StatusConflict || w.Header().Get("Content-Type") != TipoContenido {
		t.Fatalf("respuesta incorrecta: %d %s", w.Code, w.Header().Get("Content-Type"))
	}
	var p Problema
	if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	if p.Codigo != "ESTADO_NO_PERMITE_OPERACION" || p.Tipo != "https://rumbo.example/errores/estado-no-permite-operacion" || len(p.Errores) != 1 {
		t.Fatalf("problema incorrecto: %+v", p)
	}
}
