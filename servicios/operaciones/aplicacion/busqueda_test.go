package aplicacion

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/francoroblesss/Rumbo/servicios/operaciones/dominio"
	"github.com/francoroblesss/Rumbo/servicios/operaciones/puertos"
)

// buscadorMemoria es un índice en memoria con la misma regla de versión que el de Solr: guarda un
// documento solo si su versión es mayor que la indexada. Recuerda el último filtro.
type buscadorMemoria struct {
	mu     sync.Mutex
	docs   map[string]puertos.DocumentoTraslado
	filtro puertos.FiltroBusqueda
	err    error
}

func nuevoBuscador() *buscadorMemoria {
	return &buscadorMemoria{docs: map[string]puertos.DocumentoTraslado{}}
}

func (b *buscadorMemoria) Buscar(_ context.Context, f puertos.FiltroBusqueda) ([]puertos.DocumentoTraslado, int, error) {
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

func (b *buscadorMemoria) Indexar(_ context.Context, d puertos.DocumentoTraslado) (bool, error) {
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

func (b *buscadorMemoria) cantidad() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.docs)
}

// esperarReindexacion espera a que termine la reindexación en segundo plano.
func (b *Busqueda) esperarReindexacion() { b.enCurso.Wait() }

func consulta() puertos.ConsultaBusqueda {
	return puertos.ConsultaBusqueda{Pagina: 1, Tamanio: 20}
}

func TestBuscarValida(t *testing.T) {
	ctx := context.Background()
	casos := []struct {
		nombre string
		cambio func(*puertos.ConsultaBusqueda)
	}{
		{"página 0", func(c *puertos.ConsultaBusqueda) { c.Pagina = 0 }},
		{"tamaño 0", func(c *puertos.ConsultaBusqueda) { c.Tamanio = 0 }},
		{"tamaño 101", func(c *puertos.ConsultaBusqueda) { c.Tamanio = 101 }},
		{"estado que no se produce", func(c *puertos.ConsultaBusqueda) { c.Estado = "ASIGNADO" }},
		{"estado inexistente", func(c *puertos.ConsultaBusqueda) { c.Estado = "x" }},
		{"orden inexistente", func(c *puertos.ConsultaBusqueda) { c.Orden = "precio" }},
		{"desde mal formado", func(c *puertos.ConsultaBusqueda) { c.Desde = "16/10/2026" }},
		{"hasta mal formado", func(c *puertos.ConsultaBusqueda) { c.Hasta = "2026-13-01" }},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			b := nuevoBuscador()
			q := consulta()
			c.cambio(&q)
			if _, err := NuevaBusqueda(b, nuevaUoW(), func() time.Time { return ahora }).Buscar(ctx, q); !errors.Is(err, dominio.ErrDatosInvalidos) {
				t.Fatalf("err = %v", err)
			}
			// Sin motor, un filtro inválido sigue siendo 422 (y no 503).
			if _, err := NuevaBusqueda(nil, nuevaUoW(), func() time.Time { return ahora }).Buscar(ctx, q); !errors.Is(err, dominio.ErrDatosInvalidos) {
				t.Fatalf("sin motor: err = %v", err)
			}
		})
	}
}

func TestBuscarTraduceElFiltro(t *testing.T) {
	ctx := context.Background()
	b := nuevoBuscador()
	casos := NuevaBusqueda(b, nuevaUoW(), func() time.Time { return ahora })

	q := puertos.ConsultaBusqueda{Texto: "cajas", Organizacion: "org-1", Estado: "EN_CURSO", Desde: "2026-10-16",
		Hasta: "2026-10-17", SoloAlerta: true, Orden: "relevancia", Pagina: 3, Tamanio: 10}
	if _, err := casos.Buscar(ctx, q); err != nil {
		t.Fatal(err)
	}
	f := b.filtro
	// Días de Argentina (-03:00): desde el 16 a las 00:00 hasta el 18 a las 00:00 (el 17 entero).
	desde, hasta := time.Date(2026, 10, 16, 3, 0, 0, 0, time.UTC), time.Date(2026, 10, 18, 3, 0, 0, 0, time.UTC)
	if f.Texto != "cajas" || f.Organizacion != "org-1" || f.Estado != "EN_CURSO" || !f.SoloAlerta ||
		f.Orden != puertos.OrdenRelevancia || f.Desde != 20 || f.Limite != 10 ||
		f.FranjaDesde == nil || !f.FranjaDesde.Equal(desde) || f.FranjaHasta == nil || !f.FranjaHasta.Equal(hasta) {
		t.Fatalf("filtro = %+v (desde %v, hasta %v)", f, f.FranjaDesde, f.FranjaHasta)
	}

	if _, err := casos.Buscar(ctx, consulta()); err != nil {
		t.Fatal(err)
	}
	if f := b.filtro; f.Orden != puertos.OrdenFecha || f.FranjaDesde != nil || f.FranjaHasta != nil || f.Desde != 0 || f.Limite != 20 {
		t.Fatalf("sin parámetros: %+v", f)
	}
}

