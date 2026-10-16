package mysql

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/francoroblesss/Rumbo/servicios/operaciones/puertos"
)

// UnidadDeTrabajo implementa puertos.UnidadDeTrabajo con una transacción de MySQL: el cambio de
// la solicitud, sus eventos (outbox) y las marcas de idempotencia se confirman juntos o no se
// confirma nada (ACID, ADR-003).
type UnidadDeTrabajo struct {
	db *sql.DB
}

var _ puertos.UnidadDeTrabajo = (*UnidadDeTrabajo)(nil)

func NuevaUnidadDeTrabajo(db *sql.DB) *UnidadDeTrabajo {
	return &UnidadDeTrabajo{db: db}
}

// Ejecutar abre la transacción, ejecuta fn y confirma; si fn falla, deshace todo.
func (u *UnidadDeTrabajo) Ejecutar(ctx context.Context, fn func(ctx context.Context, tx puertos.Transaccion) error) error {
	tx, err := u.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("iniciar transacción: %w", err)
	}
	if err := fn(ctx, &transaccion{tx: tx}); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("confirmar transacción: %w", err)
	}
	return nil
}
