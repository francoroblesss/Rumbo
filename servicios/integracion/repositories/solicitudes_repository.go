package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/francoroblesss/Rumbo/servicios/integracion/models"
)

// Errores del repositorio.
var (
	ErrClaveExistente  = errors.New("ya existe una solicitud con esa clave de idempotencia")
	ErrNoEncontrada    = errors.New("solicitud no encontrada")
	ErrNoHayPendientes = errors.New("no hay solicitudes pendientes de envío")
)

// DuracionReclamo es cuánto tiempo una réplica se reserva una solicitud pendiente para enviarla.
// Si la réplica cae o no logra publicar, el reclamo vence y otra la reintenta (ADR-009).
const DuracionReclamo = 30 * time.Second

// FiltroListado son los parámetros de GET /v2/traslados.
type FiltroListado struct {
	OrganizacionID   string
	ActualizadoDesde *time.Time // exclusivo
	Estado           string
	Pagina, Tamanio  int
}

// SolicitudesRepository es el acceso a la colección solicitudes_externas. El servicio depende de
// esta interfaz, no de MongoDB: así se le pueden poner delante la caché (decorador) y un mock.
type SolicitudesRepository interface {
	// Crear inserta la solicitud. Si ya existe una con la misma (organizacion_id, idempotencia.clave),
	// devuelve ErrClaveExistente junto con la solicitud existente.
	Crear(ctx context.Context, s models.SolicitudExterna) (models.SolicitudExterna, error)
	BuscarPorID(ctx context.Context, organizacionID, id string) (models.SolicitudExterna, error) // ErrNoEncontrada
	Listar(ctx context.Context, f FiltroListado) ([]models.SolicitudExterna, int64, error)
	// ReclamarPendiente marca atómicamente UNA solicitud con pendiente_envio=true cuyo reclamo esté
	// vencido o vacío: reclamado_hasta = ahora+30s, reclamado_por = instancia. ErrNoHayPendientes si no hay.
	ReclamarPendiente(ctx context.Context, instancia string, ahora time.Time) (models.SolicitudExterna, error)
	MarcarEnviada(ctx context.Context, id string) error // pendiente_envio=false, borra el reclamo
	// AplicarVista reemplaza vista, version y actualizado_en SOLO si version > la guardada.
	// Devuelve (organizacion_id, aplicado). Un evento viejo o repetido devuelve aplicado=false sin error.
	AplicarVista(ctx context.Context, id string, version int64, vista models.VistaSolicitud, ahora time.Time) (string, bool, error)
	ContarPendientes(ctx context.Context) (int64, error)
}
