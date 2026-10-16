// Package entrada reúne las dos defensas más externas del gateway: no exponer las rutas
// internas y no confiar en la identidad que declara el cliente (teórico 6).
package entrada

import (
	"net/http"
	"strings"

	"github.com/francoroblesss/Rumbo/servicios/comun/identidad"
	"github.com/francoroblesss/Rumbo/servicios/comun/problema"
	"github.com/francoroblesss/Rumbo/servicios/gateway/metricas"
)

// BloquearInternas responde 404 a todo path que contenga /internas/: son endpoints entre
// servicios (docs/interfaces-internas.md §4) y el gateway no los expone.
func BloquearInternas(siguiente http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/internas/") {
			metricas.Rechazo(r.Context(), metricas.MotivoInterna)
			problema.EscribirHTTP(w, http.StatusNotFound, "NO_ENCONTRADA", "No encontrada", "La ruta no existe.")
			return
		}
		siguiente.ServeHTTP(w, r)
	})
}

// LimpiarIdentidad borra siempre los headers de identidad que mande el cliente. Solo el
// gateway los escribe, después de validar el token: así un servicio puede confiar en ellos.
func LimpiarIdentidad(siguiente http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Del(identidad.HeaderUsuario)
		r.Header.Del(identidad.HeaderOrganizacion)
		r.Header.Del(identidad.HeaderRoles)
		siguiente.ServeHTTP(w, r)
	})
}
