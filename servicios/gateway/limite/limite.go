// Package limite implementa el control de tráfico por organización (token bucket) y responde
// 429 con Retry-After al superarlo (teórico 6, ADR-002). El token bucket es el de la librería
// golang.org/x/time/rate; acá solo se administra uno por clave.
package limite

import (
	"context"
	"math"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/francoroblesss/Rumbo/servicios/comun/identidad"
	"github.com/francoroblesss/Rumbo/servicios/comun/problema"
	"github.com/francoroblesss/Rumbo/servicios/gateway/metricas"
)

// Límite de la capacidad publicada: 60 pedidos por minuto, con ráfagas de hasta 10
// (docs/interfaces-internas.md §2).
const (
	rafaga = 10

	// Las claves sin uso se descartan para que el mapa no crezca sin límite.
	intervaloLimpieza = time.Minute
	inactividadMaxima = 10 * time.Minute
)

type cubo struct {
	limitador *rate.Limiter
	ultimoUso time.Time
}

// Limitador mantiene un token bucket por clave (organización, usuario o IP).
type Limitador struct {
	mu    sync.Mutex
	cubos map[string]*cubo
}

func Nuevo() *Limitador {
	return &Limitador{cubos: map[string]*cubo{}}
}

// Permitir consume un token de la clave. Si no hay, devuelve false y cuánto falta esperar.
func (l *Limitador) Permitir(clave string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	c, ok := l.cubos[clave]
	if !ok {
		c = &cubo{limitador: rate.NewLimiter(rate.Every(time.Second), rafaga)}
		l.cubos[clave] = c
	}
	c.ultimoUso = time.Now()

	reserva := c.limitador.Reserve()
	if espera := reserva.Delay(); espera > 0 {
		reserva.Cancel()
		return false, espera
	}
	return true, 0
}

// Limpiar descarta cada minuto las claves sin uso hace más de 10 minutos, hasta que el
// contexto termine. Se lanza en su propia goroutine.
func (l *Limitador) Limpiar(ctx context.Context) {
	reloj := time.NewTicker(intervaloLimpieza)
	defer reloj.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case ahora := <-reloj.C:
			l.descartarInactivas(ahora)
		}
	}
}

func (l *Limitador) descartarInactivas(ahora time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for clave, c := range l.cubos {
		if ahora.Sub(c.ultimoUso) > inactividadMaxima {
			delete(l.cubos, clave)
		}
	}
}

// Middleware aplica el límite. Va después de la autenticación: la clave sale de la identidad
// que ya propagó (organización, o usuario si no tiene) y, en las rutas públicas, de la IP.
func Middleware(l *Limitador) func(http.Handler) http.Handler {
	return func(siguiente http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			permitido, espera := l.Permitir(clave(r))
			if !permitido {
				metricas.Rechazo(r.Context(), metricas.MotivoLimite)
				segundos := max(1, int(math.Ceil(espera.Seconds())))
				w.Header().Set("Retry-After", strconv.Itoa(segundos))
				problema.EscribirHTTP(w, http.StatusTooManyRequests, "DEMASIADOS_PEDIDOS", "Demasiados pedidos",
					"Se superó el límite de 60 pedidos por minuto; reintentar después de Retry-After segundos.")
				return
			}
			siguiente.ServeHTTP(w, r)
		})
	}
}

// clave elige a quién se le cuenta el pedido: la organización del token; si no tiene, el
// usuario (sub); y en las rutas públicas, la IP del cliente (solo auxiliar, teórico 6).
func clave(r *http.Request) string {
	if org := r.Header.Get(identidad.HeaderOrganizacion); org != "" {
		return org
	}
	if usuario := r.Header.Get(identidad.HeaderUsuario); usuario != "" {
		return usuario
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	return "ip:" + ip
}
