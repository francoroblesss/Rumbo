package api

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/francoroblesss/Rumbo/servicios/comun/identidad"
	"github.com/francoroblesss/Rumbo/servicios/comun/problema"
	"github.com/francoroblesss/Rumbo/servicios/operaciones/puertos"
)

// HandlerClima es el adaptador de entrada HTTP del clima actual del aeropuerto para la web
// (docs/interfaces-internas.md §13.5, ADR-034). No conoce al proveedor: habla con el puerto.
type HandlerClima struct {
	casos puertos.ClimaActual
}

func NuevoHandlerClima(casos puertos.ClimaActual) *HandlerClima {
	return &HandlerClima{casos: casos}
}

// Registrar agrega la ruta. Llega por el gateway con el prefijo /api/operaciones.
func (h *HandlerClima) Registrar(r gin.IRouter) {
	r.GET("/clima",
		identidad.RequiereRol(identidad.RolOperador, identidad.RolAdministrador, identidad.RolCliente), h.consultar)
}

func (h *HandlerClima) consultar(c *gin.Context) {
	actual, err := h.casos.Consultar(c.Request.Context())
	if errors.Is(err, puertos.ErrClimaNoDisponible) {
		// Fallback honesto (teórico 7): se dice que no se sabe, no se inventa un clima.
		// 30 s = lo que tarda el breaker del adaptador de clima en dejar probar de nuevo
		// (TiempoAbierto), igual que el 503 de Integración (§3).
		c.Header("Retry-After", "30")
		problema.Escribir(c, http.StatusServiceUnavailable, "SERVICIO_NO_DISPONIBLE", "Clima no disponible",
			"No se pudo consultar el clima del aeropuerto. Reintentar en unos minutos.")
		return
	}
	if err != nil {
		slog.ErrorContext(c.Request.Context(), "error al consultar el clima para la web", slog.String("error", err.Error()))
		problema.Escribir(c, http.StatusServiceUnavailable, "SERVICIO_NO_DISPONIBLE", "Servicio no disponible",
			"Operaciones no puede procesar el pedido ahora. Reintentar más tarde.")
		return
	}
	c.JSON(http.StatusOK, actual)
}
