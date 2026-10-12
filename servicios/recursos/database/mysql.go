package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	// Driver de MySQL para database/sql (ADR-005): se registra con el nombre "mysql".
	_ "github.com/go-sql-driver/mysql"
)

// Conectar abre el pool de conexiones con MySQL y verifica que responda.
// El servicio usa su propio usuario, que solo puede acceder a su propia base (ADR-003, ADR-005).
// El DSN tiene la forma usuario:clave@tcp(host:3306)/base?parseTime=true.
func Conectar(ctx context.Context, dsn string) (*sql.DB, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("configurar conexión con MySQL: %w", err)
	}
	db.SetMaxOpenConns(20)
	db.SetConnMaxLifetime(5 * time.Minute)

	ctx, cancelar := context.WithTimeout(ctx, 10*time.Second)
	defer cancelar()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("verificar conexión con MySQL: %w", err)
	}
	return db, nil
}
