package services

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

	"github.com/francoroblesss/Rumbo/servicios/comun/identidad"
	"github.com/francoroblesss/Rumbo/servicios/notificaciones/repositories"
)

// Tests de integración contra un MongoDB real (mongo:7, la misma imagen que compose.yaml): el
// servicio con el repositorio de verdad. Necesitan Docker corriendo. Cada test usa una base nueva
// en el mismo contenedor.

var (
	clienteMongo *mongo.Client
	errMongo     error
	unaVez       sync.Once
)

func servicioConMongo(t *testing.T, ahora func() time.Time) (*AvisosService, *repositories.AvisosMongo) {
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
	repo := repositories.NewAvisosMongo(clienteMongo.Database(fmt.Sprintf("prueba_%d", time.Now().UnixNano())))
	if err := repo.AsegurarIndices(context.Background()); err != nil {
		t.Fatal(err)
	}
	return NewAvisosService(repo, ahora), repo
}

var (
	operador      = identidad.Identidad{Usuario: "usr-op", Roles: []string{"operador"}}
	administrador = identidad.Identidad{Usuario: "usr-admin", Roles: []string{"administrador"}}
	clienteOrg    = identidad.Identidad{Usuario: "usr-cli", Organizacion: "org-almacenamiento", Roles: []string{"cliente"}}
	clienteOtra   = identidad.Identidad{Usuario: "usr-otra", Organizacion: "org-otra", Roles: []string{"cliente"}}
)

// CA-42: el mismo evento entregado dos veces deja un solo aviso.
func TestMongoEventoRepetidoDejaUnAviso(t *testing.T) {
	s, _ := servicioConMongo(t, time.Now)
	ctx := context.Background()
	ev := sobre(t, "operaciones.solicitud.aceptada", vista("PROGRAMADO", nil))
	for i := 0; i < 2; i++ {
		if err := s.ProcesarEvento(ctx, ev); err != nil {
			t.Fatalf("entrega %d: %v", i+1, err)
		}
	}
	p, err := s.Listar(ctx, operador, false, 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if p.Total != 1 || p.Resultados[0].ID != idMensaje || p.Resultados[0].Titulo != "Reserva nueva" {
		t.Fatalf("se esperaba un solo aviso: %+v", p)
	}
}

func TestMongoVisibilidad(t *testing.T) {
	s, _ := servicioConMongo(t, time.Now)
	ctx := context.Background()
	// "Reserva nueva" (ROL operador) y "Traslado en curso" (ORGANIZACION org-almacenamiento).
	if err := s.ProcesarEvento(ctx, sobre(t, "operaciones.solicitud.aceptada", vista("PROGRAMADO", nil))); err != nil {
		t.Fatal(err)
	}
	enCurso := sobre(t, "operaciones.traslado.estado_cambiado", vista("EN_CURSO", nil))
	enCurso.ID = "otro-mensaje"
	if err := s.ProcesarEvento(ctx, enCurso); err != nil {
		t.Fatal(err)
	}

	titulos := func(id identidad.Identidad) []string {
		p, err := s.Listar(ctx, id, false, 1, 20)
		if err != nil {
			t.Fatal(err)
		}
		var r []string
		for _, a := range p.Resultados {
			r = append(r, a.Titulo)
		}
		return r
	}
	if got := titulos(clienteOrg); len(got) != 1 || got[0] != "Traslado en curso" {
		t.Errorf("el cliente ve solo los de su organización, no los del operador: %v", got)
	}
	if got := titulos(clienteOtra); len(got) != 0 {
		t.Errorf("un cliente de otra organización no ve nada: %v", got)
	}
	if got := titulos(operador); len(got) != 1 || got[0] != "Reserva nueva" {
		t.Errorf("el operador ve los de ROL operador: %v", got)
	}
	if got := titulos(administrador); len(got) != 1 || got[0] != "Reserva nueva" {
		t.Errorf("el administrador ve los del operador: %v", got)
	}
	if err := s.MarcarLeido(ctx, clienteOtra, "otro-mensaje"); !errors.Is(err, ErrNoEncontrado) {
		t.Errorf("marcar un aviso de otra organización da no encontrado: %v", err)
	}
}

func TestMongoNoLeidosYLectura(t *testing.T) {
	s, _ := servicioConMongo(t, time.Now)
	ctx := context.Background()
	for _, id := range []string{"m-1", "m-2"} {
		ev := sobre(t, "operaciones.solicitud.aceptada", vista("PROGRAMADO", nil))
		ev.ID = id
		if err := s.ProcesarEvento(ctx, ev); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.MarcarLeido(ctx, operador, "m-1"); err != nil {
		t.Fatal(err)
	}
	if err := s.MarcarLeido(ctx, operador, "m-1"); err != nil {
		t.Fatalf("marcar de nuevo uno ya leído no es error: %v", err)
	}
	if err := s.MarcarLeido(ctx, operador, "no-existe"); !errors.Is(err, ErrNoEncontrado) {
		t.Fatalf("uno que no existe da no encontrado: %v", err)
	}
	p, err := s.Listar(ctx, operador, true, 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if p.Total != 1 || p.Resultados[0].ID != "m-2" || p.Resultados[0].Leido {
		t.Fatalf("no_leidos deja solo m-2: %+v", p)
	}
	// leido es compartido: el administrador (que ve los del operador) lo ve leído también.
	todos, _ := s.Listar(ctx, administrador, false, 1, 20)
	for _, a := range todos.Resultados {
		if a.ID == "m-1" && !a.Leido {
			t.Errorf("el leído es compartido por el destinatario: %+v", a)
		}
	}
}

func TestMongoOrdenYPosteriores(t *testing.T) {
	reloj := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	s, _ := servicioConMongo(t, func() time.Time { return reloj })
	ctx := context.Background()
	for i, id := range []string{"m-1", "m-2", "m-3"} {
		reloj = time.Date(2026, 11, 20, 12, 0, i, 0, time.UTC)
		ev := sobre(t, "operaciones.solicitud.aceptada", vista("PROGRAMADO", nil))
		ev.ID = id
		if err := s.ProcesarEvento(ctx, ev); err != nil {
			t.Fatal(err)
		}
	}
	p, _ := s.Listar(ctx, operador, false, 1, 2)
	if p.Total != 3 || len(p.Resultados) != 2 || p.Resultados[0].ID != "m-3" || p.Resultados[1].ID != "m-2" {
		t.Fatalf("la bandeja va de más nuevo a más viejo, paginada: %+v", p)
	}
	desde, _ := s.CursorInicial(ctx, operador, "m-1")
	nuevos, err := s.Posteriores(ctx, operador, desde, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(nuevos) != 2 || nuevos[0].ID != "m-2" || nuevos[1].ID != "m-3" {
		t.Fatalf("después de m-1 vienen m-2 y m-3, en orden: %+v", nuevos)
	}
	if otros, _ := s.Posteriores(ctx, clienteOtra, repositories.Cursor{}, 100); len(otros) != 0 {
		t.Fatalf("el stream tampoco muestra avisos ajenos: %+v", otros)
	}
}
