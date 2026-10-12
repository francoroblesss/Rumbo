// Package identidad lee la identidad que el gateway propaga en headers después de validar el
// token (teórico 6: autenticación centralizada en el borde, autorización en cada servicio).
// Contrato en docs/interfaces-internas.md §2.
package identidad

import (
	"net/http"
	"slices"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/francoroblesss/Rumbo/servicios/comun/problema"
)

// Headers que el gateway reemplaza en cada request (y borra si los manda el cliente).
const (
	HeaderUsuario      = "X-Rumbo-Usuario"
	HeaderOrganizacion = "X-Rumbo-Organizacion"
	HeaderRoles        = "X-Rumbo-Roles"
)

// Roles posibles.
const (
	RolCliente        = "cliente"
	RolOperador       = "operador"
	RolChofer         = "chofer"
	RolAdministrador  = "administrador"
	RolSistemaExterno = "sistema_externo"
)

// Identidad es quién hace la request, según el gateway.
type Identidad struct {
	Usuario      string
	Organizacion string
	Roles        []string
}

// TieneRol indica si la identidad tiene alguno de los roles pedidos.
func (i Identidad) TieneRol(roles ...string) bool {
	for _, r := range roles {
		if slices.Contains(i.Roles, r) {
			return true
		}
	}
	return false
}

// DesdeHeaders lee la identidad de los headers de una request.
func DesdeHeaders(h http.Header) Identidad {
	var roles []string
	for _, r := range strings.Split(h.Get(HeaderRoles), ",") {
		if r = strings.TrimSpace(r); r != "" {
			roles = append(roles, r)
		}
	}
	return Identidad{
		Usuario:      h.Get(HeaderUsuario),
		Organizacion: h.Get(HeaderOrganizacion),
		Roles:        roles,
	}
}

// Desde lee la identidad de la request de Gin.
func Desde(c *gin.Context) Identidad {
	return DesdeHeaders(c.Request.Header)
}

// RequiereRol corta con 403 SIN_PERMISO si la identidad no tiene ninguno de los roles.
func RequiereRol(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !Desde(c).TieneRol(roles...) {
			problema.Escribir(c, http.StatusForbidden, "SIN_PERMISO", "Sin permiso",
				"La identidad no tiene un rol habilitado para esta operación.")
			return
		}
		c.Next()
	}
}
