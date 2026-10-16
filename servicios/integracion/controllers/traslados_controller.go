package controllers

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"github.com/francoroblesss/Rumbo/servicios/comun/identidad"
	"github.com/francoroblesss/Rumbo/servicios/comun/problema"
	"github.com/francoroblesss/Rumbo/servicios/integracion/models"
	"github.com/francoroblesss/Rumbo/servicios/integracion/repositories"
	"github.com/francoroblesss/Rumbo/servicios/integracion/services"
)

const (
	headerIdempotencia = "Idempotency-Key"
	claveOrganizacion  = "organizacion"
)

// TrasladosController atiende el contrato público "Solicitar traslado" v2. Solo valida la forma
// del pedido HTTP (headers, query, cuerpo de las acciones) y traduce los errores del servicio a
// Problem Details; las reglas están en el servicio.
type TrasladosController struct {
	servicio *services.SolicitudesService
}

// NewTrasladosController crea el controlador.
func NewTrasladosController(servicio *services.SolicitudesService) *TrasladosController {
	return &TrasladosController{servicio: servicio}
}

// Registrar agrega las rutas al router. Todas exigen el rol sistema_externo y una organización:
// autenticación en el gateway, autorización acá (teórico 6).
func (tc *TrasladosController) Registrar(router gin.IRouter) {
	grupo := router.Group("/v2/traslados", identidad.RequiereRol(identidad.RolSistemaExterno), requiereOrganizacion)
	grupo.POST("", tc.crear)
	grupo.GET("", tc.listar)
	grupo.GET("/:id", tc.obtener)
	grupo.POST("/:id/respuesta-contrapropuesta", tc.responderContrapropuesta)
	grupo.POST("/:id/cancelacion", tc.cancelar)
}

// requiereOrganizacion corta con 403 si el token no tiene organización: todas las consultas se
// filtran por ella.
func requiereOrganizacion(c *gin.Context) {
	org := identidad.Desde(c).Organizacion
	if org == "" {
		problema.Escribir(c, http.StatusForbidden, "SIN_PERMISO", "Sin permiso",
			"La identidad no tiene una organización asociada.")
		return
	}
	c.Set(claveOrganizacion, org)
	c.Next()
}

func (tc *TrasladosController) crear(c *gin.Context) {
	clave, ok := claveIdempotencia(c)
	if !ok {
		return
	}
	cuerpo, err := io.ReadAll(c.Request.Body)
	if err != nil {
		jsonMalFormado(c, "No se pudo leer el cuerpo del pedido.")
		return
	}
	vista, estado, err := tc.servicio.Crear(c.Request.Context(), c.GetString(claveOrganizacion), clave, cuerpo)
	if err != nil {
		escribirError(c, err)
		return
	}
	c.Header("Location", "/v2/traslados/"+vista.ID)
	c.JSON(estado, aContrato(vista))
}

func (tc *TrasladosController) listar(c *gin.Context) {
	filtro := repositories.FiltroListado{OrganizacionID: c.GetString(claveOrganizacion), Pagina: 1, Tamanio: 20}
	if v, ok := c.GetQuery("actualizado_desde"); ok {
		desde, err := time.Parse(time.RFC3339, v)
		if err != nil {
			jsonMalFormado(c, "actualizado_desde tiene que ser una fecha y hora RFC 3339.")
			return
		}
		desde = desde.UTC()
		filtro.ActualizadoDesde = &desde
	}
	if v, ok := c.GetQuery("estado"); ok {
		if !slices.Contains(models.EstadosSolicitud, v) {
			jsonMalFormado(c, "estado no es uno de los estados del contrato.")
			return
		}
		filtro.Estado = v
	}
	var ok bool
	if filtro.Pagina, ok = enteroQuery(c, "pagina", 1, 1, 0); !ok {
		jsonMalFormado(c, "pagina tiene que ser un entero mayor o igual a 1.")
		return
	}
	if filtro.Tamanio, ok = enteroQuery(c, "tamanio", 20, 1, 100); !ok {
		jsonMalFormado(c, "tamanio tiene que ser un entero entre 1 y 100.")
		return
	}
	pagina, err := tc.servicio.Listar(c.Request.Context(), filtro)
	if err != nil {
		escribirError(c, err)
		return
	}
	c.JSON(http.StatusOK, paginaAContrato(pagina))
}

