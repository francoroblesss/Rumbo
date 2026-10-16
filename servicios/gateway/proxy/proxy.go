package proxy

import (
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/francoroblesss/Rumbo/servicios/comun/observabilidad"
	"github.com/francoroblesss/Rumbo/servicios/comun/problema"
)

// TiempoMaximo es el deadline total de una request a través del gateway (ADR-004).
const TiempoMaximo = 5 * time.Second

// Nuevo devuelve un reverse proxy hacia destino. Si quitarPrefijo no está vacío, se elimina
// del path antes de reenviar (ej. /api/v1/recursos/choferes → /choferes en Recursos).
// El gateway no reintenta: el reintento le corresponde al cliente (un solo dueño, teórico 7).
// Cada llamada propaga la traza (traceparent) al servicio.
func Nuevo(destino string, quitarPrefijo string) http.Handler {
	u, err := url.Parse(destino)
	if err != nil {
		slog.Error("URL de destino inválida", slog.String("destino", destino), slog.String("error", err.Error()))
		os.Exit(1)
	}

	return &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			if quitarPrefijo != "" {
				r.Out.URL.Path = strings.TrimPrefix(r.In.URL.Path, quitarPrefijo)
				r.Out.URL.RawPath = ""
			}
			r.SetURL(u)
			r.SetXForwarded()
		},
		Transport: otelhttp.NewTransport(&http.Transport{
			DialContext:           (&net.Dialer{Timeout: 2 * time.Second}).DialContext,
			ResponseHeaderTimeout: TiempoMaximo,
		}),
		// El gateway ya puso su X-Correlation-ID en la respuesta (es el mismo valor que propagó):
		// se quita el que devuelve el servicio para no mandarlo dos veces.
		ModifyResponse: func(resp *http.Response) error {
			resp.Header.Del(observabilidad.HeaderCorrelacion)
			return nil
		},
		// 502: no se pudo conectar con el servicio. 504: el servicio no respondió a tiempo. Las dos
		// van en Problem Details, como el resto de los errores del contrato.
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			estado, codigo, titulo, detalle := http.StatusBadGateway, "SERVICIO_NO_DISPONIBLE",
				"Servicio no disponible", "Rumbo no pudo atender el pedido. Reintentar con backoff y la misma Idempotency-Key."
			var errRed net.Error
			if errors.As(err, &errRed) && errRed.Timeout() {
				estado, codigo, titulo, detalle = http.StatusGatewayTimeout, "TIEMPO_AGOTADO",
					"Tiempo agotado", "Rumbo no respondió a tiempo; el pedido pudo haberse procesado. Reintentar con la misma Idempotency-Key."
			}
			slog.ErrorContext(r.Context(), "no se pudo reenviar la request",
				slog.String("metodo", r.Method), slog.String("ruta", r.URL.Path),
				slog.String("destino", destino), slog.Int("estado_http", estado), slog.String("error", err.Error()))
			problema.EscribirHTTP(w, estado, codigo, titulo, detalle)
		},
	}
}
