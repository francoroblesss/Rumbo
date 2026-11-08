package observabilidad

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Salud registra GET y HEAD /salud, que responden 200 mientras el proceso está vivo
// (docs/interfaces-internas.md §11.3). No verifica dependencias a propósito (teórico 6: vivo ≠
// listo): si mirara MySQL o RabbitMQ, una falla ajena sacaría de servicio a un proceso que
// todavía puede atender otras cosas o reconectarse solo (ADR-017).
//
// Hay que llamarla ANTES de Instrumentar y de ExigirSecretoInterno: en Gin un middleware de
// r.Use se aplica a las rutas registradas después, así las sondas no se cuentan en la latencia
// del servicio (como el /health de la demo de la clase 8) ni piden el secreto.
//
// HEAD también, porque es lo que usan por defecto los monitores externos (UptimeRobot en el plan
// gratuito no deja elegir GET) y los mantiene despiertos en Render (ADR-026). Gin no responde
// HEAD con la ruta GET: sin esto, el HEAD seguía de largo y daba 404.
func Salud(r *gin.Engine, servicio string) {
	responder := func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"estado": "ok", "servicio": servicio})
	}
	r.GET("/salud", responder)
	r.HEAD("/salud", responder)
}
