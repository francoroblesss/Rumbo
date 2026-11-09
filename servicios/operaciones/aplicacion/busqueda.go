package aplicacion

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/metric"

	"github.com/francoroblesss/Rumbo/servicios/comun/observabilidad"
	"github.com/francoroblesss/Rumbo/servicios/operaciones/dominio"
	"github.com/francoroblesss/Rumbo/servicios/operaciones/puertos"
)

// Búsqueda de traslados (ADR-011, docs/interfaces-internas.md §16). El índice es un modelo de
// lectura derivado (teórico 5): se actualiza con los eventos que Operaciones ya publica (outbox →
// broker → indexador) y se reconstruye desde MySQL. Nada de lo que decide el negocio lee el índice.

// paginaReindexacion es cuántas solicitudes se leen de MySQL por vez al reindexar.
const paginaReindexacion = 200

// Busqueda implementa puertos.Busqueda.
type Busqueda struct {
	buscador puertos.BuscadorTraslados // nil = sin motor (sin SOLR_URL, como en Render, ADR-026)
	uow      puertos.UnidadDeTrabajo
	reloj    func() time.Time

	demora      metric.Float64Histogram
	reindexando atomic.Bool    // hay una reindexación en curso
	enCurso     sync.WaitGroup // la reindexación en segundo plano (los tests la esperan)
}

var _ puertos.Busqueda = (*Busqueda)(nil)

// NuevaBusqueda arma los casos de uso de la búsqueda. Con buscador nil, todo responde
// ErrBusquedaNoDisponible: el servicio arranca igual y solo la búsqueda queda sin motor.
func NuevaBusqueda(buscador puertos.BuscadorTraslados, uow puertos.UnidadDeTrabajo, reloj func() time.Time) *Busqueda {
	// Límites en segundos (con los de OpenTelemetry por defecto, pensados para milisegundos, todo
	// caería en el primer bucket). Cubren el objetivo de 5 s de ADR-011.
	demora, _ := observabilidad.Medidor().Float64Histogram("rumbo_indexacion_demora_segundos",
		metric.WithDescription("Demora entre que ocurre un cambio en Operaciones y queda visible en la búsqueda"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30, 60))
	return &Busqueda{buscador: buscador, uow: uow, reloj: reloj, demora: demora}
}

// Buscar valida la consulta, la traduce al filtro del índice y mapea los documentos a la
// respuesta de §16.
func (b *Busqueda) Buscar(ctx context.Context, c puertos.ConsultaBusqueda) (puertos.PaginaBusqueda, error) {
	filtro, err := filtroBusqueda(c)
	if err != nil {
		return puertos.PaginaBusqueda{}, err // un filtro inválido es 422 aunque no haya motor
	}
	if b.buscador == nil {
		return puertos.PaginaBusqueda{}, puertos.ErrBusquedaNoDisponible
	}
	documentos, total, err := b.buscador.Buscar(ctx, filtro)
	if err != nil {
		return puertos.PaginaBusqueda{}, err
	}
	pagina := puertos.PaginaBusqueda{Resultados: make([]puertos.ResultadoBusqueda, 0, len(documentos)),
		Pagina: c.Pagina, Tamanio: c.Tamanio, Total: total}
	for _, d := range documentos {
		pagina.Resultados = append(pagina.Resultados, puertos.ResultadoBusqueda{
			SolicitudID:       d.SolicitudID,
			TrasladoID:        d.TrasladoID,
			OrganizacionID:    d.OrganizacionID,
			ReferenciaExterna: d.ReferenciaExterna,
			Descripcion:       d.Descripcion,
			OrigenNombre:      d.OrigenNombre,
			DestinoNombre:     d.DestinoNombre,
			EstadoTraslado:    d.EstadoTraslado,
			FranjaInicio:      horaOpcional(d.FranjaInicio),
			FranjaFin:         horaOpcional(d.FranjaFin),
			AlertaClimatica:   d.AlertaClimatica,
			CancelacionTardia: d.CancelacionTardia,
		})
	}
	return pagina, nil
}

