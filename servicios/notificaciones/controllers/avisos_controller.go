package controllers

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/francoroblesss/Rumbo/servicios/comun/identidad"
	"github.com/francoroblesss/Rumbo/servicios/comun/problema"
	"github.com/francoroblesss/Rumbo/servicios/notificaciones/services"
)

// Intervalos del stream (docs/interfaces-internas.md §14, ADR-027).
const (
	intervaloConsulta = time.Second      // cada cuánto se buscan avisos nuevos
	intervaloPing     = 15 * time.Second // comentario que mantiene viva la conexión a través del gateway
	avisosPorConsulta = 100              // los que no entren salen en la consulta siguiente
)

// AvisosController atiende la bandeja (GET /avisos, POST /avisos/{id}/lectura) y el stream
// (GET /avisos/stream). Solo valida la forma del pedido y traduce los errores a Problem Details.
type AvisosController struct {
	servicio          *services.AvisosService
	intervaloConsulta time.Duration
	intervaloPing     time.Duration
}

// NewAvisosController crea el controlador.
func NewAvisosController(servicio *services.AvisosService) *AvisosController {
	return &AvisosController{servicio: servicio, intervaloConsulta: intervaloConsulta, intervaloPing: intervaloPing}
}

// rolesBandeja son los roles que tienen bandeja: autenticación en el gateway, autorización acá
// (teórico 6).
var rolesBandeja = identidad.RequiereRol(identidad.RolOperador, identidad.RolAdministrador, identidad.RolCliente)

// Registrar agrega las rutas de la bandeja.
func (ac *AvisosController) Registrar(router gin.IRouter) {
	grupo := router.Group("/avisos", rolesBandeja)
	grupo.GET("", ac.listar)
	grupo.POST("/:id/lectura", ac.marcarLeido)
}

// RegistrarStream agrega la ruta del stream. Va aparte porque main.go la registra fuera del
// middleware de métricas (ADR-027): una conexión de stream dura minutos.
func (ac *AvisosController) RegistrarStream(router gin.IRouter) {
	router.GET("/avisos/stream", rolesBandeja, ac.stream)
}

func (ac *AvisosController) listar(c *gin.Context) {
	soloNoLeidos, err := strconv.ParseBool(c.DefaultQuery("no_leidos", "false"))
	pagina, ok1 := enteroQuery(c, "pagina", 1, 1, 0)
	tamanio, ok2 := enteroQuery(c, "tamanio", 20, 1, 100)
	if err != nil || !ok1 || !ok2 {
		problema.Escribir(c, http.StatusUnprocessableEntity, "DATOS_INVALIDOS", "Datos inválidos",
			"no_leidos true o false; pagina desde 1; tamanio de 1 a 100.")
		return
	}
	p, err := ac.servicio.Listar(c.Request.Context(), identidad.Desde(c), soloNoLeidos, pagina, tamanio)
	if err != nil {
		noDisponible(c, err)
		return
	}
	for i := range p.Resultados {
		p.Resultados[i] = p.Resultados[i].ParaRespuesta()
	}
	c.JSON(http.StatusOK, p)
}

func (ac *AvisosController) marcarLeido(c *gin.Context) {
	err := ac.servicio.MarcarLeido(c.Request.Context(), identidad.Desde(c), c.Param("id"))
	if errors.Is(err, services.ErrNoEncontrado) {
		problema.Escribir(c, http.StatusNotFound, "NO_ENCONTRADA", "No encontrado", "El aviso no existe.")
		return
	}
	if err != nil {
		noDisponible(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// noDisponible responde 503 ante una falla de MongoDB: la bandeja depende de su base y lo dice,
// en lugar de responder una lista vacía (teórico 7: degradación honesta).
func noDisponible(c *gin.Context, err error) {
	slog.ErrorContext(c.Request.Context(), "no se pudo acceder a los avisos", slog.String("error", err.Error()))
	problema.Escribir(c, http.StatusServiceUnavailable, "SERVICIO_NO_DISPONIBLE", "Servicio no disponible",
		"Los avisos no están disponibles en este momento.")
}

// enteroQuery lee un parámetro entero de la query con su default y rango (maximo 0 = sin tope).
func enteroQuery(c *gin.Context, nombre string, porDefecto, minimo, maximo int) (int, bool) {
	v, ok := c.GetQuery(nombre)
	if !ok {
		return porDefecto, true
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < minimo || (maximo > 0 && n > maximo) {
		return 0, false
	}
	return n, true
}
