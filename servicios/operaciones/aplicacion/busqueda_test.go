package aplicacion

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/francoroblesss/Rumbo/servicios/operaciones/dominio"
	"github.com/francoroblesss/Rumbo/servicios/operaciones/puertos"
)

// buscadorFalso guarda los documentos en memoria con la misma regla de versión que Solr, y el
// último filtro que recibió.
type buscadorFalso struct {
	mu     sync.Mutex
	docs   map[string]puertos.DocumentoTraslado
	filtro puertos.FiltroBusqueda
	err    error
}

func nuevoBuscador() *buscadorFalso {
	return &buscadorFalso{docs: map[string]puertos.DocumentoTraslado{}}
}

func (b *buscadorFalso) Buscar(_ context.Context, f puertos.FiltroBusqueda) ([]puertos.DocumentoTraslado, int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.filtro = f
	if b.err != nil {
		return nil, 0, b.err
	}
	var r []puertos.DocumentoTraslado
	for _, d := range b.docs {
		r = append(r, d)
	}
	return r, len(r), nil
}

func (b *buscadorFalso) Indexar(_ context.Context, d puertos.DocumentoTraslado) (bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.err != nil {
		return false, b.err
	}
	if actual, ok := b.docs[d.SolicitudID]; ok && actual.Version >= d.Version {
		return false, nil
	}
	b.docs[d.SolicitudID] = d
	return true, nil
}

func (b *buscadorFalso) cantidad() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.docs)
}

func consulta() puertos.ConsultaBusqueda { return puertos.ConsultaBusqueda{Pagina: 1, Tamanio: 20} }

func TestBuscarValidaYTraduce(t *testing.T) {
	b := nuevoBuscador()
	busqueda := NuevaBusqueda(b, nuevaUoW(), func() time.Time { return ahora })
	ctx := context.Background()

	invalidas := map[string]func(*puertos.ConsultaBusqueda){
		"página 0":                 func(c *puertos.ConsultaBusqueda) { c.Pagina = 0 },
		"tamaño 101":               func(c *puertos.ConsultaBusqueda) { c.Tamanio = 101 },
		"estado que no se produce": func(c *puertos.ConsultaBusqueda) { c.Estado = "ASIGNADO" },
		"orden desconocido":        func(c *puertos.ConsultaBusqueda) { c.Orden = "precio" },
		"desde mal formado":        func(c *puertos.ConsultaBusqueda) { c.Desde = "20/11/2026" },
		"hasta mal formado":        func(c *puertos.ConsultaBusqueda) { c.Hasta = "2026-13-01" },
	}
	for nombre, cambiar := range invalidas {
		c := consulta()
		cambiar(&c)
		if _, err := busqueda.Buscar(ctx, c); !errors.Is(err, dominio.ErrDatosInvalidos) {
			t.Errorf("%s: se esperaba ErrDatosInvalidos, vino %v", nombre, err)
		}
	}

	// Desde y hasta son días de Argentina: [20/11 00:00 -03, 22/11 00:00 -03) en UTC.
	c := consulta()
	c.Texto, c.Organizacion, c.Estado, c.Desde, c.Hasta = "  cajas  ", "org-1", "EN_CURSO", "2026-11-20", "2026-11-21"
	c.Orden, c.Pagina, c.Tamanio, c.SoloConAlerta = "relevancia", 3, 10, true
	if _, err := busqueda.Buscar(ctx, c); err != nil {
		t.Fatal(err)
	}
	f := b.filtro
	desde, hasta := time.Date(2026, 11, 20, 3, 0, 0, 0, time.UTC), time.Date(2026, 11, 22, 3, 0, 0, 0, time.UTC)
	if f.Texto != "cajas" || f.Organizacion != "org-1" || f.EstadoTraslado != "EN_CURSO" || !f.SoloConAlerta ||
		!f.PorRelevancia || f.Desde != 20 || f.Limite != 10 || !f.FranjaDesde.Equal(desde) || !f.FranjaHasta.Equal(hasta) {
		t.Fatalf("filtro traducido: %+v (desde %v, hasta %v)", f, f.FranjaDesde, f.FranjaHasta)
	}

	// Sin orden: por fecha.
	if _, err := busqueda.Buscar(ctx, consulta()); err != nil || b.filtro.PorRelevancia || b.filtro.FranjaDesde != nil {
		t.Fatalf("por defecto: %v %+v", err, b.filtro)
	}
}

