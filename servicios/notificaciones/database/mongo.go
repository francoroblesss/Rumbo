package database

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Conectar abre la conexión con MongoDB y verifica que responda.
// El servicio usa su propio usuario, que solo tiene permisos sobre su base (ADR-003).
func Conectar(ctx context.Context, uri string) (*mongo.Client, error) {
	ctx, cancelar := context.WithTimeout(ctx, 10*time.Second)
	defer cancelar()

	cliente, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, fmt.Errorf("conectar a MongoDB: %w", err)
	}
	if err := cliente.Ping(ctx, nil); err != nil {
		return nil, fmt.Errorf("verificar conexión con MongoDB: %w", err)
	}
	return cliente, nil
}
