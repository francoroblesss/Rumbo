package mensajeria

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/francoroblesss/Rumbo/servicios/comun/observabilidad"
)

// HeaderCorrelacion es el header AMQP con el correlation id.
const HeaderCorrelacion = "x-correlation-id"

// TiempoConfirmacion es lo máximo que se espera la confirmación del broker al publicar.
const TiempoConfirmacion = 5 * time.Second

// Publicador publica sobres en RabbitMQ con confirmación del broker (publisher confirms): un
// Publicar sin error significa que el broker guardó el mensaje. Es seguro para usar desde varias
// goroutines.
//
// El canal se abre cuando hace falta y se descarta ante cualquier falla, así el publicador se
// recupera solo cuando la Conexion reconecta. Mientras el broker no está, Publicar devuelve
// error y el relay deja el mensaje pendiente en el outbox (ADR-004).
type Publicador struct {
	mu            sync.Mutex
	conexion      *Conexion
	canal         *amqp.Channel
	declaraciones []func(*amqp.Channel) error // exchanges y colas a declarar en cada canal nuevo
	publicados    metric.Int64Counter
}

// NuevoPublicador crea el publicador. No necesita que el broker esté disponible.
func NuevoPublicador(conexion *Conexion) *Publicador {
	contador, _ := observabilidad.Medidor().Int64Counter("rumbo_mensajes_publicados_total",
		metric.WithDescription("Mensajes publicados en RabbitMQ"))
	return &Publicador{conexion: conexion, publicados: contador}
}

// DeclararExchangeTopic registra un exchange topic durable, que se declara (de forma idempotente)
// en cada canal que se abra. Si hay conexión, lo declara ya y devuelve el error del broker.
func (p *Publicador) DeclararExchangeTopic(nombre string) error {
	return p.declarar(func(c *amqp.Channel) error {
		return c.ExchangeDeclare(nombre, "topic", true, false, false, false, nil)
	})
}

// DeclararCola registra una cola durable, que se declara (de forma idempotente) en cada canal
// que se abra. Sirve para publicar comandos en el exchange por defecto ("") con la cola como
// routing key. Si hay conexión, la declara ya y devuelve el error del broker.
func (p *Publicador) DeclararCola(nombre string) error {
	return p.declarar(func(c *amqp.Channel) error {
		_, err := c.QueueDeclare(nombre, true, false, false, false, nil)
		return err
	})
}

func (p *Publicador) declarar(d func(*amqp.Channel) error) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.declaraciones = append(p.declaraciones, d)
	if p.canal == nil || p.canal.IsClosed() {
		return nil // se declara al abrir el próximo canal
	}
	return d(p.canal)
}

// canalAbierto devuelve el canal en modo confirmación, abriéndolo si hace falta. Se llama con
// p.mu tomado.
func (p *Publicador) canalAbierto() (*amqp.Channel, error) {
	if p.canal != nil && !p.canal.IsClosed() {
		return p.canal, nil
	}
	p.canal = nil
	conexion, err := p.conexion.Actual()
	if err != nil {
		return nil, err
	}
	canal, err := conexion.Channel()
	if err != nil {
		return nil, fmt.Errorf("abrir canal de publicación: %w", err)
	}
	if err := canal.Confirm(false); err != nil {
		_ = canal.Close()
		return nil, fmt.Errorf("activar confirmaciones: %w", err)
	}
	for _, d := range p.declaraciones {
		if err := d(canal); err != nil {
			_ = canal.Close()
			return nil, fmt.Errorf("declarar en el canal de publicación: %w", err)
		}
	}
	p.canal = canal
	return canal, nil
}

// descartar cierra el canal si sigue siendo el actual, para que la próxima publicación abra otro.
func (p *Publicador) descartar(canal *amqp.Channel) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.canal == canal {
		_ = canal.Close()
		p.canal = nil
	}
}

// Publicar envía el sobre al exchange con la routing key indicada (para un comando: exchange ""
// y routing key = nombre de la cola) y espera la confirmación del broker.
func (p *Publicador) Publicar(ctx context.Context, exchange, routingKey string, s Sobre) (err error) {
	ctx, span := observabilidad.Trazador().Start(ctx, "publicar "+s.Tipo,
		trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(attribute.String("messaging.system", "rabbitmq"),
			attribute.String("messaging.destination.name", exchange+"/"+routingKey),
			attribute.String("messaging.message.id", s.ID)))
	defer func() {
		resultado := "ok"
		if err != nil {
			resultado = "error"
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		p.publicados.Add(ctx, 1, metric.WithAttributes(attribute.String("exchange", exchange),
			attribute.String("tipo", s.Tipo), attribute.String("resultado", resultado)))
		span.End()
	}()

	cuerpo, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("serializar sobre %s: %w", s.Tipo, err)
	}
	headers := amqp.Table{HeaderCorrelacion: s.CorrelationID}
	otel.GetTextMapPropagator().Inject(ctx, encabezados(headers))

	ctx, cancelar := context.WithTimeout(ctx, TiempoConfirmacion)
	defer cancelar()

	p.mu.Lock()
	canal, err := p.canalAbierto()
	if err != nil {
		p.mu.Unlock()
		return fmt.Errorf("publicar %s: %w", s.Tipo, err)
	}
	confirmacion, err := canal.PublishWithDeferredConfirmWithContext(ctx, exchange, routingKey, false, false,
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			MessageId:    s.ID,
			Type:         s.Tipo,
			Timestamp:    s.OcurridoEn,
			Headers:      headers,
			Body:         cuerpo,
		})
	p.mu.Unlock()
	if err != nil {
		p.descartar(canal)
		return fmt.Errorf("publicar %s: %w", s.Tipo, err)
	}
	ok, err := confirmacion.WaitContext(ctx)
	if err != nil {
		p.descartar(canal)
		return fmt.Errorf("esperar confirmación de %s: %w", s.Tipo, err)
	}
	if !ok {
		p.descartar(canal)
		return fmt.Errorf("el broker rechazó %s", s.Tipo)
	}
	return nil
}

// Cerrar cierra el canal, si hay uno abierto.
func (p *Publicador) Cerrar() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.canal != nil {
		_ = p.canal.Close()
		p.canal = nil
	}
}
