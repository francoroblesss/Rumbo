package repositories

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/francoroblesss/Rumbo/servicios/identidad/models"
)

// OrganizacionesRepository es el acceso a la colección organizaciones.
type OrganizacionesRepository interface {
	BuscarPorClientID(ctx context.Context, clientID string) (*models.Organizacion, error)
	BuscarPorID(ctx context.Context, id string) (*models.Organizacion, error)
	Crear(ctx context.Context, organizacion *models.Organizacion) error
}

// OrganizacionesMongo implementa OrganizacionesRepository sobre la colección organizaciones.
type OrganizacionesMongo struct {
	coleccion *mongo.Collection
}

// NuevoOrganizacionesMongo crea el repositorio y asegura el índice único sobre el client_id de
// las credenciales de API (dos organizaciones no pueden compartir un client_id).
func NuevoOrganizacionesMongo(ctx context.Context, db *mongo.Database) (*OrganizacionesMongo, error) {
	coleccion := db.Collection("organizaciones")
	_, err := coleccion.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "clientes_api.client_id", Value: 1}},
		Options: options.Index().SetUnique(true).SetSparse(true),
	})
	if err != nil {
		return nil, fmt.Errorf("crear índice único organizaciones.clientes_api.client_id: %w", err)
	}
	return &OrganizacionesMongo{coleccion: coleccion}, nil
}

func (r *OrganizacionesMongo) BuscarPorClientID(ctx context.Context, clientID string) (*models.Organizacion, error) {
	return r.buscar(ctx, bson.D{{Key: "clientes_api.client_id", Value: clientID}})
}

func (r *OrganizacionesMongo) BuscarPorID(ctx context.Context, id string) (*models.Organizacion, error) {
	return r.buscar(ctx, bson.D{{Key: "_id", Value: id}})
}

func (r *OrganizacionesMongo) buscar(ctx context.Context, filtro bson.D) (*models.Organizacion, error) {
	var organizacion models.Organizacion
	err := r.coleccion.FindOne(ctx, filtro).Decode(&organizacion)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNoEncontrado
	}
	if err != nil {
		return nil, fmt.Errorf("buscar organización: %w", err)
	}
	return &organizacion, nil
}

func (r *OrganizacionesMongo) Crear(ctx context.Context, organizacion *models.Organizacion) error {
	_, err := r.coleccion.InsertOne(ctx, organizacion)
	if mongo.IsDuplicateKeyError(err) {
		return ErrDuplicado
	}
	if err != nil {
		return fmt.Errorf("crear organización: %w", err)
	}
	return nil
}

// OrganizacionesMock implementa OrganizacionesRepository en memoria, para los tests del service.
type OrganizacionesMock struct {
	mu    sync.Mutex
	PorID map[string]*models.Organizacion
}

func NuevoOrganizacionesMock() *OrganizacionesMock {
	return &OrganizacionesMock{PorID: map[string]*models.Organizacion{}}
}

func (m *OrganizacionesMock) BuscarPorClientID(_ context.Context, clientID string) (*models.Organizacion, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, organizacion := range m.PorID {
		for _, cliente := range organizacion.ClientesAPI {
			if cliente.ClientID == clientID {
				return organizacion, nil
			}
		}
	}
	return nil, ErrNoEncontrado
}

func (m *OrganizacionesMock) BuscarPorID(_ context.Context, id string) (*models.Organizacion, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	organizacion, ok := m.PorID[id]
	if !ok {
		return nil, ErrNoEncontrado
	}
	return organizacion, nil
}

func (m *OrganizacionesMock) Crear(_ context.Context, organizacion *models.Organizacion) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.PorID[organizacion.ID]; ok {
		return ErrDuplicado
	}
	m.PorID[organizacion.ID] = organizacion
	return nil
}
