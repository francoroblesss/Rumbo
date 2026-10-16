package controllers

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/francoroblesss/Rumbo/servicios/comun/problema"
	"github.com/francoroblesss/Rumbo/servicios/identidad/services"
)

// AuthController expone el login de los usuarios de la web y el token de los sistemas externos
// (docs/interfaces-internas.md §4.3). Las dos rutas son públicas: el gateway no pide token.
type AuthController struct {
	servicio *services.AuthService
}

func NuevoAuthController(servicio *services.AuthService) *AuthController {
	return &AuthController{servicio: servicio}
}

// Registrar agrega las rutas al router.
func (ctrl *AuthController) Registrar(r *gin.Engine) {
	r.POST("/v2/auth/token", ctrl.tokenCliente)
	r.POST("/login", ctrl.login)
}

type pedidoTokenCliente struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

type pedidoLogin struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (ctrl *AuthController) tokenCliente(c *gin.Context) {
	var pedido pedidoTokenCliente
	if !leerJSON(c, &pedido) {
		return
	}
	var faltantes []problema.ErrorCampo
	if pedido.ClientID == "" {
		faltantes = append(faltantes, problema.ErrorCampo{Campo: "client_id", Mensaje: "es obligatorio"})
	}
	if pedido.ClientSecret == "" {
		faltantes = append(faltantes, problema.ErrorCampo{Campo: "client_secret", Mensaje: "es obligatorio"})
	}
	if len(faltantes) > 0 {
		datosInvalidos(c, faltantes)
		return
	}

	token, err := ctrl.servicio.TokenCliente(c.Request.Context(), pedido.ClientID, pedido.ClientSecret)
	ctrl.responder(c, token, err)
}

func (ctrl *AuthController) login(c *gin.Context) {
	var pedido pedidoLogin
	if !leerJSON(c, &pedido) {
		return
	}
	var faltantes []problema.ErrorCampo
	if pedido.Email == "" {
		faltantes = append(faltantes, problema.ErrorCampo{Campo: "email", Mensaje: "es obligatorio"})
	}
	if pedido.Password == "" {
		faltantes = append(faltantes, problema.ErrorCampo{Campo: "password", Mensaje: "es obligatorio"})
	}
	if len(faltantes) > 0 {
		datosInvalidos(c, faltantes)
		return
	}

	token, err := ctrl.servicio.Login(c.Request.Context(), pedido.Email, pedido.Password)
	ctrl.responder(c, token, err)
}

func (ctrl *AuthController) responder(c *gin.Context, token *services.Token, err error) {
	switch {
	case errors.Is(err, services.ErrCredencialesInvalidas):
		problema.Escribir(c, http.StatusUnauthorized, "NO_AUTENTICADO", "No autenticado",
			"Las credenciales no son válidas.")
	case err != nil:
		slog.ErrorContext(c.Request.Context(), "no se pudo emitir el token", slog.String("error", err.Error()))
		problema.Escribir(c, http.StatusServiceUnavailable, "SERVICIO_NO_DISPONIBLE", "Servicio no disponible",
			"No se pudo procesar el pedido ahora; reintentar más tarde.")
	default:
		c.JSON(http.StatusOK, token)
	}
}

// leerJSON decodifica el cuerpo; si no es JSON válido responde 400 y devuelve false.
func leerJSON(c *gin.Context, destino any) bool {
	if err := c.ShouldBindJSON(destino); err != nil {
		problema.Escribir(c, http.StatusBadRequest, "JSON_MAL_FORMADO", "JSON mal formado",
			"El cuerpo no es un JSON válido.")
		return false
	}
	return true
}

func datosInvalidos(c *gin.Context, errores []problema.ErrorCampo) {
	problema.Escribir(c, http.StatusUnprocessableEntity, "DATOS_INVALIDOS", "Datos inválidos",
		"Falta un dato obligatorio.", errores...)
}
