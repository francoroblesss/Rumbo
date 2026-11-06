package identidad

import (
	"crypto/subtle"
	"log/slog"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"

	"github.com/francoroblesss/Rumbo/servicios/comun/problema"
)

// HeaderInterno lleva el secreto compartido entre el gateway y los servicios
// (docs/interfaces-internas.md §11.2, ADR-026). El gateway lo agrega a todo lo que reenvía y
// borra el que mande el cliente; las llamadas entre servicios lo agregan con
// observabilidad.ClienteInterno.
const HeaderInterno = "X-Rumbo-Interno"

// VariableSecretoInterno es la variable de entorno con el secreto.
const VariableSecretoInterno = "SECRETO_INTERNO"

// rutaSalud no exige el secreto: la consultan los healthchecks y el gateway al despertar la
// cadena (§11.3), y no devuelve datos.
const rutaSalud = "/salud"

// ExigirSecretoInterno rechaza con 401 toda request que no traiga el secreto interno. Es el
// "canal confiable diseñado" del teórico 6: en Render cada servicio tiene URL pública, así que
// no alcanza con que solo se llegue a ellos por el gateway; sin el secreto, cualquiera podría
// mandar X-Rumbo-Roles: operador directo al servicio y los headers de identidad no valdrían nada.
//
// Con SECRETO_INTERNO vacío (tests unitarios) no exige nada y lo avisa una vez al arrancar.
func ExigirSecretoInterno() gin.HandlerFunc {
	secreto := []byte(os.Getenv(VariableSecretoInterno))
	if len(secreto) == 0 {
		slog.Warn("secreto interno deshabilitado", slog.String("variable", VariableSecretoInterno))
		return func(c *gin.Context) { c.Next() }
	}
	return func(c *gin.Context) {
		if c.Request.URL.Path == rutaSalud {
			c.Next()
			return
		}
		// Comparación en tiempo constante: no revela por el tiempo de respuesta cuántos bytes acertó.
		if subtle.ConstantTimeCompare([]byte(c.GetHeader(HeaderInterno)), secreto) != 1 {
			problema.Escribir(c, http.StatusUnauthorized, "NO_AUTENTICADO", "No autenticado",
				"El pedido no llegó por el punto de entrada de Rumbo.")
			return
		}
		c.Next()
	}
}
