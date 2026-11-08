package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/francoroblesss/Rumbo/servicios/notificaciones/models"
)

// Errores del repositorio.
var (
	ErrAvisoExistente = errors.New("el aviso ya estaba registrado")
	ErrNoEncontrado   = errors.New("aviso no encontrado")
)

// Cursor es la posición del último aviso enviado por el stream: los siguientes son los de
// creado_en mayor, o el mismo creado_en con _id mayor (desempate, §14).
type Cursor struct {
	CreadoEn time.Time
	ID       string
}

// AvisosRepository es el acceso a la colección avisos. El servicio depende de esta interfaz, no
// de MongoDB. Todas las lecturas reciben los destinatarios visibles para quien llama: el filtro
// de visibilidad se aplica en la misma consulta (un $or), nunca después de leer.
type AvisosRepository interface {
	// Insertar guarda el aviso. Si ya existe uno con el mismo _id devuelve ErrAvisoExistente.
	Insertar(ctx context.Context, a models.Aviso) error
	// Listar devuelve una página, más nuevos primero, y el total.
	Listar(ctx context.Context, visibles []models.Destinatario, soloNoLeidos bool, pagina, tamanio int) ([]models.Aviso, int64, error)
	Buscar(ctx context.Context, visibles []models.Destinatario, id string) (models.Aviso, error) // ErrNoEncontrado
	MarcarLeido(ctx context.Context, visibles []models.Destinatario, id string) error            // ErrNoEncontrado
	// Posteriores devuelve hasta limite avisos posteriores al cursor, en orden (creado_en, _id).
	Posteriores(ctx context.Context, visibles []models.Destinatario, desde Cursor, limite int) ([]models.Aviso, error)
}
