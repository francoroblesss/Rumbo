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
	BuscarPorID(ctx context.Context, id string) (*models.Usuario, error)
	Crear(ctx context.Context, usuario *models.Usuario) error
	// CompletarNombre pone el nombre solo si el usuario no tiene uno: nunca pisa uno existente.
	CompletarNombre(ctx context.Context, id, nombre string) error
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
	return r.buscar(ctx, bson.D{{Key: "email", Value: email}})
}

func (r *UsuariosMongo) BuscarPorID(ctx context.Context, id string) (*models.Usuario, error) {
	return r.buscar(ctx, bson.D{{Key: "_id", Value: id}})
}

func (r *UsuariosMongo) buscar(ctx context.Context, filtro bson.D) (*models.Usuario, error) {
	var usuario models.Usuario
	err := r.coleccion.FindOne(ctx, filtro).Decode(&usuario)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNoEncontrado
	}
	if err != nil {
		return nil, fmt.Errorf("buscar usuario: %w", err)
	}
	return &usuario, nil
}

func (r *UsuariosMongo) CompletarNombre(ctx context.Context, id, nombre string) error {
	// La condición va en el filtro (y no en un "leer y después escribir") para que dos réplicas
	// arrancando a la vez no pisen un nombre que otra ya puso.
	_, err := r.coleccion.UpdateOne(ctx, filtroSinNombre(id), bson.D{{Key: "$set", Value: bson.D{{Key: "nombre", Value: nombre}}}})
	if err != nil {
		return fmt.Errorf("completar nombre del usuario: %w", err)
	}
	return nil
}

// filtroSinNombre encuentra el documento con ese _id solo si no tiene nombre (campo ausente o vacío).
func filtroSinNombre(id string) bson.D {
	return bson.D{
		{Key: "_id", Value: id},
		{Key: "$or", Value: bson.A{
			bson.D{{Key: "nombre", Value: bson.D{{Key: "$exists", Value: false}}}},
			bson.D{{Key: "nombre", Value: ""}},
		}},
	}
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

func (m *UsuariosMock) BuscarPorID(_ context.Context, id string) (*models.Usuario, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, usuario := range m.PorEmail {
		if usuario.ID == id {
			return usuario, nil
		}
	}
	return nil, ErrNoEncontrado
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

func (m *UsuariosMock) CompletarNombre(_ context.Context, id, nombre string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, usuario := range m.PorEmail {
		if usuario.ID == id && usuario.Nombre == "" {
			usuario.Nombre = nombre
		}
	}
	return nil
}