func TestBuscarSinMotor(t *testing.T) {
	ctx := context.Background()
	casos := NuevaBusqueda(nil, nuevaUoW(), func() time.Time { return ahora })
	if _, err := casos.Buscar(ctx, consulta()); !errors.Is(err, puertos.ErrBusquedaNoDisponible) {
		t.Errorf("buscar: %v", err)
	}
	if err := casos.IndexarEvento(ctx, "org-1", puertos.Vista{ID: "sol-1"}, ahora); !errors.Is(err, puertos.ErrBusquedaNoDisponible) {
		t.Errorf("indexar: %v", err)
	}
	if err := casos.Reindexar(ctx); !errors.Is(err, puertos.ErrBusquedaNoDisponible) {
		t.Errorf("reindexar: %v", err)
	}
	// Solr caído: el error del buscador llega igual.
	b := nuevoBuscador()
	b.err = fmt.Errorf("%w: conexión rechazada", puertos.ErrBusquedaNoDisponible)
	if _, err := NuevaBusqueda(b, nuevaUoW(), func() time.Time { return ahora }).Buscar(ctx, consulta()); !errors.Is(err, puertos.ErrBusquedaNoDisponible) {
		t.Errorf("Solr caído: %v", err)
	}
}

func TestIndexarEvento(t *testing.T) {
	ctx := context.Background()
	e := nuevoEscenario()
	d := datos(nil)
	ref := "TRF-2026-00123"
	d.ReferenciaExterna = &ref
	id := e.reservar(t, "org-1", d, franja16)
	if _, err := e.casos.AvanzarEstado(ctx, "", id, dominio.TrasladoEnCurso); err != nil {
		t.Fatal(err)
	}
	vista, err := e.casos.Obtener(ctx, "", id)
	if err != nil {
		t.Fatal(err)
	}

	b := nuevoBuscador()
	casos := NuevaBusqueda(b, e.uow, func() time.Time { return ahora })
	if err := casos.IndexarEvento(ctx, "org-1", vista, ahora.Add(-2*time.Second)); err != nil {
		t.Fatal(err)
	}
	doc := b.docs[id]
	inicio, fin := franja16, franja16.Add(2*time.Hour)
	if doc.SolicitudID != id || doc.TrasladoID != vista.Traslado.ID || doc.OrganizacionID != "org-1" ||
		doc.ReferenciaExterna == nil || *doc.ReferenciaExterna != ref || doc.Descripcion != "Cajas" ||
		doc.OrigenNombre != "Sucursal Centro" || doc.OrigenDireccion != "Av. Colón 1234" ||
		doc.DestinoNombre != "Aeropuerto" || doc.DestinoDireccion != "Av. La Voz del Interior 8500" ||
		doc.EstadoTraslado != "EN_CURSO" || doc.Version != vista.Version || doc.CancelacionTardia ||
		!doc.FranjaInicio.Equal(inicio) || !doc.FranjaFin.Equal(fin) || doc.ActualizadoEn.IsZero() {
		t.Fatalf("documento = %+v", doc)
	}

	t.Run("el resultado sale con las horas en -03:00", func(t *testing.T) {
		p, err := casos.Buscar(ctx, consulta())
		if err != nil {
			t.Fatal(err)
		}
		r := p.Resultados[0]
		if p.Total != 1 || p.Pagina != 1 || p.Tamanio != 20 || r.SolicitudID != id ||
			*r.FranjaInicio != "2026-10-16T10:00:00-03:00" || *r.FranjaFin != "2026-10-16T12:00:00-03:00" ||
			r.EstadoTraslado != "EN_CURSO" || *r.ReferenciaExterna != ref {
			t.Fatalf("página = %+v, resultado = %+v", p, r)
		}
	})
	t.Run("un evento viejo o repetido no cambia lo indexado", func(t *testing.T) {
		viejo := vista
		viejo.Version--
		viejo.Traslado = &puertos.VistaTraslado{ID: vista.Traslado.ID, Estado: "PROGRAMADO", Franja: vista.Traslado.Franja}
		for _, v := range []puertos.Vista{viejo, vista} {
			if err := casos.IndexarEvento(ctx, "org-1", v, ahora); err != nil {
				t.Fatal(err)
			}
		}
		if b.docs[id].EstadoTraslado != "EN_CURSO" {
			t.Fatalf("quedó %s", b.docs[id].EstadoTraslado)
		}
	})
	t.Run("alerta y cancelación tardía", func(t *testing.T) {
		v := vista
		v.Version += 10
		tr := *vista.Traslado
		tr.AlertaClimatica = &puertos.VistaAlerta{Estado: "NO_OPERABLE", Motivos: []string{"HIGH_WIND"}}
		v.Traslado = &tr
		v.Cancelacion = &puertos.VistaCancelacion{Estado: "APROBADA", Tardia: true}
		if err := casos.IndexarEvento(ctx, "org-1", v, ahora); err != nil {
			t.Fatal(err)
		}
		if d := b.docs[id]; d.AlertaClimatica == nil || *d.AlertaClimatica != "NO_OPERABLE" || !d.CancelacionTardia {
			t.Fatalf("documento = %+v", d)
		}
	})
	t.Run("una solicitud sin traslado no se indexa", func(t *testing.T) {
		antes := b.cantidad()
		if err := casos.IndexarEvento(ctx, "org-1", puertos.Vista{ID: "sol-sin-traslado", Version: 1, ActualizadoEn: hora(ahora)}, ahora); err != nil {
			t.Fatal(err)
		}
		if b.cantidad() != antes {
			t.Fatal("se indexó una solicitud sin traslado")
		}
	})
	t.Run("una vista con fechas ilegibles es un dato inválido", func(t *testing.T) {
		v := vista
		v.Version += 20
		v.ActualizadoEn = "ayer"
		if err := casos.IndexarEvento(ctx, "org-1", v, ahora); !errors.Is(err, dominio.ErrDatosInvalidos) {
			t.Fatalf("err = %v", err)
		}
	})
}

