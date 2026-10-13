// Package mensajeria implementa la comunicación asíncrona de Rumbo sobre RabbitMQ con
// github.com/rabbitmq/amqp091-go, según ADR-004 y docs/interfaces-internas.md §5:
// sobre versionado, publicación con confirmación, propagación de trazas, ACK después del
// efecto, reintentos con backoff y jitter, y DLQ.
package mensajeria

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/francoroblesss/Rumbo/servicios/comun/observabilidad"
)

// Sobre es la forma de todo mensaje de Rumbo (contrato versionado, teórico 4).
type Sobre struct {
	ID              string          `json:"id"`
	Tipo            string          `json:"tipo"`
	Version         int             `json:"version"`
	OcurridoEn      time.Time       `json:"ocurrido_en"`
	CorrelationID   string          `json:"correlation_id"`
	AgregadoID      string          `json:"agregado_id"`
	AgregadoVersion int64           `json:"agregado_version"`
	Datos           json.RawMessage `json:"datos"`
}

// NuevoSobre arma un sobre (versión 1) con un id nuevo y el correlation id del contexto.
func NuevoSobre(ctx context.Context, tipo, agregadoID string, agregadoVersion int64, datos any) (Sobre, error) {
	crudo, err := json.Marshal(datos)
	if err != nil {
		return Sobre{}, fmt.Errorf("serializar datos de %s: %w", tipo, err)
	}
	return Sobre{
		ID:              uuid.NewString(),
		Tipo:            tipo,
		Version:         1,
		OcurridoEn:      time.Now(),
		CorrelationID:   observabilidad.Correlacion(ctx),
		AgregadoID:      agregadoID,
		AgregadoVersion: agregadoVersion,
		Datos:           crudo,
	}, nil
}

// LeerDatos decodifica los datos del sobre en destino.
func (s Sobre) LeerDatos(destino any) error {
	if err := json.Unmarshal(s.Datos, destino); err != nil {
		return Permanente(fmt.Errorf("datos inválidos en %s: %w", s.Tipo, err))
	}
	return nil
}

// errorPermanente marca un error que no se arregla reintentando (payload inválido, versión no
// soportada): el mensaje va directo a la DLQ.
type errorPermanente struct{ err error }

func (e errorPermanente) Error() string { return e.err.Error() }
func (e errorPermanente) Unwrap() error { return e.err }

// Permanente envuelve un error para que el consumidor no reintente el mensaje.
func Permanente(err error) error { return errorPermanente{err: err} }

// EsPermanente indica si un error fue marcado como permanente.
func EsPermanente(err error) bool {
	var p errorPermanente
	return errors.As(err, &p)
}
