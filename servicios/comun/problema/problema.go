// Package problema escribe errores HTTP en formato Problem Details (RFC 9457), con el campo
// "codigo" estable que usan el contrato público y los endpoints internos
// (docs/interfaces-internas.md §3).
package problema

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// TipoContenido es el Content-Type de Problem Details.
const TipoContenido = "application/problem+json"

// ErrorCampo detalla un dato inválido (se usa con el código DATOS_INVALIDOS).
type ErrorCampo struct {
	Campo   string `json:"campo"`
	Mensaje string `json:"mensaje"`
}

// Problema es el cuerpo de una respuesta de error.
type Problema struct {
	Tipo    string       `json:"type"`
	Titulo  string       `json:"title"`
	Estado  int          `json:"status"`
	Codigo  string       `json:"codigo"`
	Detalle string       `json:"detail,omitempty"`
	Errores []ErrorCampo `json:"errores,omitempty"`
}

// Nuevo arma un Problema. El "type" se deriva del código: ESTADO_NO_PERMITE_OPERACION →
// https://rumbo.example/errores/estado-no-permite-operacion.
func Nuevo(estado int, codigo, titulo, detalle string, errores ...ErrorCampo) Problema {
	return Problema{
		Tipo:    "https://rumbo.example/errores/" + strings.ReplaceAll(strings.ToLower(codigo), "_", "-"),
		Titulo:  titulo,
		Estado:  estado,
		Codigo:  codigo,
		Detalle: detalle,
		Errores: errores,
	}
}

// Escribir responde el error desde un handler de Gin y corta la cadena de middlewares.
func Escribir(c *gin.Context, estado int, codigo, titulo, detalle string, errores ...ErrorCampo) {
	c.Header("Content-Type", TipoContenido)
	c.AbortWithStatusJSON(estado, Nuevo(estado, codigo, titulo, detalle, errores...))
}

// EscribirHTTP responde el error desde un handler de net/http (lo usa el gateway).
func EscribirHTTP(w http.ResponseWriter, estado int, codigo, titulo, detalle string) {
	w.Header().Set("Content-Type", TipoContenido)
	w.WriteHeader(estado)
	_ = json.NewEncoder(w).Encode(Nuevo(estado, codigo, titulo, detalle))
}