// solicitudConTraslado arma una solicitud aceptada con su franja, sin pasar por Reservar (que
// limita el cupo de cada franja).
func solicitudConTraslado(i int) *dominio.Solicitud {
	inicio := franja16.Add(time.Duration(i) * time.Minute)
	return &dominio.Solicitud{
		ID: fmt.Sprintf("sol-%04d", i), OrganizacionID: "org-1", Estado: dominio.SolicitudAceptada,
		Cosa:  dominio.Cosa{Descripcion: "Cajas", Cantidad: 1},
		Desde: dominio.Ubicacion{Nombre: "A", Direccion: "a"}, Hasta: dominio.Ubicacion{Nombre: "B", Direccion: "b"},
		Traslado: &dominio.Traslado{ID: fmt.Sprintf("tra-%04d", i), Estado: dominio.TrasladoProgramado,
			HoraLlegadaComprometida: inicio.Add(2 * time.Hour), FranjaInicio: &inicio},
		Version: 1, CreadoEn: ahora, ActualizadoEn: ahora,
	}
}

func TestReindexar(t *testing.T) {
	ctx := context.Background()

	t.Run("recorre más de una página de MySQL", func(t *testing.T) {
		uow := nuevaUoW()
		const cantidad = 2*paginaReindexacion + 5
		for i := range cantidad {
			s := solicitudConTraslado(i)
			uow.estado.Solicitudes[s.ID] = s
		}
		b := nuevoBuscador()
		// Un evento más nuevo que lo que hay en MySQL ya llegó: la reindexación no lo pisa.
		nuevo := puertos.DocumentoTraslado{SolicitudID: "sol-0000", EstadoTraslado: "EN_CURSO", Version: 5}
		b.docs[nuevo.SolicitudID] = nuevo
		casos := NuevaBusqueda(b, uow, func() time.Time { return ahora })
		if err := casos.Reindexar(ctx); err != nil {
			t.Fatal(err)
		}
		casos.esperarReindexacion()
		if b.cantidad() != cantidad {
			t.Fatalf("indexados = %d, se esperaban %d", b.cantidad(), cantidad)
		}
		if b.docs["sol-0000"].EstadoTraslado != "EN_CURSO" {
			t.Fatal("la reindexación pisó un documento más nuevo")
		}
	})

	t.Run("se saltea las solicitudes sin traslado", func(t *testing.T) {
		// La uowMemoria ordena por franja y no admite solicitudes sin traslado: acá se usa una
		// transacción que devuelve una lista fija.
		lista := []*dominio.Solicitud{solicitudConTraslado(1), {ID: "sol-vieja", OrganizacionID: "org-1",
			Estado: dominio.EstadoSolicitud("RECIBIDA"), CreadoEn: ahora, ActualizadoEn: ahora}, solicitudConTraslado(2)}
		b := nuevoBuscador()
		casos := NuevaBusqueda(b, uowFija{lista}, func() time.Time { return ahora })
		if err := casos.Reindexar(ctx); err != nil {
			t.Fatal(err)
		}
		casos.esperarReindexacion()
		if _, ok := b.docs["sol-vieja"]; ok || b.cantidad() != 2 {
			t.Fatalf("indexados = %d", b.cantidad())
		}
	})
}

// uowFija es una unidad de trabajo cuyo Listar devuelve una lista fija (paginada). El resto de
// la transacción no se usa al reindexar.
type uowFija struct{ lista []*dominio.Solicitud }

func (u uowFija) Ejecutar(ctx context.Context, fn func(ctx context.Context, tx puertos.Transaccion) error) error {
	return fn(ctx, txFija{lista: u.lista})
}

type txFija struct {
	puertos.Transaccion
	lista []*dominio.Solicitud
}

func (t txFija) Listar(_ context.Context, _ puertos.FiltroSolicitudes, desde, limite int) ([]*dominio.Solicitud, int, error) {
	total := len(t.lista)
	desde = min(desde, total)
	return t.lista[desde:min(desde+limite, total)], total, nil
}
