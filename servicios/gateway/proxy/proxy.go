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
		// 502: no se pudo conectar con el servicio. 504: el servicio no respondió a tiempo.
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			estado := http.StatusBadGateway
			var errRed net.Error
			if errors.As(err, &errRed) && errRed.Timeout() {
				estado = http.StatusGatewayTimeout
			}
			slog.ErrorContext(r.Context(), "no se pudo reenviar la request",
				slog.String("metodo", r.Method), slog.String("ruta", r.URL.Path),
				slog.String("destino", destino), slog.Int("estado_http", estado), slog.String("error", err.Error()))
			w.WriteHeader(estado)
		},
	}
}
