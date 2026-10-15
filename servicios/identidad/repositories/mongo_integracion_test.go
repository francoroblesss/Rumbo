package repositories_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/francoroblesss/Rumbo/servicios/identidad/database"
	"github.com/francoroblesss/Rumbo/servicios/identidad/models"
	"github.com/francoroblesss/Rumbo/servicios/identidad/repositories"
	"github.com/francoroblesss/Rumbo/servicios/identidad/services"
)

// Test de integración (consigna §2.11): usa un MongoDB real en un contenedor, no un mock.
// Requiere Docker en ejecución.
func baseReal(t *testing.T) *mongo.Database {
	t.Helper()
	ctx := context.Background()

	contenedor, err := mongodb.Run(ctx, "mongo:7")
	testcontainers.CleanupContainer(t, contenedor)
	if err != nil {
		t.Fatalf("no se pudo levantar MongoDB: %v", err)
	}
	uri, err := contenedor.ConnectionString(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cliente, err := database.Conectar(ctx, uri)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cliente.Disconnect(context.Background()) })
	return cliente.Database("identidad")
}

func TestIndicesUnicosRechazanDuplicados(t *testing.T) {
	ctx := context.Background()
	base := baseReal(t)
	usuarios, err := repositories.NuevoUsuariosMongo(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	organizaciones, err := repositories.NuevoOrganizacionesMongo(ctx, base)
	if err != nil {
		t.Fatal(err)
	}

	nuevoUsuario := func(id string) *models.Usuario {
		return &models.Usuario{ID: id, Email: "ana@rumbo.local", Roles: []string{"operador"}, CreadoEn: time.Now().UTC()}
	}
	if err := usuarios.Crear(ctx, nuevoUsuario("usr-1")); err != nil {
		t.Fatal(err)
	}
	if err := usuarios.Crear(ctx, nuevoUsuario("usr-2")); !errors.Is(err, repositories.ErrDuplicado) {
		t.Errorf("dos usuarios con el mismo email: error = %v, esperaba ErrDuplicado", err)
	}

	conCliente := func(id, clientID string) *models.Organizacion {
		return &models.Organizacion{
			ID: id, Nombre: id, TipoServicio: models.ServicioStock, CreadoEn: time.Now().UTC(),
			ClientesAPI: []models.ClienteAPI{{ClientID: clientID, HashSecreto: "hash"}},
		}
	}
	if err := organizaciones.Crear(ctx, conCliente("org-1", "mismo-client-id")); err != nil {
		t.Fatal(err)
	}
	if err := organizaciones.Crear(ctx, conCliente("org-2", "mismo-client-id")); !errors.Is(err, repositories.ErrDuplicado) {
		t.Errorf("dos organizaciones con el mismo client_id: error = %v, esperaba ErrDuplicado", err)
	}

	// Las organizaciones sin credenciales de API (clientes comunes) no chocan entre sí.
	for _, id := range []string{"org-3", "org-4"} {
		sinAPI := &models.Organizacion{ID: id, Nombre: id, TipoServicio: models.ServicioEscolar, CreadoEn: time.Now().UTC()}
		if err := organizaciones.Crear(ctx, sinAPI); err != nil {
			t.Errorf("organización sin credenciales de API %s: %v", id, err)
		}
	}
}

func TestSembrarDosVecesNoDuplicaYPermiteAutenticar(t *testing.T) {
	ctx := context.Background()
	base := baseReal(t)
	usuarios, err := repositories.NuevoUsuariosMongo(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	organizaciones, err := repositories.NuevoOrganizacionesMongo(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	servicio := services.NuevoAuthService(usuarios, organizaciones, "secreto-de-prueba", services.Siembra{
		AdminEmail: "admin@rumbo.local", AdminPassword: "clave-admin",
		ExternoClientID: "externo", ExternoClientSecret: "secreto-externo",
	})

	for i := 0; i < 2; i++ {
		if err := servicio.Sembrar(ctx); err != nil {
			t.Fatalf("sembrar (vez %d): %v", i+1, err)
		}
	}

	if n, err := base.Collection("usuarios").CountDocuments(ctx, bson.D{}); err != nil || n != 1 {
		t.Errorf("usuarios = %d (error %v), esperaba 1", n, err)
	}
	if n, err := base.Collection("organizaciones").CountDocuments(ctx, bson.D{}); err != nil || n != 1 {
		t.Errorf("organizaciones = %d (error %v), esperaba 1", n, err)
	}

	if _, err := servicio.Login(ctx, "admin@rumbo.local", "clave-admin"); err != nil {
		t.Errorf("login del administrador sembrado: %v", err)
	}
	if _, err := servicio.TokenCliente(ctx, "externo", "secreto-externo"); err != nil {
		t.Errorf("token del sistema externo sembrado: %v", err)
	}
}
