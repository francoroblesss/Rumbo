package mensajeria

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strconv"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/francoroblesss/Rumbo/servicios/comun/observabilidad"
)

const headerReintentos = "x-reintentos"

// ConfigConsumidor describe una cola de consumo (docs/interfaces-internas.md §5.2).
type ConfigConsumidor struct {
	Cola          string   // nombre de la cola principal
	Exchange      string   // exchange topic del que se suscribe ("" para una cola de comandos)
	Claves        []string // routing keys a vincular (solo si Exchange no es "")
	Prefetch      int      // mensajes sin confirmar por consumidor; 0 = 10
	MaxReintentos int      // reintentos ante error transitorio; 0 = 3
}

// Manejador procesa un mensaje. Si devuelve nil, el mensaje se confirma (ACK). Si devuelve un
// error marcado con Permanente, va a la DLQ. Cualquier otro error se reintenta.
// El efecto tiene que ser idempotente: con entrega at-least-once el mismo mensaje puede llegar
// más de una vez (teórico 4).
type Manejador func(ctx context.Context, s Sobre) error

// Espera devuelve el tiempo antes del reintento número n (0, 1, 2…): ~1 s, ~4 s, ~16 s, con
// ±20 % de jitter para no sincronizar reintentos (teórico 7).
func Espera(n int) time.Duration {
	base := time.Second * time.Duration(1<<(2*n))
	jitter := time.Duration(rand.Int64N(int64(base)*2/5)) - base/5
	return base + jitter
}

// Consumir declara la cola (con su cola de reintento y su DLQ), la vincula al exchange y procesa
// los mensajes hasta que se cancele ctx o se cierre la conexión. El ACK se envía recién después
// de que el manejador terminó el efecto. Devuelve error si el canal se cierra: el servicio debe
// terminar para que Docker lo reinicie.
func Consumir(ctx context.Context, conexion *amqp.Connection, cfg ConfigConsumidor, manejar Manejador) error {
	if cfg.Prefetch == 0 {
		cfg.Prefetch = 10
	}
	if cfg.MaxReintentos == 0 {
		cfg.MaxReintentos = 3
	}
	canal, err := conexion.Channel()
	if err != nil {
		return fmt.Errorf("abrir canal de consumo: %w", err)
	}
	defer canal.Close()

	if err := declararColas(canal, cfg); err != nil {
		return err
	}
	if err := canal.Qos(cfg.Prefetch, 0, false); err != nil {
		return fmt.Errorf("configurar prefetch: %w", err)
	}
	entregas, err := canal.ConsumeWithContext(ctx, cfg.Cola, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("consumir %s: %w", cfg.Cola, err)
	}
	procesados, _ := observabilidad.Medidor().Int64Counter("rumbo_mensajes_procesados_total",
		metric.WithDescription("Mensajes consumidos por resultado"))
	slog.InfoContext(ctx, "consumidor listo", slog.String("cola", cfg.Cola))

	for entrega := range entregas {
		resultado := procesar(ctx, canal, cfg, entrega, manejar)
		procesados.Add(ctx, 1, metric.WithAttributes(attribute.String("cola", cfg.Cola), attribute.String("resultado", resultado)))
	}
	if ctx.Err() != nil {
		return nil
	}
	return errors.New("se cerró el canal de consumo de " + cfg.Cola)
}

