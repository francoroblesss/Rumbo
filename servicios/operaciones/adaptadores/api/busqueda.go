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

// HandlerBusqueda es el adaptador de entrada HTTP de la búsqueda (docs/interfaces-internas.md
// §16). Como en la clase práctica 5, no conoce Solr: valida y traduce parámetros y errores.
type HandlerBusqueda struct {
	casos puertos.Busqueda
}

func NuevoHandlerBusqueda(casos puertos.Busqueda) *HandlerBusqueda {
	return &HandlerBusqueda{casos: casos}
}

// Registrar agrega las rutas. Llegan por el gateway con el prefijo /api/operaciones.
func (h *HandlerBusqueda) Registrar(r gin.IRouter) {
	conCliente := identidad.RequiereRol(identidad.RolOperador, identidad.RolAdministrador, identidad.RolCliente)
	r.GET("/traslados/busqueda", conCliente, h.buscar)
	r.POST("/admin/reindexacion", identidad.RequiereRol(identidad.RolAdministrador), h.reindexar)
}

func (h *HandlerBusqueda) buscar(c *gin.Context) {
	// Al cliente se le fuerza su organización (CA-25): sale de la identidad del gateway y el
	// parámetro se ignora. El personal de Rumbo puede filtrar por cualquiera.
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
	var alerta bool
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
		Desde: c.Query("desde"), Hasta: c.Query("hasta"), SoloConAlerta: alerta,
		Orden: c.Query("orden"), Pagina: pagina, Tamanio: tamanio,
	})
	if errors.Is(err, dominio.ErrDatosInvalidos) {
		datosInvalidos(c, "estado, desde, hasta, orden, pagina o tamanio",
			"estado PROGRAMADO, EN_CURSO, COMPLETADO o CANCELADO; desde y hasta AAAA-MM-DD; orden fecha o relevancia; pagina desde 1; tamanio de 1 a 100")
		return
	}
	if err != nil {
		busquedaNoDisponible(c, err)
		return
	}
	c.JSON(http.StatusOK, p)
}

func (h *HandlerBusqueda) reindexar(c *gin.Context) {
	if err := h.casos.Reindexar(c.Request.Context()); err != nil {
		busquedaNoDisponible(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"estado": "en_curso"})
}

// busquedaNoDisponible responde 503 cuando el motor no responde (ADR-011): solo se cae la
// búsqueda, y la web lo muestra sin afectar el resto.
func busquedaNoDisponible(c *gin.Context, err error) {
	slog.WarnContext(c.Request.Context(), "búsqueda no disponible", slog.String("error", err.Error()))
	problema.Escribir(c, http.StatusServiceUnavailable, "SERVICIO_NO_DISPONIBLE", "Búsqueda no disponible",
		"La búsqueda no está disponible en este momento. El resto de Rumbo funciona igual; reintentar más tarde.")
}
