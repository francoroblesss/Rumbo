package solr

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/francoroblesss/Rumbo/servicios/operaciones/puertos"
)

// Tests de integración con un Solr 9 real en un contenedor (testcontainers), con el mismo core
// que compose.yaml: prueban el esquema, el análisis de texto, los filtros y la concurrencia
// optimista, que con un doble en memoria no se verían.

func solrDePrueba(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	contenedor, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "solr:9",
			Cmd:          []string{"solr-precreate", "traslados"},
			ExposedPorts: []string{"8983/tcp"},
			WaitingFor:   wait.ForHTTP("/solr/traslados/admin/ping").WithPort("8983/tcp").WithStartupTimeout(3 * time.Minute),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("levantar Solr (¿Docker está corriendo?): %v", err)
	}
	t.Cleanup(func() { _ = contenedor.Terminate(context.Background()) })
	base, err := contenedor.PortEndpoint(ctx, "8983/tcp", "http")
	if err != nil {
		t.Fatal(err)
	}
	return base + "/solr/traslados"
}

// clienteDePrueba usa un timeout más largo que el del servicio: el primer cambio de esquema
// recarga el core y puede pasarse de 2 s (en el servicio lo cubre el reintento de main.go).
func clienteDePrueba(url string) *Cliente {
	return NuevoCliente(url, &http.Client{Timeout: 30 * time.Second})
}

func texto(s string) *string { return &s }

func momento(dia, hora int) *time.Time {
	t := time.Date(2026, 11, dia, hora, 0, 0, 0, time.UTC)
	return &t
}

// doc arma un documento con lo mínimo; cada test cambia lo que necesita.
func doc(id, org string) puertos.DocumentoTraslado {
	return puertos.DocumentoTraslado{
		SolicitudID: id, TrasladoID: "tra-" + id, OrganizacionID: org,
		Descripcion: "Cajas de repuestos", OrigenNombre: "Depósito Norte", OrigenDireccion: "Av. Colón 1234",
		DestinoNombre: "Sucursal Centro", DestinoDireccion: "San Martín 50", EstadoTraslado: "PROGRAMADO",
		FranjaInicio: momento(20, 13), FranjaFin: momento(20, 15), Version: 1,
		ActualizadoEn: time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC),
	}
}

func indexar(t *testing.T, c *Cliente, docs ...puertos.DocumentoTraslado) {
	t.Helper()
	for _, d := range docs {
		if aplicado, err := c.Indexar(context.Background(), d); err != nil || !aplicado {
			t.Fatalf("indexar %s: aplicado = %v, err = %v", d.SolicitudID, aplicado, err)
		}
	}
}

func buscar(t *testing.T, c *Cliente, f puertos.FiltroBusqueda) ([]puertos.DocumentoTraslado, int) {
	t.Helper()
	if f.Limite == 0 {
		f.Limite = 20
	}
	docs, total, err := c.Buscar(context.Background(), f)
	if err != nil {
		t.Fatalf("buscar %+v: %v", f, err)
	}
	return docs, total
}

func ids(docs []puertos.DocumentoTraslado) []string {
	r := []string{}
	for _, d := range docs {
		r = append(r, d.SolicitudID)
	}
	return r
}

func iguales(a, b []string) bool {
	return fmt.Sprint(a) == fmt.Sprint(b)
}

