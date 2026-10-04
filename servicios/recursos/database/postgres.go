package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Conectar abre un pool de conexiones con PostgreSQL y verifica que responda.
// El servicio usa su propio usuario, que solo puede conectarse a su propia base (ADR-003).
func Conectar(ctx context.Context, uri string) (*pgxpool.Pool, error) {
	ctx, cancelar := context.WithTimeout(ctx, 10*time.Second)
	defer cancelar()

	pool, err := pgxpool.New(ctx, uri)
	if err != nil {
		return nil, fmt.Errorf("crear pool de PostgreSQL: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("verificar conexión con PostgreSQL: %w", err)
	}
	return pool, nil
}
