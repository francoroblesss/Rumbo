// Package metricas registra las métricas propias del gateway (docs/interfaces-internas.md §8,
// ADR-014): cuántos pedidos rechaza y por qué motivo.
package metricas

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/francoroblesss/Rumbo/servicios/comun/observabilidad"
)

// Motivos de rechazo (valores de la etiqueta "motivo").
const (
	MotivoNoAutenticado = "no_autenticado"
	MotivoLimite        = "limite"
	MotivoInterna       = "interna"
)

var rechazos, _ = observabilidad.Medidor().Int64Counter("rumbo_gateway_rechazos_total",
	metric.WithDescription("Pedidos rechazados por el gateway, por motivo"))

// Rechazo cuenta un pedido rechazado por el gateway.
func Rechazo(ctx context.Context, motivo string) {
	rechazos.Add(ctx, 1, metric.WithAttributes(attribute.String("motivo", motivo)))
}
