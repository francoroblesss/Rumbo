package puertos

import (
	"context"
	"errors"
	"time"
)

// Búsqueda de traslados (ADR-011, con el alcance de ADR-024; docs/interfaces-internas.md §16).
// El índice es un modelo de lectura derivado de los datos de Operaciones (teórico 5): ayuda a
// descubrir, pero cancelar o avanzar se decide siempre contra MySQL.

// ErrBusquedaNoDisponible: el motor de búsqueda no responde o no está configurado (sin SOLR_URL).
// Solo afecta a la búsqueda: el resto de Operaciones sigue funcionando (teórico 7).
var ErrBusquedaNoDisponible = errors.New("la búsqueda no está disponible")

// Órdenes de la búsqueda (§16).
const (
	OrdenFecha      = "fecha"      // por inicio de la franja, ascendente (por defecto)
	OrdenRelevancia = "relevancia" // por puntaje; sin texto, por fecha
)

// Busqueda son los casos de uso de la búsqueda (puerto de entrada). Los invocan la API de la web
// y el consumidor del indexador.
type Busqueda interface {
	Buscar(ctx context.Context, c ConsultaBusqueda) (PaginaBusqueda, error)
	// IndexarEvento aplica al índice la vista de un evento de §5.4. Es idempotente: un evento
	// repetido o más viejo que lo indexado no cambia nada (versión monotónica).
	IndexarEvento(ctx context.Context, organizacion string, v Vista, ocurridoEn time.Time) error
	// Reindexar reconstruye el índice desde MySQL en segundo plano y responde enseguida.
	Reindexar(ctx context.Context) error
}

// ConsultaBusqueda son los parámetros de GET /traslados/busqueda tal como llegan (§16). Para el
// cliente, Organizacion es siempre la suya (la pone el adaptador HTTP desde la identidad).
type ConsultaBusqueda struct {
	Texto        string
	Organizacion string // "" = todas (solo el personal de Rumbo)
	Estado       string // estado del traslado; "" = todos
	Desde        string // AAAA-MM-DD, día de Argentina, sobre el inicio de la franja; "" = sin límite
	Hasta        string // AAAA-MM-DD, inclusive; "" = sin límite
	SoloAlerta   bool   // alerta=true: solo los que tienen alerta climática
	Orden        string // OrdenFecha (por defecto) u OrdenRelevancia
	Pagina       int
	Tamanio      int
}

// ResultadoBusqueda es un traslado encontrado, con exactamente los campos de §16.
type ResultadoBusqueda struct {
	SolicitudID       string  `json:"solicitud_id"`
	TrasladoID        string  `json:"traslado_id"`
	OrganizacionID    string  `json:"organizacion_id"`
	ReferenciaExterna *string `json:"referencia_externa"`
	Descripcion       string  `json:"descripcion"`
	OrigenNombre      string  `json:"origen_nombre"`
	DestinoNombre     string  `json:"destino_nombre"`
	EstadoTraslado    string  `json:"estado_traslado"`
	FranjaInicio      *string `json:"franja_inicio"` // RFC 3339 con -03:00
	FranjaFin         *string `json:"franja_fin"`
	AlertaClimatica   *string `json:"alerta_climatica"` // NO_OPERABLE | NO_VERIFICADO | null
	CancelacionTardia bool    `json:"cancelacion_tardia"`
}

// PaginaBusqueda es una página de resultados (§11.1).
type PaginaBusqueda struct {
	Resultados []ResultadoBusqueda `json:"resultados"`
	Pagina     int                 `json:"pagina"`
	Tamanio    int                 `json:"tamanio"`
	Total      int                 `json:"total"`
}

// BuscadorTraslados es el índice de búsqueda (puerto de salida). El núcleo no sabe que es Solr,
// como la interfaz Engine de la clase práctica 5: cambiar de motor es cambiar el adaptador.
type BuscadorTraslados interface {
	// Buscar devuelve una página de documentos y el total de los que cumplen el filtro.
	Buscar(ctx context.Context, f FiltroBusqueda) ([]DocumentoTraslado, int, error)
	// Indexar guarda el documento solo si su versión es mayor que la indexada; aplicado = false
	// si ya estaba (evento repetido) o había uno más nuevo (evento desordenado).
	Indexar(ctx context.Context, d DocumentoTraslado) (aplicado bool, err error)
}

// FiltroBusqueda es la consulta ya validada por la aplicación: las fechas llegan como rango UTC
// [FranjaDesde, FranjaHasta) sobre el inicio de la franja, y la página como desde/límite.
type FiltroBusqueda struct {
	Texto        string
	Organizacion string
	Estado       string
	SoloAlerta   bool
	FranjaDesde  *time.Time
	FranjaHasta  *time.Time
	Orden        string
	Desde        int
	Limite       int
}

// DocumentoTraslado es lo que se indexa de un traslado: diseñado para la lectura (texto, filtros,
// orden y respuesta), no una copia de las tablas (teórico 5).
type DocumentoTraslado struct {
	SolicitudID       string // id del documento
	TrasladoID        string
	OrganizacionID    string
	ReferenciaExterna *string
	Descripcion       string
	OrigenNombre      string
	OrigenDireccion   string // solo para el texto libre
	DestinoNombre     string
	DestinoDireccion  string // solo para el texto libre
	EstadoTraslado    string
	FranjaInicio      *time.Time
	FranjaFin         *time.Time
	AlertaClimatica   *string
	CancelacionTardia bool
	Version           int64 // agregado_version del último evento aplicado
	ActualizadoEn     time.Time
}
