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

// solrDePrueba levanta Solr 9 con el core "traslados", como compose.yaml (solr-precreate).
func solrDePrueba(t *testing.T) *Cliente {
	t.Helper()
	if testing.Short() {
		t.Skip("test de integración con contenedores")
	}
	ctx := context.Background()
	contenedor, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "solr:9",
			Cmd:          []string{"solr-precreate", "traslados"},
			ExposedPorts: []string{"8983/tcp"},
			WaitingFor:   wait.ForHTTP("/solr/traslados/admin/ping").WithPort("8983/tcp").WithStartupTimeout(2 * time.Minute),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("levantar Solr (¿Docker está corriendo?): %v", err)
	}
	t.Cleanup(func() { _ = contenedor.Terminate(context.Background()) })
	host, err := contenedor.Host(ctx)
	if err != nil {
		t.Fatal(err)
	}
	puerto, err := contenedor.MappedPort(ctx, "8983/tcp")
	if err != nil {
		t.Fatal(err)
	}
	c := NuevoCliente(fmt.Sprintf("http://%s:%s/solr/traslados", host, puerto.Port()), &http.Client{Timeout: 5 * time.Second})
	if err := c.AsegurarEsquema(ctx); err != nil {
		t.Fatalf("esquema: %v", err)
	}
	// Idempotente: un segundo cliente (otra réplica, otro arranque) lo vuelve a aplicar sin error.
	if err := NuevoCliente(c.base, c.http).AsegurarEsquema(ctx); err != nil {
		t.Fatalf("esquema por segunda vez: %v", err)
	}
	return c
}

// franja es la franja del día dia de noviembre de 2026 a las h (hora de Córdoba), en UTC.
func franja(dia, h int) *time.Time {
	t := time.Date(2026, 11, dia, h+3, 0, 0, 0, time.UTC)
	return &t
}

func doc(id, org, descripcion, destino, estado string, inicio *time.Time, version int64) puertos.DocumentoTraslado {
	fin := inicio.Add(2 * time.Hour)
	return puertos.DocumentoTraslado{
		SolicitudID: id, TrasladoID: "tra-" + id, OrganizacionID: org, Descripcion: descripcion,
		OrigenNombre: "Sucursal Centro", OrigenDireccion: "Av. Colón 1234, Córdoba",
		DestinoNombre: destino, DestinoDireccion: "Av. Rafael Núñez 4500, Córdoba",
		EstadoTraslado: estado, FranjaInicio: inicio, FranjaFin: &fin, Version: version,
		ActualizadoEn: time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC),
	}
}

func ids(docs []puertos.DocumentoTraslado) []string {
	r := []string{}
	for _, d := range docs {
		r = append(r, d.SolicitudID)
	}
	return r
}

