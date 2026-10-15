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

// Errores que devuelven los repositorios; el service los traduce a reglas de negocio.
var (
	ErrNoEncontrado = errors.New("no encontrado")
	ErrDuplicado    = errors.New("ya existe")
)

// UsuariosRepository es el acceso a la colección usuarios. El service depende de esta interfaz,
// no de MongoDB.
type UsuariosRepository interface {
	BuscarPorEmail(ctx context.Context, email string) (*models.Usuario, error)
	Crear(ctx context.Context, usuario *models.Usuario) error
}

// UsuariosMongo implementa UsuariosRepository sobre la colección usuarios de la base identidad.
type UsuariosMongo struct {
	coleccion *mongo.Collection
}

// NuevoUsuariosMongo crea el repositorio y asegura el índice único sobre el email.
func NuevoUsuariosMongo(ctx context.Context, db *mongo.Database) (*UsuariosMongo, error) {
	coleccion := db.Collection("usuarios")
	_, err := coleccion.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "email", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return nil, fmt.Errorf("crear índice único usuarios.email: %w", err)
	}
	return &UsuariosMongo{coleccion: coleccion}, nil
}

func (r *UsuariosMongo) BuscarPorEmail(ctx context.Context, email string) (*models.Usuario, error) {
	var usuario models.Usuario
	err := r.coleccion.FindOne(ctx, bson.D{{Key: "email", Value: email}}).Decode(&usuario)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNoEncontrado
	}
	if err != nil {
		return nil, fmt.Errorf("buscar usuario por email: %w", err)
	}
	return &usuario, nil
}

func (r *UsuariosMongo) Crear(ctx context.Context, usuario *models.Usuario) error {
	_, err := r.coleccion.InsertOne(ctx, usuario)
	if mongo.IsDuplicateKeyError(err) {
		return ErrDuplicado
	}
	if err != nil {
		return fmt.Errorf("crear usuario: %w", err)
	}
	return nil
}

// UsuariosMock implementa UsuariosRepository en memoria, para los tests del service.
type UsuariosMock struct {
	mu       sync.Mutex
	PorEmail map[string]*models.Usuario
}

func NuevoUsuariosMock() *UsuariosMock {
	return &UsuariosMock{PorEmail: map[string]*models.Usuario{}}
}

func (m *UsuariosMock) BuscarPorEmail(_ context.Context, email string) (*models.Usuario, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	usuario, ok := m.PorEmail[email]
	if !ok {
		return nil, ErrNoEncontrado
	}
	return usuario, nil
}

func (m *UsuariosMock) Crear(_ context.Context, usuario *models.Usuario) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.PorEmail[usuario.Email]; ok {
		return ErrDuplicado
	}
	m.PorEmail[usuario.Email] = usuario
	return nil
}
