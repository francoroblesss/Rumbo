package aplicacion

import (
	"context"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/metric"

	"github.com/francoroblesss/Rumbo/servicios/comun/observabilidad"
	"github.com/francoroblesss/Rumbo/servicios/operaciones/dominio"
	"github.com/francoroblesss/Rumbo/servicios/operaciones/puertos"
)

// Búsqueda de traslados (ADR-011 con el alcance de ADR-024, docs/interfaces-internas.md §16).

// paginaReindexacion es cuántas solicitudes se leen de MySQL por vez al reindexar.
const paginaReindexacion = 200

// bucketsDemora son los límites del histograma de demora de indexación, en segundos. Se fijan
// explícitos: los de OpenTelemetry por defecto (0, 5, 10, 25…) están pensados para milisegundos
// y con segundos todo caería en el primer bucket (hallazgo del postmortem). El objetivo de
// ADR-011 es un p95 de 5 s.
var bucketsDemora = []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30, 60}

// Busqueda implementa puertos.Busqueda. buscador es nil si no hay motor configurado (SOLR_URL
// vacío, como en Render, ADR-026): la búsqueda responde 503 y el resto sigue igual.
type Busqueda struct {
	buscador    puertos.BuscadorTraslados
	uow         puertos.UnidadDeTrabajo
	reloj       func() time.Time
	demora      metric.Float64Histogram
	reindexando atomic.Bool
}

var _ puertos.Busqueda = (*Busqueda)(nil)

// NuevaBusqueda arma los casos de uso de la búsqueda.
func NuevaBusqueda(buscador puertos.BuscadorTraslados, uow puertos.UnidadDeTrabajo, reloj func() time.Time) *Busqueda {
	demora, _ := observabilidad.Medidor().Float64Histogram("rumbo_indexacion_demora_segundos",
		metric.WithDescription("Demora entre que ocurrió el cambio en Operaciones y quedó visible en la búsqueda"),
		metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(bucketsDemora...))
	return &Busqueda{buscador: buscador, uow: uow, reloj: reloj, demora: demora}
}

// Buscar valida la consulta y la traduce al filtro del motor. Las fechas son días de Argentina y
// se pasan a un rango UTC sobre el inicio de la franja, como en el listado de la web.
func (b *Busqueda) Buscar(ctx context.Context, c puertos.ConsultaBusqueda) (puertos.PaginaBusqueda, error) {
	if c.Orden == "" {
		c.Orden = "fecha"
	}
	if c.Pagina < 1 || c.Tamanio < 1 || c.Tamanio > TamanioMaximo ||
		(c.Estado != "" && !estadosListables[dominio.EstadoTraslado(c.Estado)]) ||
		(c.Orden != "fecha" && c.Orden != "relevancia") {
		return puertos.PaginaBusqueda{}, dominio.ErrDatosInvalidos
	}
	f := puertos.FiltroBusqueda{
		Texto: strings.TrimSpace(c.Texto), Organizacion: c.Organizacion, EstadoTraslado: c.Estado,
		SoloConAlerta: c.SoloConAlerta, PorRelevancia: c.Orden == "relevancia",
		Desde: (c.Pagina - 1) * c.Tamanio, Limite: c.Tamanio,
	}
	var err error
	if f.FranjaDesde, err = dia(c.Desde, 0); err != nil {
		return puertos.PaginaBusqueda{}, err
	}
	if f.FranjaHasta, err = dia(c.Hasta, 1); err != nil { // hasta inclusive: el día siguiente, exclusive
		return puertos.PaginaBusqueda{}, err
	}
	if b.buscador == nil {
		return puertos.PaginaBusqueda{}, puertos.ErrBusquedaNoDisponible
	}
	docs, total, err := b.buscador.Buscar(ctx, f)
	if err != nil {
		return puertos.PaginaBusqueda{}, err
	}
	p := puertos.PaginaBusqueda{Resultados: make([]puertos.ResultadoBusqueda, 0, len(docs)), Pagina: c.Pagina, Tamanio: c.Tamanio, Total: total}
	for _, d := range docs {
		p.Resultados = append(p.Resultados, resultado(d))
	}
	return p, nil
}

// dia traduce AAAA-MM-DD (hora de Argentina) al instante UTC de ese día más dias. "" = sin límite.
func dia(fecha string, dias int) (*time.Time, error) {
	if fecha == "" {
		return nil, nil
	}
	d, err := time.ParseInLocation(time.DateOnly, fecha, zonaRumbo)
	if err != nil {
		return nil, dominio.ErrDatosInvalidos
	}
	t := d.AddDate(0, 0, dias).UTC()
	return &t, nil
}

func resultado(d puertos.DocumentoTraslado) puertos.ResultadoBusqueda {
	r := puertos.ResultadoBusqueda{
		SolicitudID: d.SolicitudID, TrasladoID: d.TrasladoID, OrganizacionID: d.OrganizacionID,
		Descripcion: d.Descripcion, OrigenNombre: d.OrigenNombre, DestinoNombre: d.DestinoNombre,
		EstadoTraslado: d.EstadoTraslado, FranjaInicio: horaOpcional(d.FranjaInicio), FranjaFin: horaOpcional(d.FranjaFin),
		CancelacionTardia: d.CancelacionTardia,
	}
	if d.ReferenciaExterna != "" {
		r.ReferenciaExterna = &d.ReferenciaExterna
	}
	if d.AlertaClimatica != "" {
		r.AlertaClimatica = &d.AlertaClimatica
	}
	return r
}