func TestBusquedaSinMotor(t *testing.T) {
	busqueda := NuevaBusqueda(nil, nuevaUoW(), func() time.Time { return ahora })
	ctx := context.Background()
	if _, err := busqueda.Buscar(ctx, consulta()); !errors.Is(err, puertos.ErrBusquedaNoDisponible) {
		t.Fatalf("buscar sin motor: %v", err)
	}
	if err := busqueda.Reindexar(ctx); !errors.Is(err, puertos.ErrBusquedaNoDisponible) {
		t.Fatalf("reindexar sin motor: %v", err)
	}
	if err := busqueda.IndexarEvento(ctx, "org-1", puertos.Vista{ID: "sol-1"}, ahora); !errors.Is(err, puertos.ErrBusquedaNoDisponible) {
		t.Fatalf("indexar sin motor: %v", err)
	}
	// Un filtro inválido sigue siendo 422 aunque no haya motor.
	c := consulta()
	c.Pagina = 0
	if _, err := busqueda.Buscar(ctx, c); !errors.Is(err, dominio.ErrDatosInvalidos) {
		t.Fatalf("filtro inválido sin motor: %v", err)
	}
}

func TestIndexarEventoArmaElDocumentoYRespetaLaVersion(t *testing.T) {
	e := nuevoEscenario()
	b := nuevoBuscador()
	busqueda := NuevaBusqueda(b, e.uow, func() time.Time { return ahora.Add(2 * time.Second) })
	ctx := context.Background()

	id := e.reservar(t, "org-1", alAeropuerto(), franja16)
	if _, err := e.casos.CancelarDesdeWeb(ctx, "", id, "no viaja", clave("cancelar", "h")); err != nil {
		t.Fatal(err)
	}
	v, err := e.casos.Obtener(ctx, "", id)
	if err != nil {
		t.Fatal(err)
	}
	if err := busqueda.IndexarEvento(ctx, "org-1", v, ahora); err != nil {
		t.Fatal(err)
	}
	d := b.docs[id]
	if d.OrganizacionID != "org-1" || d.TrasladoID == "" || d.EstadoTraslado != "CANCELADO" || d.Version != v.Version ||
		d.FranjaInicio == nil || d.FranjaFin == nil || !d.FranjaFin.After(*d.FranjaInicio) ||
		d.DestinoNombre != alAeropuerto().Hasta.Nombre || d.Descripcion != alAeropuerto().Cosa.Descripcion {
		t.Fatalf("documento: %+v", d)
	}

	// Un evento viejo (versión menor) que llega tarde no pisa el documento nuevo.
	vieja := v
	vieja.Version = v.Version - 1
	vieja.Traslado = &puertos.VistaTraslado{ID: v.Traslado.ID, Estado: "PROGRAMADO"}
	if err := busqueda.IndexarEvento(ctx, "org-1", vieja, ahora); err != nil {
		t.Fatal(err)
	}
	if b.docs[id].EstadoTraslado != "CANCELADO" {
		t.Fatalf("un evento viejo pisó al nuevo: %+v", b.docs[id])
	}

	// Una solicitud sin traslado no se indexa (se buscan traslados).
	if err := busqueda.IndexarEvento(ctx, "org-1", puertos.Vista{ID: "sol-sin-traslado", Version: 1}, ahora); err != nil {
		t.Fatal(err)
	}
	if _, ok := b.docs["sol-sin-traslado"]; ok {
		t.Fatal("se indexó una solicitud sin traslado")
	}

	// Solr caído: el error sale tal cual (el consumidor reintenta).
	b.err = puertos.ErrBusquedaNoDisponible
	if err := busqueda.IndexarEvento(ctx, "org-1", v, ahora); !errors.Is(err, puertos.ErrBusquedaNoDisponible) {
		t.Fatalf("con Solr caído: %v", err)
	}
}

func TestReindexarRecorreTodasLasSolicitudes(t *testing.T) {
	e := nuevoEscenario()
	b := nuevoBuscador()
	busqueda := NuevaBusqueda(b, e.uow, func() time.Time { return ahora })

	// Más de una página de MySQL (paginaReindexacion = 200). El calendario de prueba tiene 6
	// franjas por día con lugar para 2: se reparten desde el 16/10 a las 8 de Córdoba.
	const cantidad = 230
	primera := time.Date(2026, 10, 16, 11, 0, 0, 0, time.UTC)
	porDia := 6 * calendario.CupoTraslados
	for i := range cantidad {
		franja := primera.AddDate(0, 0, i/porDia).Add(time.Duration(i%porDia/calendario.CupoTraslados) * 2 * time.Hour)
		e.reservar(t, []string{"org-1", "org-2"}[i%2], comun(), franja)
	}
	if err := busqueda.Reindexar(context.Background()); err != nil {
		t.Fatal(err)
	}
	limite := time.Now().Add(5 * time.Second)
	for b.cantidad() < cantidad || busqueda.reindexando.Load() {
		if time.Now().After(limite) {
			t.Fatalf("indexadas %d de %d", b.cantidad(), cantidad)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if b.cantidad() != cantidad {
		t.Fatalf("indexadas %d, se esperaban %d", b.cantidad(), cantidad)
	}
}
