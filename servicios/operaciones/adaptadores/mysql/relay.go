package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"

	"github.com/francoroblesss/Rumbo/servicios/comun/mensajeria"
	"github.com/francoroblesss/Rumbo/servicios/comun/observabilidad"
)

// IntervaloRelay es cada cuánto el relay busca eventos pendientes en el outbox.
const IntervaloRelay = 500 * time.Millisecond

// LoteRelay es cuántos eventos publica por vuelta.
const LoteRelay = 100

// Publicador es lo que el relay necesita del broker (lo cumple *mensajeria.Publicador).
type Publicador interface {
	Publicar(ctx context.Context, exchange, routingKey string, s mensajeria.Sobre) error
}

// Relay publica en RabbitMQ los eventos guardados en el outbox (patrón outbox, teórico 2 y 4).
// El cambio de negocio y su evento se confirmaron juntos en MySQL; el relay los publica después.
// Si el servicio cae entre medio, el evento sigue en el outbox y se publica al volver. Como puede
// publicar dos veces (cae después de publicar y antes de marcar), los consumidores son idempotentes.
type Relay struct {
	db         *sql.DB
	publicador Publicador
}

// NuevoRelay arma el relay y registra el gauge rumbo_outbox_pendientes.
func NuevoRelay(db *sql.DB, publicador Publicador) (*Relay, error) {
	r := &Relay{db: db, publicador: publicador}
	atributos := metric.WithAttributes(attribute.String("servicio", "operaciones"))
	_, err := observabilidad.Medidor().Int64ObservableGauge("rumbo_outbox_pendientes",
		metric.WithDescription("Eventos del outbox todavía no publicados"),
		metric.WithInt64Callback(func(ctx context.Context, o metric.Int64Observer) error {
			var pendientes int64
			if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM outbox WHERE publicado_en IS NULL`).Scan(&pendientes); err != nil {
				return err
			}
			o.Observe(pendientes, atributos)
			return nil
		}))
	if err != nil {
		return nil, fmt.Errorf("registrar métrica del outbox: %w", err)
	}
	return r, nil
}

// Ejecutar publica cada IntervaloRelay hasta que se cancele ctx.
func (r *Relay) Ejecutar(ctx context.Context) {
	ticker := time.NewTicker(IntervaloRelay)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := r.PublicarPendientes(ctx); err != nil && ctx.Err() == nil {
				slog.ErrorContext(ctx, "el relay del outbox no pudo publicar", slog.String("error", err.Error()))
			}
		}
	}
}

// PublicarPendientes publica una tanda en una transacción. Las filas se toman con FOR UPDATE
// SKIP LOCKED: si hubiera otra réplica, no publica las mismas (ADR-013). Ante el primer error se
// corta la tanda: se confirma lo publicado y el resto queda pendiente, así se respeta el orden.
func (r *Relay) PublicarPendientes(ctx context.Context) (int, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("iniciar transacción del relay: %w", err)
	}
	defer tx.Rollback()

	filas, err := tx.QueryContext(ctx, `SELECT id, exchange_nombre, routing_key, sobre, contexto_traza FROM outbox
		WHERE publicado_en IS NULL ORDER BY id LIMIT ? FOR UPDATE SKIP LOCKED`, LoteRelay)
	if err != nil {
		return 0, fmt.Errorf("leer outbox: %w", err)
	}
	type pendiente struct {
		id                   int64
		exchange, routingKey string
		sobre, traza         []byte
	}
	var lote []pendiente
	for filas.Next() {
		var p pendiente
		if err := filas.Scan(&p.id, &p.exchange, &p.routingKey, &p.sobre, &p.traza); err != nil {
			filas.Close()
			return 0, fmt.Errorf("leer fila del outbox: %w", err)
		}
		lote = append(lote, p)
	}
	filas.Close()
	if err := filas.Err(); err != nil {
		return 0, fmt.Errorf("leer outbox: %w", err)
	}

	publicados := 0
	var errPublicar error
	for _, p := range lote {
		var sobre mensajeria.Sobre
		if err := json.Unmarshal(p.sobre, &sobre); err != nil {
			errPublicar = fmt.Errorf("sobre inválido en el outbox (id %d): %w", p.id, err)
			break
		}
		// La publicación continúa la traza del pedido que generó el evento.
		ctxMensaje := ctx
		if len(p.traza) > 0 {
			traza := propagation.MapCarrier{}
			if json.Unmarshal(p.traza, &traza) == nil {
				ctxMensaje = otel.GetTextMapPropagator().Extract(ctx, traza)
			}
		}
		if sobre.CorrelationID != "" {
			ctxMensaje = observabilidad.ConCorrelacion(ctxMensaje, sobre.CorrelationID)
		}
		if err := r.publicador.Publicar(ctxMensaje, p.exchange, p.routingKey, sobre); err != nil {
			errPublicar = err
			break
		}
		if _, err := tx.ExecContext(ctx, `UPDATE outbox SET publicado_en = ? WHERE id = ?`, time.Now().UTC(), p.id); err != nil {
			errPublicar = fmt.Errorf("marcar publicado: %w", err)
			break
		}
		publicados++
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("confirmar el relay: %w", err)
	}
	return publicados, errPublicar
}