// filtroBusqueda valida la consulta (dominio.ErrDatosInvalidos) y la traduce: las fechas son días
// de Argentina; desde → inicio de ese día y hasta (inclusive) → inicio del día siguiente, en UTC.
func filtroBusqueda(c puertos.ConsultaBusqueda) (puertos.FiltroBusqueda, error) {
	if c.Pagina < 1 || c.Tamanio < 1 || c.Tamanio > TamanioMaximo ||
		(c.Estado != "" && !estadosListables[dominio.EstadoTraslado(c.Estado)]) {
		return puertos.FiltroBusqueda{}, dominio.ErrDatosInvalidos
	}
	orden := c.Orden
	if orden == "" {
		orden = puertos.OrdenFecha
	}
	if orden != puertos.OrdenFecha && orden != puertos.OrdenRelevancia {
		return puertos.FiltroBusqueda{}, dominio.ErrDatosInvalidos
	}
	f := puertos.FiltroBusqueda{Texto: c.Texto, Organizacion: c.Organizacion, Estado: c.Estado, SoloAlerta: c.SoloAlerta,
		Orden: orden, Desde: (c.Pagina - 1) * c.Tamanio, Limite: c.Tamanio}
	if c.Desde != "" {
		dia, err := time.ParseInLocation(time.DateOnly, c.Desde, zonaRumbo)
		if err != nil {
			return puertos.FiltroBusqueda{}, dominio.ErrDatosInvalidos
		}
		desde := dia.UTC()
		f.FranjaDesde = &desde
	}
	if c.Hasta != "" {
		dia, err := time.ParseInLocation(time.DateOnly, c.Hasta, zonaRumbo)
		if err != nil {
			return puertos.FiltroBusqueda{}, dominio.ErrDatosInvalidos
		}
		hasta := dia.AddDate(0, 0, 1).UTC()
		f.FranjaHasta = &hasta
	}
	return f, nil
}

// IndexarEvento aplica la vista de un evento (§5.4) al índice, con la regla de versión del
// indexador idempotente (teórico 4 y 5): at-least-once significa que un evento puede llegar dos
// veces o desordenado, y ninguno de los dos casos cambia lo indexado.
func (b *Busqueda) IndexarEvento(ctx context.Context, organizacion string, v puertos.Vista, ocurridoEn time.Time) error {
	if b.buscador == nil {
		return puertos.ErrBusquedaNoDisponible
	}
	if v.Traslado == nil {
		// Se buscan traslados: una solicitud que nunca se aceptó no tiene nada que indexar.
		slog.InfoContext(ctx, "solicitud sin traslado: no se indexa", slog.String("solicitud_id", v.ID))
		return nil
	}
	d, err := documentoDesdeVista(organizacion, v)
	if err != nil {
		return err
	}
	aplicado, err := b.buscador.Indexar(ctx, d)
	if err != nil {
		return err
	}
	if !aplicado {
		slog.InfoContext(ctx, "evento ya indexado", slog.String("solicitud_id", d.SolicitudID), slog.Int64("version", d.Version))
		return nil
	}
	demora := b.reloj().Sub(ocurridoEn).Seconds()
	b.demora.Record(ctx, demora)
	slog.InfoContext(ctx, "traslado indexado", slog.String("solicitud_id", d.SolicitudID),
		slog.Int64("version", d.Version), slog.Float64("demora_segundos", demora))
	return nil
}

