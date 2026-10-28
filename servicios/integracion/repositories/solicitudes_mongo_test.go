package repositories

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/francoroblesss/Rumbo/servicios/integracion/models"
)

// Tests de integración contra un MongoDB real (mongo:7, la misma imagen que compose.yaml).
// Necesitan Docker corriendo. Cada test usa una base nueva en el mismo contenedor.

var (
	clienteMongo *mongo.Client
	errMongo     error
	unaVez       sync.Once
)

func repoMongoDePrueba(t *testing.T) *SolicitudesMongo {
	t.Helper()
	if testing.Short() {
		t.Skip("test de integración: se omite con -short")
	}
	unaVez.Do(func() {
		ctx := context.Background()
		contenedor, err := mongodb.Run(ctx, "mongo:7")
		if err != nil {
			errMongo = err
			return
		}
		uri, err := contenedor.ConnectionString(ctx)
		if err != nil {
			errMongo = err
			return
		}
		clienteMongo, errMongo = mongo.Connect(ctx, options.Client().ApplyURI(uri))
	})
	if errMongo != nil {
		t.Fatalf("no se pudo levantar MongoDB con testcontainers (¿Docker está corriendo?): %v", errMongo)
	}
	repo := NewSolicitudesMongo(clienteMongo.Database(fmt.Sprintf("prueba_%d", time.Now().UnixNano())))
	if err := repo.AsegurarIndices(context.Background()); err != nil {
		t.Fatal(err)
	}
	return repo
}

func nuevaSolicitud(id, org, clave string, actualizado time.Time) models.SolicitudExterna {
	return models.SolicitudExterna{
		ID: id, OrganizacionID: org, Version: 1,
		Idempotencia: models.ClaveIdempotencia{Clave: clave, HashCuerpo: "hash-" + id},
		Vista: models.VistaSolicitud{ID: id, Estado: "ACEPTADA", Version: 1, CreadoEn: actualizado, ActualizadoEn: actualizado,
			Traslado: &models.Traslado{Estado: models.EstadoProgramado}},
		CreadoEn:      actualizado,
		ActualizadoEn: actualizado,
	}
}

func TestMongoIndiceUnicoDeIdempotencia(t *testing.T) {
	repo := repoMongoDePrueba(t)
	ctx := context.Background()
	ahora := time.Now().UTC().Truncate(time.Millisecond)
	if _, err := repo.Crear(ctx, nuevaSolicitud("sol-1", "org-1", "clave-0001", ahora)); err != nil {
		t.Fatal(err)
	}
	existente, err := repo.Crear(ctx, nuevaSolicitud("sol-2", "org-1", "clave-0001", ahora))
	if !errors.Is(err, ErrClaveExistente) || existente.ID != "sol-1" || existente.Idempotencia.HashCuerpo != "hash-sol-1" {
		t.Fatalf("se esperaba ErrClaveExistente con la solicitud original: %+v %v", existente, err)
	}
	if _, err := repo.Crear(ctx, nuevaSolicitud("sol-3", "org-2", "clave-0001", ahora)); err != nil {
		t.Fatalf("la misma clave en otra organización es válida: %v", err)
	}
	if _, err := repo.BuscarPorID(ctx, "org-2", "sol-1"); !errors.Is(err, ErrNoEncontrada) {
		t.Fatalf("una organización no ve las solicitudes de otra: %v", err)
	}
}

func TestMongoBuscarPorClave(t *testing.T) {
	repo := repoMongoDePrueba(t)
	ctx := context.Background()
	ahora := time.Now().UTC().Truncate(time.Millisecond)
	if _, err := repo.Crear(ctx, nuevaSolicitud("sol-1", "org-1", "clave-0001", ahora)); err != nil {
		t.Fatal(err)
	}
	s, err := repo.BuscarPorClave(ctx, "org-1", "clave-0001")
	if err != nil || s.ID != "sol-1" || s.Idempotencia.HashCuerpo != "hash-sol-1" {
		t.Fatalf("no se encontró por clave: %+v %v", s, err)
	}
	if _, err := repo.BuscarPorClave(ctx, "org-2", "clave-0001"); !errors.Is(err, ErrNoEncontrada) {
		t.Fatalf("la clave es por organización: %v", err)
	}
}