func (tc *TrasladosController) obtener(c *gin.Context) {
	vista, err := tc.servicio.Obtener(c.Request.Context(), c.GetString(claveOrganizacion), c.Param("id"))
	if err != nil {
		escribirError(c, err)
		return
	}
	c.JSON(http.StatusOK, aContrato(vista))
}

func (tc *TrasladosController) responderContrapropuesta(c *gin.Context) {
	clave, ok := claveIdempotencia(c)
	if !ok {
		return
	}
	var pedido models.RespuestaContrapropuesta
	if !leerJSON(c, &pedido) {
		return
	}
	if pedido.Acepta == nil {
		datosInvalidos(c, problema.ErrorCampo{Campo: "acepta", Mensaje: "es obligatorio"})
		return
	}
	vista, err := tc.servicio.ResponderContrapropuesta(c.Request.Context(), c.GetString(claveOrganizacion), clave, c.Param("id"), *pedido.Acepta)
	if err != nil {
		escribirError(c, err)
		return
	}
	c.JSON(http.StatusOK, aContrato(vista))
}

func (tc *TrasladosController) cancelar(c *gin.Context) {
	clave, ok := claveIdempotencia(c)
	if !ok {
		return
	}
	var pedido models.PedidoCancelacion
	if !leerJSON(c, &pedido) {
		return
	}
	if strings.TrimSpace(pedido.Motivo) == "" {
		datosInvalidos(c, problema.ErrorCampo{Campo: "motivo", Mensaje: "es obligatorio"})
		return
	}
	if utf8.RuneCountInString(pedido.Motivo) > 500 {
		datosInvalidos(c, problema.ErrorCampo{Campo: "motivo", Mensaje: "debe tener como máximo 500 caracteres"})
		return
	}
	vista, err := tc.servicio.Cancelar(c.Request.Context(), c.GetString(claveOrganizacion), clave, c.Param("id"), pedido.Motivo)
	if err != nil {
		escribirError(c, err)
		return
	}
	c.JSON(http.StatusOK, aContrato(vista))
}

// claveIdempotencia lee y valida el header Idempotency-Key (8 a 128 caracteres).
func claveIdempotencia(c *gin.Context) (string, bool) {
	clave := c.GetHeader(headerIdempotencia)
	if n := utf8.RuneCountInString(clave); n < 8 || n > 128 {
		datosInvalidos(c, problema.ErrorCampo{Campo: headerIdempotencia, Mensaje: "es obligatorio y debe tener entre 8 y 128 caracteres"})
		return "", false
	}
	return clave, true
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

func leerJSON(c *gin.Context, destino any) bool {
	if err := json.NewDecoder(c.Request.Body).Decode(destino); err != nil {
		jsonMalFormado(c, "El cuerpo no es JSON válido o un campo tiene un tipo incorrecto.")
		return false
	}
	return true
}

func jsonMalFormado(c *gin.Context, detalle string) {
	problema.Escribir(c, http.StatusBadRequest, "JSON_MAL_FORMADO", "Pedido mal formado", detalle)
}

func datosInvalidos(c *gin.Context, errores ...problema.ErrorCampo) {
	problema.Escribir(c, http.StatusUnprocessableEntity, "DATOS_INVALIDOS", "Datos inválidos",
		"Hay datos que no cumplen el contrato.", errores...)
}

// escribirError traduce un error del servicio a Problem Details. Un 503 lleva Retry-After con
// los segundos que indica el servicio.
func escribirError(c *gin.Context, err error) {
	var e *services.ErrorServicio
	if !errors.As(err, &e) {
		slog.ErrorContext(c.Request.Context(), "error no clasificado", slog.String("error", err.Error()))
		e = &services.ErrorServicio{Estado: http.StatusServiceUnavailable, Codigo: "SERVICIO_NO_DISPONIBLE",
			Titulo: "Servicio no disponible", Detalle: "Rumbo no puede procesar el pedido en este momento."}
	}
	if e.Estado == http.StatusServiceUnavailable && e.ReintentoS > 0 {
		c.Header("Retry-After", strconv.Itoa(e.ReintentoS))
	}
	problema.Escribir(c, e.Estado, e.Codigo, e.Titulo, e.Detalle, e.Errores...)
}
