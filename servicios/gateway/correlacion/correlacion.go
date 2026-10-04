package correlacion

import (
	"net/http"

	"github.com/google/uuid"
)

// Encabezado es el header que viaja en todas las llamadas internas y en los mensajes (ADR-004).
const Encabezado = "X-Correlation-ID"

// Middleware conserva el correlation id que traiga la request o le asigna uno nuevo (UUID), y lo
// devuelve en la respuesta para poder seguir la operación de punta a punta.
func Middleware(siguiente http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(Encabezado)
		if id == "" {
			id = uuid.NewString()
			r.Header.Set(Encabezado, id)
		}
		w.Header().Set(Encabezado, id)
		siguiente.ServeHTTP(w, r)
	})
}
