// Package autenticacion valida en el borde el token firmado de cada request (firma, vigencia,
// emisor y audiencia) y responde 401 si es inválido (teórico 6, ADR-002). Si es válido,
// propaga la identidad a los servicios en los headers X-Rumbo-* (docs/interfaces-internas.md §2).
// La validación la hace la librería github.com/golang-jwt/jwt/v5.
package autenticacion

import (
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"

	"github.com/francoroblesss/Rumbo/servicios/comun/identidad"
	"github.com/francoroblesss/Rumbo/servicios/comun/problema"
	"github.com/francoroblesss/Rumbo/servicios/gateway/metricas"
)

// Datos que Identidad pone en el token y que el gateway exige.
const (
	emisorEsperado    = "rumbo-identidad"
	audienciaEsperada = "rumbo"
)

type reclamos struct {
	Org   string   `json:"org,omitempty"`
	Roles []string `json:"roles"`
	jwt.RegisteredClaims
}

// Middleware exige un token válido en todas las rutas salvo las públicas (login y token del
// sistema externo). Con token válido pone X-Rumbo-Usuario, X-Rumbo-Organizacion y X-Rumbo-Roles.
func Middleware(secreto string) func(http.Handler) http.Handler {
	clave := []byte(secreto)
	return func(siguiente http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if esPublica(r) {
				siguiente.ServeHTTP(w, r)
				return
			}

			r2, ok := autenticar(r, clave)
			if !ok {
				metricas.Rechazo(r.Context(), metricas.MotivoNoAutenticado)
				w.Header().Set("WWW-Authenticate", "Bearer")
				problema.EscribirHTTP(w, http.StatusUnauthorized, "NO_AUTENTICADO", "No autenticado",
					"Falta el token, está vencido o es inválido.")
				return
			}
			siguiente.ServeHTTP(w, r2)
		})
	}
}

// esPublica indica las rutas que no piden token (docs/interfaces-internas.md §2).
func esPublica(r *http.Request) bool {
	if r.Method != http.MethodPost {
		return false
	}
	return r.URL.Path == "/auth/token" || r.URL.Path == "/api/identidad/login"
}

// autenticar valida el token del header Authorization y devuelve la request con la identidad
// en los headers. El gateway ya borró los que mandó el cliente (entrada.LimpiarIdentidad).
func autenticar(r *http.Request, clave []byte) (*http.Request, bool) {
	esquema, token, encontrado := strings.Cut(r.Header.Get("Authorization"), " ")
	token = strings.TrimSpace(token)
	if !encontrado || !strings.EqualFold(esquema, "Bearer") || token == "" {
		return nil, false
	}

	var datos reclamos
	_, err := jwt.ParseWithClaims(token, &datos, func(*jwt.Token) (any, error) { return clave, nil },
		jwt.WithValidMethods([]string{"HS256"}),
		jwt.WithIssuer(emisorEsperado),
		jwt.WithAudience(audienciaEsperada),
		jwt.WithExpirationRequired())
	if err != nil || datos.Subject == "" {
		return nil, false
	}

	r.Header.Set(identidad.HeaderUsuario, datos.Subject)
	if datos.Org != "" {
		r.Header.Set(identidad.HeaderOrganizacion, datos.Org)
	}
	r.Header.Set(identidad.HeaderRoles, strings.Join(datos.Roles, ","))
	return r, true
}
