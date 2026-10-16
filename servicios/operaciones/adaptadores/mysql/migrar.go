package mysql

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"log/slog"

	"github.com/golang-migrate/migrate/v4"
	migratemysql "github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// Las migraciones viajan dentro del binario: el esquema de la base es parte del servicio que es
// su dueño (ADR-003), no un script aparte.
//
//go:embed migraciones/*.sql
var archivosMigraciones embed.FS

// Migrar aplica las migraciones pendientes con golang-migrate. Usa una conexión propia que cierra
// al terminar (el driver de migrate cierra el *sql.DB que recibe). El DSN tiene que tener
// multiStatements=true porque cada archivo trae varias sentencias.
func Migrar(ctx context.Context, dsn string) error {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("abrir conexión para migrar: %w", err)
	}
	fuente, err := iofs.New(archivosMigraciones, "migraciones")
	if err != nil {
		db.Close()
		return fmt.Errorf("leer migraciones: %w", err)
	}
	destino, err := migratemysql.WithInstance(db, &migratemysql.Config{})
	if err != nil {
		db.Close()
		return fmt.Errorf("preparar migraciones: %w", err)
	}
	m, err := migrate.NewWithInstance("iofs", fuente, "mysql", destino)
	if err != nil {
		db.Close()
		return fmt.Errorf("preparar migraciones: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("aplicar migraciones: %w", err)
	}
	version, _, _ := m.Version()
	slog.InfoContext(ctx, "migraciones aplicadas", slog.Uint64("version_esquema", uint64(version)))
	return nil
}
