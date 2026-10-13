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
type Publicador struct {
	mu         sync.Mutex
	canal      *amqp.Channel
	publicados metric.Int64Counter
}

// NuevoPublicador abre un canal en modo confirmación.
func NuevoPublicador(conexion *amqp.Connection) (*Publicador, error) {
	canal, err := conexion.Channel()
	if err != nil {
		return nil, fmt.Errorf("abrir canal de publicación: %w", err)
	}
	if err := canal.Confirm(false); err != nil {
		return nil, fmt.Errorf("activar confirmaciones: %w", err)
	}
	contador, _ := observabilidad.Medidor().Int64Counter("rumbo_mensajes_publicados_total",
		metric.WithDescription("Mensajes publicados en RabbitMQ"))
	return &Publicador{canal: canal, publicados: contador}, nil
}

// DeclararExchangeTopic declara (de forma idempotente) un exchange topic durable.
func (p *Publicador) DeclararExchangeTopic(nombre string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.canal.ExchangeDeclare(nombre, "topic", true, false, false, false, nil)
}

// DeclararCola declara (de forma idempotente) una cola durable. Sirve para publicar comandos en
// el exchange por defecto ("") con la cola como routing key.
func (p *Publicador) DeclararCola(nombre string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, err := p.canal.QueueDeclare(nombre, true, false, false, false, nil)
	return err
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
	confirmacion, err := p.canal.PublishWithDeferredConfirmWithContext(ctx, exchange, routingKey, false, false,
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
		return fmt.Errorf("publicar %s: %w", s.Tipo, err)
	}
	ok, err := confirmacion.WaitContext(ctx)
	if err != nil {
		return fmt.Errorf("esperar confirmación de %s: %w", s.Tipo, err)
	}
	if !ok {
		return fmt.Errorf("el broker rechazó %s", s.Tipo)
	}
	return nil
}

// Cerrar cierra el canal.
func (p *Publicador) Cerrar() error { return p.canal.Close() }
