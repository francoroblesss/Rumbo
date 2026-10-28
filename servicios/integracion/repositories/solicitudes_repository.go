package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/francoroblesss/Rumbo/servicios/integracion/models"
)

// Errores del repositorio.
var (
	ErrClaveExistente = errors.New("ya existe una solicitud con esa clave de idempotencia")
	ErrNoEncontrada   = errors.New("solicitud no encontrada")
)

// FiltroListado son los parámetros de GET /traslados.
type FiltroListado struct {
	OrganizacionID   string
	ActualizadoDesde *time.Time // exclusivo
	Estado           string     // estado del traslado
	Pagina, Tamanio  int
}

// SolicitudesRepository es el acceso a la colección solicitudes_externas. El servicio depende de
// esta interfaz, no de MongoDB: así se le pueden poner delante la caché (decorador) y un mock.
type SolicitudesRepository interface {
	// Crear inserta la solicitud. Si ya existe una con la misma (organizacion_id, idempotencia.clave),
	// devuelve ErrClaveExistente junto con la solicitud existente.
	Crear(ctx context.Context, s models.SolicitudExterna) (models.SolicitudExterna, error)
	BuscarPorID(ctx context.Context, organizacionID, id string) (models.SolicitudExterna, error)       // ErrNoEncontrada
	BuscarPorClave(ctx context.Context, organizacionID, clave string) (models.SolicitudExterna, error) // ErrNoEncontrada
	Listar(ctx context.Context, f FiltroListado) ([]models.SolicitudExterna, int64, error)
	// AplicarVista reemplaza vista, version y actualizado_en SOLO si version > la guardada.
	// Devuelve (organizacion_id, aplicado). Un evento viejo o repetido devuelve aplicado=false sin error.
	AplicarVista(ctx context.Context, id string, version int64, vista models.VistaSolicitud, ahora time.Time) (string, bool, error)
}
