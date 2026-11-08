package controllers

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/francoroblesss/Rumbo/servicios/comun/identidad"
	"github.com/francoroblesss/Rumbo/servicios/comun/problema"
	"github.com/francoroblesss/Rumbo/servicios/identidad/services"
)

// PerfilController expone GET /yo (docs/interfaces-internas.md §15): el perfil del usuario que
// ingresó a la web. Por el gateway es GET /api/identidad/yo. El gateway validó el token y puso
// X-Rumbo-Usuario; acá solo se autoriza por rol (teórico 6: autorización en cada servicio).
type PerfilController struct {
	servicio *services.PerfilService
}

func NuevoPerfilController(servicio *services.PerfilService) *PerfilController {
	return &PerfilController{servicio: servicio}
}

// rolesPerfil son los roles que pueden pedir su perfil (los usuarios de la web).
var rolesPerfil = []string{identidad.RolOperador, identidad.RolAdministrador, identidad.RolCliente}

// Registrar agrega las rutas al router.
func (ctrl *PerfilController) Registrar(r *gin.Engine) {
	r.GET("/yo", ctrl.yo)
}

// yo busca primero al usuario y después mira el rol. El orden importa: §15 pide 404 para el token
// del sistema externo (su sub es un client_id, no un usuario), y ese token solo tiene el rol
// sistema_externo; con identidad.RequiereRol como middleware respondería 403 antes de buscarlo.
// Un usuario que sí existe pero no tiene un rol habilitado recibe 403 SIN_PERMISO, como
// identidad.RequiereRol.
func (ctrl *PerfilController) yo(c *gin.Context) {
	quien := identidad.Desde(c)
	perfil, err := ctrl.servicio.Perfil(c.Request.Context(), quien.Usuario)
	switch {
	case errors.Is(err, services.ErrUsuarioNoEncontrado):
		problema.Escribir(c, http.StatusNotFound, "NO_ENCONTRADA", "Usuario no encontrado",
			"La identidad del token no corresponde a un usuario.")
	case err != nil:
		slog.ErrorContext(c.Request.Context(), "no se pudo leer el perfil", slog.String("error", err.Error()))
		problema.Escribir(c, http.StatusServiceUnavailable, "SERVICIO_NO_DISPONIBLE", "Servicio no disponible",
			"No se pudo procesar el pedido ahora; reintentar más tarde.")
	case !quien.TieneRol(rolesPerfil...):
		problema.Escribir(c, http.StatusForbidden, "SIN_PERMISO", "Sin permiso",
			"La identidad no tiene un rol habilitado para esta operación.")
	default:
		c.JSON(http.StatusOK, perfil)
	}
}