// IndexarEvento es el indexador (cola operaciones.indexador): arma el documento con la vista del
// evento y lo aplica solo si su versión es mayor que la indexada (teórico 5: consumidor
// idempotente con versión monotónica). Un evento repetido o que llega tarde no pisa uno nuevo.
// Si Solr no responde, devuelve el error: comun/mensajeria reintenta y, agotados los reintentos,
// lo manda a la DLQ; la reindexación lo recupera.
func (b *Busqueda) IndexarEvento(ctx context.Context, organizacion string, v puertos.Vista, ocurridoEn time.Time) error {
	if b.buscador == nil {
		return puertos.ErrBusquedaNoDisponible
	}
	if v.Traslado == nil {
		return nil // se buscan traslados: una solicitud sin traslado (de antes de ADR-024) no se indexa
	}
	aplicado, err := b.buscador.Indexar(ctx, documento(organizacion, v))
	if err != nil {
		return err
	}
	if !aplicado {
		slog.InfoContext(ctx, "evento ya indexado", slog.String("solicitud_id", v.ID), slog.Int64("version", v.Version))
		return nil
	}
	// Evidencia del retraso tolerable de ADR-011 (5 s p95): desde el cambio hasta que Solr lo
	// confirmó visible (el adaptador hace soft commit en cada escritura).
	segundos := b.reloj().Sub(ocurridoEn).Seconds()
	b.demora.Record(ctx, segundos)
	slog.InfoContext(ctx, "traslado indexado", slog.String("solicitud_id", v.ID), slog.Int64("version", v.Version),
		slog.Float64("demora_segundos", segundos))
	return nil
}

// documento arma el documento del índice con la vista de la solicitud (§6).
func documento(organizacion string, v puertos.Vista) puertos.DocumentoTraslado {
	d := puertos.DocumentoTraslado{
		SolicitudID: v.ID, OrganizacionID: organizacion, Descripcion: v.Cosa.Descripcion,
		OrigenNombre: v.Origen.Nombre, OrigenDireccion: v.Origen.Direccion,
		DestinoNombre: v.Destino.Nombre, DestinoDireccion: v.Destino.Direccion,
		Version: v.Version, ActualizadoEn: instante(v.ActualizadoEn),
	}
	if v.ReferenciaExterna != nil {
		d.ReferenciaExterna = *v.ReferenciaExterna
	}
	if t := v.Traslado; t != nil {
		d.TrasladoID, d.EstadoTraslado = t.ID, t.Estado
		if t.Franja != nil {
			inicio, fin := instante(t.Franja.Inicio), instante(t.Franja.Fin)
			d.FranjaInicio, d.FranjaFin = &inicio, &fin
		}
		if t.AlertaClimatica != nil {
			d.AlertaClimatica = t.AlertaClimatica.Estado
		}
	}
	if c := v.Cancelacion; c != nil {
		d.CancelacionTardia = c.Tardia
	}
	return d
}

// instante lee una hora RFC 3339 de la vista (la arma NuevaVista, así que siempre es válida).
func instante(s string) time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return t.UTC()
}

// Reindexar arranca la reconstrucción del índice desde MySQL en segundo plano (ADR-011: el índice
// se puede reconstruir desde el dueño). Vuelve enseguida: el endpoint responde 202.
func (b *Busqueda) Reindexar(ctx context.Context) error {
	if b.buscador == nil {
		return puertos.ErrBusquedaNoDisponible
	}
	if !b.reindexando.CompareAndSwap(false, true) {
		slog.InfoContext(ctx, "ya hay una reindexación en curso")
		return nil
	}
	// Con un contexto propio: la reindexación sigue aunque la request que la pidió ya respondió.
	go func() {
		defer b.reindexando.Store(false)
		b.reindexar(context.WithoutCancel(ctx))
	}()
	return nil
}

// reindexar lee las solicitudes de a páginas (cada una en su propia lectura, sin retener una
// transacción larga) y las indexa con la misma regla de versión que el indexador: lo que ya está
// al día no se reescribe y un evento que llega en el medio no se pisa.
func (b *Busqueda) reindexar(ctx context.Context) {
	inicio := b.reloj()
	leidas, aplicadas := 0, 0
	for desde := 0; ; desde += paginaReindexacion {
		var pagina []*dominio.Solicitud
		var total int
		err := b.uow.Ejecutar(ctx, func(ctx context.Context, tx puertos.Transaccion) error {
			var err error
			pagina, total, err = tx.Listar(ctx, puertos.FiltroSolicitudes{}, desde, paginaReindexacion)
			return err
		})
		if err != nil {
			slog.ErrorContext(ctx, "reindexación cortada: no se pudo leer MySQL", slog.Int("leidas", leidas), slog.String("error", err.Error()))
			return
		}
		for _, s := range pagina {
			if s.Traslado == nil {
				continue // como en IndexarEvento: solo traslados
			}
			aplicado, err := b.buscador.Indexar(ctx, documento(s.OrganizacionID, NuevaVista(s)))
			if err != nil {
				slog.ErrorContext(ctx, "reindexación cortada: la búsqueda no responde", slog.Int("leidas", leidas), slog.String("error", err.Error()))
				return
			}
			leidas++
			if aplicado {
				aplicadas++
			}
		}
		if len(pagina) < paginaReindexacion || desde+len(pagina) >= total {
			break
		}
	}
	slog.InfoContext(ctx, "reindexación terminada", slog.Int("cantidad", leidas), slog.Int("actualizadas", aplicadas),
		slog.Int64("duracion_ms", b.reloj().Sub(inicio).Milliseconds()))
}
