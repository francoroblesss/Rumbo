package api

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/francoroblesss/Rumbo/servicios/comun/identidad"
	"github.com/francoroblesss/Rumbo/servicios/comun/problema"
	"github.com/francoroblesss/Rumbo/servicios/operaciones/dominio"
	"github.com/francoroblesss/Rumbo/servicios/operaciones/puertos"
)

// HandlerBusqueda es el adaptador de entrada HTTP de la búsqueda de traslados
// (docs/interfaces-internas.md §16). No conoce Solr: habla con el puerto Busqueda.
type HandlerBusqueda struct {
	casos puertos.Busqueda
}

func NuevoHandlerBusqueda(casos puertos.Busqueda) *HandlerBusqueda {
	return &HandlerBusqueda{casos: casos}
}

// Registrar agrega las rutas. Llegan por el gateway con el prefijo /api/operaciones.
func (h *HandlerBusqueda) Registrar(r gin.IRouter) {
	r.GET("/traslados/busqueda",
		identidad.RequiereRol(identidad.RolOperador, identidad.RolAdministrador, identidad.RolCliente), h.buscar)
	r.POST("/admin/reindexacion", identidad.RequiereRol(identidad.RolAdministrador), h.reindexar)
}

func (h *HandlerBusqueda) buscar(c *gin.Context) {
	// Aislamiento por organización (CA-25): al cliente se le fuerza la suya, tomada de la
	// identidad del gateway; el parámetro organizacion_id solo lo usa el personal de Rumbo.
	org, ok := organizacionWeb(c)
	if !ok {
		return
	}
	if org == "" {
		org = c.Query("organizacion_id")
	}
	pagina, ok1 := entero(c, "pagina", paginaPorDefecto)
	tamanio, ok2 := entero(c, "tamanio", tamanioPorDefecto)
	if !ok1 || !ok2 {
		return
	}
	alerta := false
	switch c.Query("alerta") {
	case "", "false":
	case "true":
		alerta = true
	default:
		datosInvalidos(c, "alerta", "debe ser true o false")
		return
	}
	p, err := h.casos.Buscar(c.Request.Context(), puertos.ConsultaBusqueda{
		Texto: c.Query("q"), Organizacion: org, Estado: c.Query("estado"),
		Desde: c.Query("desde"), Hasta: c.Query("hasta"), SoloAlerta: alerta,
		Orden: c.Query("orden"), Pagina: pagina, Tamanio: tamanio,
	})
	if errors.Is(err, dominio.ErrDatosInvalidos) {
		datosInvalidos(c, "estado, desde, hasta, orden, pagina o tamanio",
			"estado PROGRAMADO, EN_CURSO, COMPLETADO o CANCELADO; desde y hasta AAAA-MM-DD; orden fecha o relevancia; pagina desde 1; tamanio de 1 a 100")
		return
	}
	if err != nil {
		h.error(c, err)
		return
	}
	c.JSON(http.StatusOK, p)
}

func (h *HandlerBusqueda) reindexar(c *gin.Context) {
	if err := h.casos.Reindexar(c.Request.Context()); err != nil {
		h.error(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"estado": "en_curso"})
}

// error traduce las fallas: sin motor de búsqueda, 503 solo para la búsqueda (teórico 7).
func (h *HandlerBusqueda) error(c *gin.Context, err error) {
	ctx := c.Request.Context()
	if errors.Is(err, puertos.ErrBusquedaNoDisponible) {
		slog.WarnContext(ctx, "búsqueda no disponible", slog.String("error", err.Error()))
		problema.Escribir(c, http.StatusServiceUnavailable, "SERVICIO_NO_DISPONIBLE", "Búsqueda no disponible",
			"El motor de búsqueda no responde. El resto de Rumbo funciona igual: reintentar la búsqueda más tarde.")
		return
	}
	slog.ErrorContext(ctx, "error al atender la búsqueda", slog.String("error", err.Error()))
	problema.Escribir(c, http.StatusServiceUnavailable, "SERVICIO_NO_DISPONIBLE", "Servicio no disponible",
		"Operaciones no puede procesar el pedido ahora. Reintentar más tarde.")
}
