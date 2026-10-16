package mensajeria

import (
	"context"
	"errors"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel/propagation"
)

// EsperaMaximaReconexion es el tope del backoff entre intentos de conexión a RabbitMQ.
const EsperaMaximaReconexion = 30 * time.Second

// ErrSinConexion indica que en este momento no hay conexión con RabbitMQ.
var ErrSinConexion = errors.New("RabbitMQ no disponible")

// Conexion mantiene una conexión con RabbitMQ y la vuelve a abrir cada vez que se cae.
//
// amqp091-go no reconecta solo: su documentación deja la reconexión a cargo de la aplicación
// (escuchar NotifyClose y volver a abrir). Esto es ese pegamento mínimo. Así la caída del broker
// no termina el proceso: el servicio sigue atendiendo HTTP con lo que no depende del broker
// (teórico 6: vivo ≠ listo ≠ dependencia degradada; teórico 7: contener la perturbación) y los
// pendientes del outbox se publican cuando vuelve (ADR-004).
type Conexion struct {
	uri    string
	mu     sync.Mutex
	actual *amqp.Connection
	cambio chan struct{} // se cierra cada vez que cambia actual
	fin    context.CancelFunc
}

// Conectar devuelve enseguida y se conecta en segundo plano, reintentando con backoff
// exponencial y jitter (1 s, 2 s, 4 s… hasta 30 s) mientras ctx siga vivo (teórico 7). El
// servicio no espera al broker para arrancar.
func Conectar(ctx context.Context, uri string) *Conexion {
	ctx, fin := context.WithCancel(ctx)
	c := &Conexion{uri: uri, cambio: make(chan struct{}), fin: fin}
	go c.mantener(ctx)
	return c
}

func (c *Conexion) mantener(ctx context.Context) {
	for intento := 0; ctx.Err() == nil; {
		conexion, err := amqp.Dial(c.uri)
		if err != nil {
			espera := esperaReconexion(intento)
			intento++
			slog.Warn("RabbitMQ no responde, se reintenta",
				slog.Int("intento", intento), slog.Duration("espera", espera), slog.String("error", err.Error()))
			dormir(ctx, espera)
			continue
		}
		intento = 0
		cerrada := conexion.NotifyClose(make(chan *amqp.Error, 1))
		c.reemplazar(conexion)
		slog.Info("conectado a RabbitMQ")

		select {
		case <-ctx.Done():
			c.reemplazar(nil)
			_ = conexion.Close()
			return
		case motivo := <-cerrada:
			c.reemplazar(nil)
			if motivo != nil {
				slog.Warn("se cerró la conexión con RabbitMQ, se reconecta", slog.String("error", motivo.Error()))
			}
		}
	}
}

func (c *Conexion) reemplazar(conexion *amqp.Connection) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.actual = conexion
	close(c.cambio)
	c.cambio = make(chan struct{})
}

// Actual devuelve la conexión abierta, o ErrSinConexion si en este momento no hay.
func (c *Conexion) Actual() (*amqp.Connection, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.actual == nil || c.actual.IsClosed() {
		return nil, ErrSinConexion
	}
	return c.actual, nil
}

// Esperar bloquea hasta que haya una conexión abierta o hasta que se cancele ctx.
func (c *Conexion) Esperar(ctx context.Context) (*amqp.Connection, error) {
	for {
		c.mu.Lock()
		conexion, cambio := c.actual, c.cambio
		c.mu.Unlock()
		if conexion != nil && !conexion.IsClosed() {
			return conexion, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-cambio:
		}
	}
}

// Cerrar deja de reconectar y cierra la conexión actual.
func (c *Conexion) Cerrar() { c.fin() }

// esperaReconexion: 1 s, 2 s, 4 s… con tope de 30 s, más hasta 50 % de jitter para que varias
// réplicas no golpeen al broker al mismo tiempo.
func esperaReconexion(intento int) time.Duration {
	espera := EsperaMaximaReconexion
	if intento < 5 {
		espera = time.Second << intento
	}
	return espera + time.Duration(rand.Int64N(int64(espera/2)))
}

// dormir espera d o hasta que se cancele ctx.
func dormir(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

// encabezados adapta los headers AMQP al propagador de OpenTelemetry, para que la traza viaje
// dentro del mensaje (traceparent / tracestate).
type encabezados amqp.Table

var _ propagation.TextMapCarrier = encabezados{}

func (e encabezados) Get(clave string) string {
	if v, ok := e[clave].(string); ok {
		return v
	}
	return ""
}

func (e encabezados) Set(clave, valor string) { e[clave] = valor }

func (e encabezados) Keys() []string {
	claves := make([]string, 0, len(e))
	for k := range e {
		claves = append(claves, k)
	}
	return claves
}