// documentoDesdeVista arma el documento del índice con la vista (la de los eventos y la que se
// arma desde MySQL al reindexar). La vista tiene un traslado.
func documentoDesdeVista(organizacion string, v puertos.Vista) (puertos.DocumentoTraslado, error) {
	actualizado, err := time.Parse(time.RFC3339, v.ActualizadoEn)
	if err != nil {
		return puertos.DocumentoTraslado{}, fmt.Errorf("%w: actualizado_en de %s: %v", dominio.ErrDatosInvalidos, v.ID, err)
	}
	t := v.Traslado
	d := puertos.DocumentoTraslado{
		SolicitudID:       v.ID,
		TrasladoID:        t.ID,
		OrganizacionID:    organizacion,
		ReferenciaExterna: v.ReferenciaExterna,
		Descripcion:       v.Cosa.Descripcion,
		OrigenNombre:      v.Origen.Nombre,
		OrigenDireccion:   v.Origen.Direccion,
		DestinoNombre:     v.Destino.Nombre,
		DestinoDireccion:  v.Destino.Direccion,
		EstadoTraslado:    t.Estado,
		CancelacionTardia: v.Cancelacion != nil && v.Cancelacion.Tardia,
		Version:           v.Version,
		ActualizadoEn:     actualizado.UTC(),
	}
	if f := t.Franja; f != nil {
		inicio, err1 := time.Parse(time.RFC3339, f.Inicio)
		fin, err2 := time.Parse(time.RFC3339, f.Fin)
		if err1 != nil || err2 != nil {
			return puertos.DocumentoTraslado{}, fmt.Errorf("%w: franja de %s", dominio.ErrDatosInvalidos, v.ID)
		}
		inicio, fin = inicio.UTC(), fin.UTC()
		d.FranjaInicio, d.FranjaFin = &inicio, &fin
	}
	if a := t.AlertaClimatica; a != nil {
		estado := a.Estado
		d.AlertaClimatica = &estado
	}
	return d, nil
}

// Reindexar reconstruye el índice desde MySQL, el dueño de los datos (teórico 5). Responde enseguida
// y corre en segundo plano; si ya hay una en curso, no arranca otra.
func (b *Busqueda) Reindexar(ctx context.Context) error {
	if b.buscador == nil {
		return puertos.ErrBusquedaNoDisponible
	}
	if !b.reindexando.CompareAndSwap(false, true) {
		slog.InfoContext(ctx, "reindexación ya en curso")
		return nil
	}
	b.enCurso.Add(1)
	// La request ya respondió: el contexto no se cancela con ella, pero conserva la traza y el
	// correlation id.
	go func(ctx context.Context) {
		defer b.enCurso.Done()
		defer b.reindexando.Store(false)
		slog.InfoContext(ctx, "reindexación iniciada")
		cantidad, err := b.reindexar(ctx)
		if err != nil {
			slog.ErrorContext(ctx, "reindexación fallida", slog.Int("cantidad", cantidad), slog.String("error", err.Error()))
			return
		}
		slog.InfoContext(ctx, "reindexación terminada", slog.Int("cantidad", cantidad))
	}(context.WithoutCancel(ctx))
	return nil
}

// reindexar recorre MySQL de a páginas, cada una en su propia transacción corta (sin una
// transacción larga que retenga la base), e indexa cada traslado con la misma regla de versión
// que el indexador: si un evento más nuevo ya llegó, no se pisa. Devuelve cuántos traslados recorrió.
func (b *Busqueda) reindexar(ctx context.Context) (int, error) {
	cantidad := 0
	for desde := 0; ; {
		var pagina []*dominio.Solicitud
		var total int
		err := b.uow.Ejecutar(ctx, func(ctx context.Context, tx puertos.Transaccion) error {
			var err error
			pagina, total, err = tx.Listar(ctx, puertos.FiltroSolicitudes{}, desde, paginaReindexacion)
			return err
		})
		if err != nil {
			return cantidad, fmt.Errorf("leer solicitudes: %w", err)
		}
		for _, s := range pagina {
			if s.Traslado == nil {
				continue // solicitudes viejas que nunca se aceptaron: no son traslados
			}
			d, err := documentoDesdeVista(s.OrganizacionID, NuevaVista(s))
			if err != nil {
				return cantidad, err
			}
			if _, err := b.buscador.Indexar(ctx, d); err != nil {
				return cantidad, fmt.Errorf("indexar %s: %w", s.ID, err)
			}
			cantidad++
		}
		desde += len(pagina)
		if len(pagina) == 0 || desde >= total {
			return cantidad, nil
		}
	}
}