func TestMongoAplicarVistaIgnoraVersionesViejas(t *testing.T) {
	repo := repoMongoDePrueba(t)
	ctx := context.Background()
	ahora := time.Now().UTC().Truncate(time.Millisecond)
	if _, err := repo.Crear(ctx, nuevaSolicitud("sol-1", "org-1", "clave-0001", ahora)); err != nil {
		t.Fatal(err)
	}
	asignado := models.VistaSolicitud{ID: "sol-1", Version: 3, Traslado: &models.Traslado{Estado: models.EstadoAsignado}}
	org, aplicado, err := repo.AplicarVista(ctx, "sol-1", 3, asignado, ahora.Add(time.Minute))
	if err != nil || !aplicado || org != "org-1" {
		t.Fatalf("la versión 3 se tenía que aplicar: %q %v %v", org, aplicado, err)
	}
	for _, version := range []int64{3, 2, 1} {
		viejo := models.VistaSolicitud{Traslado: &models.Traslado{Estado: models.EstadoProgramado}}
		_, aplicado, err := repo.AplicarVista(ctx, "sol-1", version, viejo, ahora.Add(2*time.Minute))
		if err != nil || aplicado {
			t.Fatalf("la versión %d es vieja o repetida y no se tenía que aplicar: %v %v", version, aplicado, err)
		}
	}
	s, _ := repo.BuscarPorID(ctx, "org-1", "sol-1")
	if s.Version != 3 || s.Vista.Traslado.Estado != models.EstadoAsignado || !s.ActualizadoEn.Equal(ahora.Add(time.Minute)) {
		t.Fatalf("estado incorrecto: versión %d, %s, %v", s.Version, s.Vista.Traslado.Estado, s.ActualizadoEn)
	}
	if _, aplicado, err := repo.AplicarVista(ctx, "sol-no-existe", 5, models.VistaSolicitud{}, ahora); err != nil || aplicado {
		t.Fatalf("una solicitud desconocida no se aplica ni da error: %v %v", aplicado, err)
	}
}

func TestMongoListarConActualizadoDesdeYPaginacion(t *testing.T) {
	repo := repoMongoDePrueba(t)
	ctx := context.Background()
	base := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		if _, err := repo.Crear(ctx, nuevaSolicitud(fmt.Sprintf("sol-%d", i), "org-1", fmt.Sprintf("clave-%04d", i), base.Add(time.Duration(i)*time.Minute))); err != nil {
			t.Fatal(err)
		}
	}
	_, _ = repo.Crear(ctx, nuevaSolicitud("sol-otra", "org-2", "clave-9999", base))

	pagina, total, err := repo.Listar(ctx, FiltroListado{OrganizacionID: "org-1", Pagina: 1, Tamanio: 2})
	if err != nil || total != 5 || len(pagina) != 2 || pagina[0].ID != "sol-0" || pagina[1].ID != "sol-1" {
		t.Fatalf("primera página incorrecta: %d %v %v", total, ids(pagina), err)
	}
	pagina, _, _ = repo.Listar(ctx, FiltroListado{OrganizacionID: "org-1", Pagina: 3, Tamanio: 2})
	if len(pagina) != 1 || pagina[0].ID != "sol-4" {
		t.Fatalf("última página incorrecta: %v", ids(pagina))
	}

	// actualizado_desde es exclusivo: la de las 12:02 no entra.
	desde := base.Add(2 * time.Minute)
	pagina, total, _ = repo.Listar(ctx, FiltroListado{OrganizacionID: "org-1", ActualizadoDesde: &desde, Pagina: 1, Tamanio: 20})
	if total != 2 || len(pagina) != 2 || pagina[0].ID != "sol-3" || pagina[1].ID != "sol-4" {
		t.Fatalf("filtro actualizado_desde incorrecto: %d %v", total, ids(pagina))
	}

	completado := models.VistaSolicitud{ID: "sol-0", Traslado: &models.Traslado{Estado: models.EstadoCompletado}}
	_, _, _ = repo.AplicarVista(ctx, "sol-0", 2, completado, base.Add(time.Hour))
	pagina, total, _ = repo.Listar(ctx, FiltroListado{OrganizacionID: "org-1", Estado: models.EstadoCompletado, Pagina: 1, Tamanio: 20})
	if total != 1 || pagina[0].ID != "sol-0" {
		t.Fatalf("filtro por estado incorrecto: %d %v", total, ids(pagina))
	}
}

func ids(solicitudes []models.SolicitudExterna) []string {
	var r []string
	for _, s := range solicitudes {
		r = append(r, s.ID)
	}
	return r
}