func TestSolr(t *testing.T) {
	c := solrDePrueba(t)
	ctx := context.Background()

	alerta := doc("sol-4", "org-1", "Equipaje de tripulación", "Aeropuerto Internacional", "PROGRAMADO", franja(21, 8), 1)
	alerta.AlertaClimatica = "NO_OPERABLE"
	alerta.ReferenciaExterna = "TRF-2026-00999"
	cargados := []puertos.DocumentoTraslado{
		doc("sol-1", "org-1", "Cajas de mercadería seca", "Sucursal Cerro", "PROGRAMADO", franja(20, 10), 1),
		doc("sol-2", "org-1", "Heladeras industriales", "Depósito Norte", "EN_CURSO", franja(20, 8), 2),
		doc("sol-3", "org-2", "Cajas de repuestos", "Sucursal Cerro", "PROGRAMADO", franja(20, 12), 1),
		alerta,
		{SolicitudID: "sol-5", OrganizacionID: "org-1", Descripcion: "Solicitud sin traslado", Version: 1}, // de antes de ADR-024
	}
	for _, d := range cargados {
		if ok, err := c.Indexar(ctx, d); err != nil || !ok {
			t.Fatalf("indexar %s: %v %v", d.SolicitudID, ok, err)
		}
	}
	buscar := func(f puertos.FiltroBusqueda) ([]string, int) {
		t.Helper()
		if f.Limite == 0 {
			f.Limite = 20
		}
		docs, total, err := c.Buscar(ctx, f)
		if err != nil {
			t.Fatalf("buscar %+v: %v", f, err)
		}
		return ids(docs), total
	}
	igual := func(nombre string, obtenido []string, esperado ...string) {
		t.Helper()
		if fmt.Sprint(obtenido) != fmt.Sprint(esperado) {
			t.Errorf("%s: %v, se esperaba %v", nombre, obtenido, esperado)
		}
	}

	t.Run("sin texto, todo por fecha de franja", func(t *testing.T) {
		r, total := buscar(puertos.FiltroBusqueda{})
		igual("orden por fecha, la que no tiene franja al final", r, "sol-2", "sol-1", "sol-3", "sol-4", "sol-5")
		if total != 5 {
			t.Errorf("total %d", total)
		}
	})
	t.Run("texto en español: el plural, las mayúsculas y las tildes no importan", func(t *testing.T) {
		r, _ := buscar(puertos.FiltroBusqueda{Texto: "CAJA"})
		igual("caja", r, "sol-1", "sol-3")
		r, _ = buscar(puertos.FiltroBusqueda{Texto: "mercaderia"}) // sin tilde encuentra "mercadería"
		igual("sin tilde", r, "sol-1")
		r, _ = buscar(puertos.FiltroBusqueda{Texto: "cerro"}) // en el destino
		igual("destino", r, "sol-1", "sol-3")
		r, _ = buscar(puertos.FiltroBusqueda{Texto: "TRF-2026-00999"}) // la referencia externa
		igual("referencia", r, "sol-4")
	})
	t.Run("relevancia: la descripción pesa más que la dirección", func(t *testing.T) {
		// "Norte" está solo en el destino de sol-2; "heladeras" en su descripción. Con las dos
		// palabras (OR de eDisMax), sol-2 queda primero.
		r, _ := buscar(puertos.FiltroBusqueda{Texto: "heladeras cajas", PorRelevancia: true})
		if len(r) != 3 || r[0] != "sol-2" {
			t.Errorf("relevancia: %v", r)
		}
	})
	t.Run("filtros", func(t *testing.T) {
		r, _ := buscar(puertos.FiltroBusqueda{EstadoTraslado: "EN_CURSO"})
		igual("estado", r, "sol-2")
		r, _ = buscar(puertos.FiltroBusqueda{SoloConAlerta: true})
		igual("alerta", r, "sol-4")
		desde, hasta := franja(20, 9), franja(20, 12) // [09:00, 12:00): la de las 10
		r, _ = buscar(puertos.FiltroBusqueda{FranjaDesde: desde, FranjaHasta: hasta})
		igual("rango de fechas", r, "sol-1")
	})
	t.Run("cada organización ve solo lo suyo (CA-25)", func(t *testing.T) {
		r, _ := buscar(puertos.FiltroBusqueda{Organizacion: "org-2"})
		igual("org-2", r, "sol-3")
		r, _ = buscar(puertos.FiltroBusqueda{Organizacion: "org-2", Texto: "heladeras"})
		igual("org-2 no ve las heladeras de org-1", r)
		// {!term}: un valor con sintaxis de Solr se toma literal, no amplía el filtro.
		r, _ = buscar(puertos.FiltroBusqueda{Organizacion: "org-1 OR organizacion_id:org-2"})
		igual("sin inyección en el filtro", r)
	})
	t.Run("paginación", func(t *testing.T) {
		r, total := buscar(puertos.FiltroBusqueda{Desde: 2, Limite: 2})
		igual("página 2", r, "sol-3", "sol-4")
		if total != 5 {
			t.Errorf("total %d", total)
		}
	})
	t.Run("los campos vuelven completos", func(t *testing.T) {
		docs, _, err := c.Buscar(ctx, puertos.FiltroBusqueda{Texto: "tripulación", Limite: 1})
		if err != nil || len(docs) != 1 {
			t.Fatal(err, docs)
		}
		d := docs[0]
		if d.TrasladoID != "tra-sol-4" || d.AlertaClimatica != "NO_OPERABLE" || d.ReferenciaExterna != "TRF-2026-00999" ||
			d.DestinoNombre != "Aeropuerto Internacional" || !d.FranjaInicio.Equal(*franja(21, 8)) || d.Version != 1 {
			t.Errorf("documento: %+v", d)
		}
	})
	t.Run("versión: un evento viejo no pisa uno nuevo y uno repetido no cambia nada", func(t *testing.T) {
		nuevo := doc("sol-2", "org-1", "Heladeras industriales", "Depósito Norte", "COMPLETADO", franja(20, 8), 3)
		if ok, err := c.Indexar(ctx, nuevo); err != nil || !ok {
			t.Fatalf("versión 3: %v %v", ok, err)
		}
		if ok, err := c.Indexar(ctx, nuevo); err != nil || ok {
			t.Fatalf("la misma versión otra vez: %v %v", ok, err)
		}
		viejo := doc("sol-2", "org-1", "Heladeras industriales", "Depósito Norte", "EN_CURSO", franja(20, 8), 2)
		if ok, err := c.Indexar(ctx, viejo); err != nil || ok {
			t.Fatalf("versión 2 después de la 3: %v %v", ok, err)
		}
		r, _ := buscar(puertos.FiltroBusqueda{EstadoTraslado: "COMPLETADO"})
		igual("quedó la versión nueva", r, "sol-2")
	})
	t.Run("escrituras concurrentes del mismo documento: gana la versión mayor", func(t *testing.T) {
		var espera sync.WaitGroup
		for v := int64(4); v <= 13; v++ {
			espera.Go(func() {
				d := doc("sol-1", "org-1", "Cajas de mercadería seca", "Sucursal Cerro", fmt.Sprintf("V%d", v), franja(20, 10), v)
				if _, err := c.Indexar(ctx, d); err != nil {
					t.Errorf("versión %d: %v", v, err)
				}
			})
		}
		espera.Wait()
		docs, _, err := c.Buscar(ctx, puertos.FiltroBusqueda{Texto: "mercadería", Limite: 1})
		if err != nil || len(docs) != 1 || docs[0].Version != 13 || docs[0].EstadoTraslado != "V13" {
			t.Fatalf("después de las escrituras concurrentes: %v %+v", err, docs)
		}
	})
}

func TestSolrCaido(t *testing.T) {
	c := NuevoCliente("http://127.0.0.1:1/solr/traslados", &http.Client{Timeout: time.Second})
	if _, _, err := c.Buscar(context.Background(), puertos.FiltroBusqueda{Limite: 10}); !errors.Is(err, puertos.ErrBusquedaNoDisponible) {
		t.Fatalf("buscar con Solr caído: %v", err)
	}
	inicio := franja(20, 10)
	if _, err := c.Indexar(context.Background(), doc("sol-1", "org-1", "x", "y", "PROGRAMADO", inicio, 1)); !errors.Is(err, puertos.ErrBusquedaNoDisponible) {
		t.Fatalf("indexar con Solr caído: %v", err)
	}
}