func TestSolr(t *testing.T) {
	if testing.Short() {
		t.Skip("test de integración: necesita Docker")
	}
	url := solrDePrueba(t)
	ctx := context.Background()
	c := clienteDePrueba(url)

	t.Run("el esquema se aplica dos veces sin error", func(t *testing.T) {
		if err := c.AsegurarEsquema(ctx); err != nil {
			t.Fatal(err)
		}
		// Otro cliente (otra réplica u otro arranque) encuentra todo y no agrega nada.
		if err := clienteDePrueba(url).AsegurarEsquema(ctx); err != nil {
			t.Fatalf("segunda vez: %v", err)
		}
	})

	t.Run("análisis de texto en español", func(t *testing.T) {
		org := "org-texto"
		a := doc("sol-texto-1", org)
		a.Descripcion = "Cajas"
		a.ReferenciaExterna = texto("TRF-2026-00123")
		b := doc("sol-texto-2", org)
		b.Descripcion = "Mercadería refrigerada"
		b.DestinoNombre = "Hospital Privado"
		b.DestinoDireccion = "Naciones Unidas 346"
		indexar(t, c, a, b)
		casos := []struct{ q, id string }{
			{"CAJA", "sol-texto-1"},            // mayúsculas y singular encuentran "Cajas"
			{"mercaderia", "sol-texto-2"},      // sin tilde encuentra "mercadería"
			{"hospital", "sol-texto-2"},        // una palabra del destino
			{"Naciones", "sol-texto-2"},        // una palabra de la dirección
			{"TRF-2026-00123", "sol-texto-1"},  // la referencia externa
			{"camión Hospital", "sol-texto-2"}, // una palabra que no está no impide encontrar
		}
		for _, cs := range casos {
			docs, _ := buscar(t, c, puertos.FiltroBusqueda{Texto: cs.q, Organizacion: org})
			if len(docs) == 0 || docs[0].SolicitudID != cs.id {
				t.Errorf("q=%q: %v, se esperaba %s primero", cs.q, ids(docs), cs.id)
			}
		}
	})

	t.Run("relevancia: lo que tiene las dos palabras va primero", func(t *testing.T) {
		org := "org-relevancia"
		una := doc("sol-rel-1", org)
		una.Descripcion = "Sillas"
		una.FranjaInicio = momento(18, 13) // por fecha iría primero
		dos := doc("sol-rel-2", org)
		dos.Descripcion = "Sillas de oficina"
		dos.FranjaInicio = momento(25, 13)
		indexar(t, c, una, dos)
		docs, _ := buscar(t, c, puertos.FiltroBusqueda{Texto: "sillas oficina", Organizacion: org, Orden: puertos.OrdenRelevancia})
		if !iguales(ids(docs), []string{"sol-rel-2", "sol-rel-1"}) {
			t.Fatalf("por relevancia: %v", ids(docs))
		}
		docs, _ = buscar(t, c, puertos.FiltroBusqueda{Texto: "sillas oficina", Organizacion: org, Orden: puertos.OrdenFecha})
		if !iguales(ids(docs), []string{"sol-rel-1", "sol-rel-2"}) {
			t.Fatalf("por fecha: %v", ids(docs))
		}
	})

	t.Run("filtros por estado, alerta y rango de fechas", func(t *testing.T) {
		org := "org-filtros"
		a := doc("sol-fil-1", org)
		a.FranjaInicio = momento(10, 13)
		b := doc("sol-fil-2", org)
		b.EstadoTraslado = "EN_CURSO"
		b.AlertaClimatica = texto("NO_OPERABLE")
		b.FranjaInicio = momento(11, 13)
		d := doc("sol-fil-3", org)
		d.FranjaInicio = momento(12, 13)
		indexar(t, c, a, b, d)
		if docs, _ := buscar(t, c, puertos.FiltroBusqueda{Organizacion: org, Estado: "EN_CURSO"}); !iguales(ids(docs), []string{"sol-fil-2"}) {
			t.Errorf("estado: %v", ids(docs))
		}
		if docs, _ := buscar(t, c, puertos.FiltroBusqueda{Organizacion: org, SoloAlerta: true}); !iguales(ids(docs), []string{"sol-fil-2"}) {
			t.Errorf("alerta: %v", ids(docs))
		}
		// [11, 12): el 12 queda afuera (hasta es exclusive).
		if docs, _ := buscar(t, c, puertos.FiltroBusqueda{Organizacion: org, FranjaDesde: momento(11, 0), FranjaHasta: momento(12, 0)}); !iguales(ids(docs), []string{"sol-fil-2"}) {
			t.Errorf("rango: %v", ids(docs))
		}
		if docs, _ := buscar(t, c, puertos.FiltroBusqueda{Organizacion: org, FranjaDesde: momento(11, 0)}); !iguales(ids(docs), []string{"sol-fil-2", "sol-fil-3"}) {
			t.Errorf("solo desde: %v", ids(docs))
		}
		if docs, _ := buscar(t, c, puertos.FiltroBusqueda{Organizacion: org, FranjaHasta: momento(11, 0)}); !iguales(ids(docs), []string{"sol-fil-1"}) {
			t.Errorf("solo hasta: %v", ids(docs))
		}
	})

	t.Run("cada organización ve solo lo suyo (CA-25)", func(t *testing.T) {
		uno := doc("sol-org-1", "org-1")
		uno.DestinoNombre = "Clínica Reina Fabiola"
		dos := doc("sol-org-2", "org-2")
		dos.DestinoNombre = "Clínica Reina Fabiola"
		indexar(t, c, uno, dos)
		if docs, _ := buscar(t, c, puertos.FiltroBusqueda{Texto: "Fabiola", Organizacion: "org-1"}); !iguales(ids(docs), []string{"sol-org-1"}) {
			t.Errorf("org-1: %v", ids(docs))
		}
		// {!term} toma el valor como un literal: la sintaxis de Solr no amplía el filtro.
		if docs, total := buscar(t, c, puertos.FiltroBusqueda{Organizacion: "org-1 OR organizacion_id:org-2"}); total != 0 {
			t.Errorf("inyección en el filtro: %v", ids(docs))
		}
	})

	t.Run("paginación con total", func(t *testing.T) {
		org := "org-paginas"
		for i := range 5 {
			d := doc(fmt.Sprintf("sol-pag-%d", i), org)
			d.FranjaInicio = momento(10+i, 13)
			indexar(t, c, d)
		}
		docs, total := buscar(t, c, puertos.FiltroBusqueda{Organizacion: org, Desde: 2, Limite: 2})
		if total != 5 || !iguales(ids(docs), []string{"sol-pag-2", "sol-pag-3"}) {
			t.Fatalf("total = %d, página = %v", total, ids(docs))
		}
	})

	t.Run("un documento sin franja queda último por fecha", func(t *testing.T) {
		org := "org-sin-franja"
		sin := doc("sol-sf-1", org)
		sin.FranjaInicio, sin.FranjaFin = nil, nil
		con := doc("sol-sf-2", org)
		indexar(t, c, sin, con)
		docs, _ := buscar(t, c, puertos.FiltroBusqueda{Organizacion: org})
		if !iguales(ids(docs), []string{"sol-sf-2", "sol-sf-1"}) || docs[1].FranjaInicio != nil {
			t.Fatalf("orden: %v", ids(docs))
		}
	})

	t.Run("los campos vuelven completos", func(t *testing.T) {
		d := doc("sol-campos", "org-campos")
		d.ReferenciaExterna = texto("TRF-1")
		d.AlertaClimatica = texto("NO_VERIFICADO")
		d.CancelacionTardia = true
		indexar(t, c, d)
		docs, _ := buscar(t, c, puertos.FiltroBusqueda{Organizacion: "org-campos"})
		if len(docs) != 1 {
			t.Fatalf("docs = %v", ids(docs))
		}
		g := docs[0]
		if g.SolicitudID != d.SolicitudID || g.TrasladoID != d.TrasladoID || g.OrganizacionID != d.OrganizacionID ||
			*g.ReferenciaExterna != "TRF-1" || g.Descripcion != d.Descripcion || g.OrigenNombre != d.OrigenNombre ||
			g.DestinoNombre != d.DestinoNombre || g.EstadoTraslado != d.EstadoTraslado ||
			!g.FranjaInicio.Equal(*d.FranjaInicio) || !g.FranjaFin.Equal(*d.FranjaFin) ||
			*g.AlertaClimatica != "NO_VERIFICADO" || !g.CancelacionTardia {
			t.Fatalf("documento devuelto: %+v", g)
		}
	})

	t.Run("un evento viejo no pisa uno nuevo y uno repetido no cambia nada", func(t *testing.T) {
		org := "org-version"
		nuevo := doc("sol-ver", org)
		nuevo.Version, nuevo.EstadoTraslado = 3, "EN_CURSO"
		indexar(t, c, nuevo)
		viejo := nuevo
		viejo.Version, viejo.EstadoTraslado = 2, "PROGRAMADO"
		if aplicado, err := c.Indexar(ctx, viejo); err != nil || aplicado {
			t.Fatalf("viejo: aplicado = %v, err = %v", aplicado, err)
		}
		if aplicado, err := c.Indexar(ctx, nuevo); err != nil || aplicado {
			t.Fatalf("repetido: aplicado = %v, err = %v", aplicado, err)
		}
		docs, _ := buscar(t, c, puertos.FiltroBusqueda{Organizacion: org})
		if len(docs) != 1 || docs[0].EstadoTraslado != "EN_CURSO" {
			t.Fatalf("quedó: %+v", docs)
		}
	})

	t.Run("10 escrituras concurrentes del mismo documento: queda la versión más alta", func(t *testing.T) {
		org := "org-concurrencia"
		var wg sync.WaitGroup
		errs := make(chan error, 10)
		for v := int64(4); v <= 13; v++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				d := doc("sol-conc", org)
				d.Version, d.EstadoTraslado = v, fmt.Sprintf("V%d", v)
				if _, err := c.Indexar(ctx, d); err != nil {
					errs <- fmt.Errorf("versión %d: %w", v, err)
				}
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			t.Error(err)
		}
		docs, _ := buscar(t, c, puertos.FiltroBusqueda{Organizacion: org})
		if len(docs) != 1 || docs[0].EstadoTraslado != "V13" {
			t.Fatalf("quedó: %+v", docs)
		}
	})

	t.Run("sin Solr: ErrBusquedaNoDisponible", func(t *testing.T) {
		caido := NuevoCliente("http://127.0.0.1:1/solr/traslados", &http.Client{Timeout: 2 * time.Second})
		if _, _, err := caido.Buscar(ctx, puertos.FiltroBusqueda{Limite: 10}); !errors.Is(err, puertos.ErrBusquedaNoDisponible) {
			t.Errorf("buscar: %v", err)
		}
		if _, err := caido.Indexar(ctx, doc("sol-x", "org-x")); !errors.Is(err, puertos.ErrBusquedaNoDisponible) {
			t.Errorf("indexar: %v", err)
		}
	})
}