func declararColas(canal *amqp.Channel, cfg ConfigConsumidor) error {
	if _, err := canal.QueueDeclare(cfg.Cola, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declarar %s: %w", cfg.Cola, err)
	}
	// Cola de reintento: el mensaje espera su TTL y vuelve a la cola principal.
	if _, err := canal.QueueDeclare(cfg.Cola+".reintento", true, false, false, false, amqp.Table{
		"x-dead-letter-exchange":    "",
		"x-dead-letter-routing-key": cfg.Cola,
	}); err != nil {
		return fmt.Errorf("declarar %s.reintento: %w", cfg.Cola, err)
	}
	if _, err := canal.QueueDeclare(cfg.Cola+".dlq", true, false, false, false, nil); err != nil {
		return fmt.Errorf("declarar %s.dlq: %w", cfg.Cola, err)
	}
	if cfg.Exchange != "" {
		if err := canal.ExchangeDeclare(cfg.Exchange, "topic", true, false, false, false, nil); err != nil {
			return fmt.Errorf("declarar exchange %s: %w", cfg.Exchange, err)
		}
		for _, clave := range cfg.Claves {
			if err := canal.QueueBind(cfg.Cola, clave, cfg.Exchange, false, nil); err != nil {
				return fmt.Errorf("vincular %s a %s (%s): %w", cfg.Cola, cfg.Exchange, clave, err)
			}
		}
	}
	return nil
}

// procesar maneja una entrega y devuelve el resultado para la métrica: ok, reintento o dlq.
func procesar(ctx context.Context, canal *amqp.Channel, cfg ConfigConsumidor, e amqp.Delivery, manejar Manejador) string {
	if e.Headers == nil {
		e.Headers = amqp.Table{}
	}
	ctx = otel.GetTextMapPropagator().Extract(ctx, encabezados(e.Headers))
	if id, ok := e.Headers[HeaderCorrelacion].(string); ok && id != "" {
		ctx = observabilidad.ConCorrelacion(ctx, id)
	}
	ctx, span := observabilidad.Trazador().Start(ctx, "consumir "+e.Type,
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(attribute.String("messaging.system", "rabbitmq"),
			attribute.String("messaging.destination.name", cfg.Cola),
			attribute.String("messaging.message.id", e.MessageId)))
	defer span.End()

	var s Sobre
	err := json.Unmarshal(e.Body, &s)
	if err != nil {
		err = Permanente(fmt.Errorf("sobre inválido: %w", err))
	} else {
		err = manejar(ctx, s)
	}
	if err == nil {
		if errAck := e.Ack(false); errAck != nil {
			slog.ErrorContext(ctx, "no se pudo confirmar el mensaje", slog.String("error", errAck.Error()))
		}
		return "ok"
	}

	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
	reintentos := leerReintentos(e.Headers)
	destino, resultado := cfg.Cola+".dlq", "dlq"
	var expiracion string
	if !EsPermanente(err) && reintentos < cfg.MaxReintentos {
		destino, resultado = cfg.Cola+".reintento", "reintento"
		expiracion = strconv.FormatInt(Espera(reintentos).Milliseconds(), 10)
	}
	slog.WarnContext(ctx, "mensaje no procesado", slog.String("cola", cfg.Cola), slog.String("tipo_mensaje", e.Type),
		slog.Int("reintentos", reintentos), slog.String("destino", destino), slog.String("error", err.Error()))

	headers := amqp.Table{}
	for k, v := range e.Headers {
		headers[k] = v
	}
	headers[headerReintentos] = int32(reintentos + 1)
	headers["x-ultimo-error"] = err.Error()
	errPub := canal.PublishWithContext(ctx, "", destino, false, false, amqp.Publishing{
		ContentType:  e.ContentType,
		DeliveryMode: amqp.Persistent,
		MessageId:    e.MessageId,
		Type:         e.Type,
		Expiration:   expiracion,
		Headers:      headers,
		Body:         e.Body,
	})
	if errPub != nil {
		// No se pudo apartar el mensaje: se devuelve a la cola para no perderlo.
		slog.ErrorContext(ctx, "no se pudo reencolar el mensaje", slog.String("error", errPub.Error()))
		_ = e.Nack(false, true)
		return "reintento"
	}
	_ = e.Ack(false)
	return resultado
}

func leerReintentos(h amqp.Table) int {
	switch v := h[headerReintentos].(type) {
	case int32:
		return int(v)
	case int64:
		return int(v)
	case int:
		return v
	}
	return 0
}
