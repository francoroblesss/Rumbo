package puertos

import (
	"context"
	"errors"
	"time"
)

// Búsqueda de traslados (ADR-011, docs/interfaces-internas.md §16). El índice es una vista
// derivada de los datos de Operaciones (teórico 5): se arma con los eventos y se puede
// reconstruir desde MySQL. El resultado de una búsqueda nunca habilita una acción: cancelar o
// avanzar se valida siempre contra MySQL.

// ErrBusquedaNoDisponible: el motor de búsqueda no responde (o no está configurado). Ninguna otra
// operación depende de él: solo la búsqueda responde 503.
var ErrBusquedaNoDisponible = errors.New("la búsqueda no está disponible")

// Busqueda son los casos de uso de la búsqueda (puerto de entrada). Los invocan el adaptador HTTP
// (búsqueda y reindexación) y el consumidor de eventos (indexador).
type Busqueda interface {
	// Buscar devuelve una página de traslados. dominio.ErrDatosInvalidos si un filtro no es válido.
	Buscar(ctx context.Context, c ConsultaBusqueda) (PaginaBusqueda, error)
	// IndexarEvento aplica al índice la vista que trae un evento (§5.4). Es idempotente: una
	// versión igual o más vieja que la indexada no cambia nada.
	IndexarEvento(ctx context.Context, organizacion string, v Vista, ocurridoEn time.Time) error
	// Reindexar recorre MySQL y vuelve a indexar todo en segundo plano. ErrBusquedaNoDisponible si
	// no hay motor configurado; si ya hay una reindexación en curso, no arranca otra.
	Reindexar(ctx context.Context) error
}

// ConsultaBusqueda es lo que pide la web, ya con la organización resuelta: para el cliente es
// siempre la suya (la pone el adaptador desde la identidad del gateway, nunca de un parámetro).
type ConsultaBusqueda struct {
	Texto         string // texto libre; "" = todos
	Organizacion  string // "" = todas (solo el personal de Rumbo)
	Estado        string // estado del traslado; "" = todos
	Desde         string // AAAA-MM-DD sobre el inicio de la franja; "" = sin límite
	Hasta         string // AAAA-MM-DD, inclusive
	SoloConAlerta bool
	Orden         string // "fecha" (por defecto) o "relevancia"
	Pagina        int
	Tamanio       int
}

// ResultadoBusqueda es un traslado tal como lo devuelve la búsqueda (§16).
type ResultadoBusqueda struct {
	SolicitudID       string  `json:"solicitud_id"`
	TrasladoID        string  `json:"traslado_id"`
	OrganizacionID    string  `json:"organizacion_id"`
	ReferenciaExterna *string `json:"referencia_externa"`
	Descripcion       string  `json:"descripcion"`
	OrigenNombre      string  `json:"origen_nombre"`
	DestinoNombre     string  `json:"destino_nombre"`
	EstadoTraslado    string  `json:"estado_traslado"`
	FranjaInicio      *string `json:"franja_inicio"`
	FranjaFin         *string `json:"franja_fin"`
	AlertaClimatica   *string `json:"alerta_climatica"` // NO_OPERABLE, NO_VERIFICADO o null
	CancelacionTardia bool    `json:"cancelacion_tardia"`
}

// PaginaBusqueda es una página de resultados (§11.1).
type PaginaBusqueda struct {
	Resultados []ResultadoBusqueda `json:"resultados"`
	Pagina     int                 `json:"pagina"`
	Tamanio    int                 `json:"tamanio"`
	Total      int                 `json:"total"`
}

// BuscadorTraslados es el motor de búsqueda (puerto de salida). Lo implementa adaptadores/solr;
// el núcleo no sabe que es Solr (como la interfaz Engine de la clase práctica 5).
type BuscadorTraslados interface {
	// Buscar devuelve los documentos de una página y el total. ErrBusquedaNoDisponible si falla.
	Buscar(ctx context.Context, f FiltroBusqueda) ([]DocumentoTraslado, int, error)
	// Indexar guarda el documento solo si su versión es mayor que la indexada (comparación y
	// escritura atómicas). Devuelve false si no hizo falta (versión igual o más vieja).
	Indexar(ctx context.Context, d DocumentoTraslado) (bool, error)
}

// FiltroBusqueda es la consulta ya validada y traducida a valores (las fechas en UTC).
type FiltroBusqueda struct {
	Texto          string
	Organizacion   string
	EstadoTraslado string
	FranjaDesde    *time.Time // inclusive
	FranjaHasta    *time.Time // exclusive
	SoloConAlerta  bool
	PorRelevancia  bool // con Texto: por puntaje; si no, por inicio de la franja
	Desde, Limite  int
}

// DocumentoTraslado es el documento indexado: diseñado para la lectura (texto, filtros y orden),
// no una copia de las tablas (teórico 5).
type DocumentoTraslado struct {
	SolicitudID       string
	TrasladoID        string
	OrganizacionID    string
	ReferenciaExterna string
	Descripcion       string
	OrigenNombre      string
	OrigenDireccion   string
	DestinoNombre     string
	DestinoDireccion  string
	EstadoTraslado    string
	FranjaInicio      *time.Time
	FranjaFin         *time.Time
	AlertaClimatica   string // "" = sin alerta
	CancelacionTardia bool
	Version           int64 // agregado_version del último cambio aplicado
	ActualizadoEn     time.Time
}
