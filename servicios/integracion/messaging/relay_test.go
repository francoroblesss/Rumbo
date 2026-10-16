package messaging

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/francoroblesss/Rumbo/servicios/comun/mensajeria"
	"github.com/francoroblesss/Rumbo/servicios/integracion/models"
	"github.com/francoroblesss/Rumbo/servicios/integracion/repositories"
)

// publicadorFalso guarda lo publicado. Con Error distinto de nil simula RabbitMQ caído.
type publicadorFalso struct {
	mu         sync.Mutex
	Error      error
	intentos   int
	publicados []mensajeria.Sobre
	colas      []string
}

func (p *publicadorFalso) DeclararCola(string) error { return nil }

func (p *publicadorFalso) Publicar(_ context.Context, _, routingKey string, s mensajeria.Sobre) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.intentos++
	if p.Error != nil {
		return p.Error
	}
	p.publicados = append(p.publicados, s)
	p.colas = append(p.colas, routingKey)
	return nil
}

func pendienteDePrueba(t *testing.T, repo *repositories.SolicitudesMock, id string) {
	t.Helper()
	_, err := repo.Crear(context.Background(), models.SolicitudExterna{
		ID:             id,
		OrganizacionID: "org-almacenamiento",
		Pedido: models.PedidoTraslado{
			Cosa:    models.Cosa{Descripcion: "cajas", Cantidad: 4},
			Origen:  models.Ubicacion{Nombre: "Depósito", Direccion: "Av. Colón 1000"},
			Destino: models.Ubicacion{Nombre: "Sucursal", Direccion: "Obispo Trejo 300"},
		},
		Idempotencia:   models.ClaveIdempotencia{Clave: "clave-" + id},
		PendienteEnvio: true,
		CreadoEn:       time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
}

func pendientes(t *testing.T, repo *repositories.SolicitudesMock) int64 {
	t.Helper()
	n, err := repo.ContarPendientes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestRelayPublicaElComandoYMarcaEnviadas(t *testing.T) {
	repo := repositories.NewSolicitudesMock()
	pendienteDePrueba(t, repo, "sol-1")
	pendienteDePrueba(t, repo, "sol-2")
	pub := &publicadorFalso{}

	NewRelay(repo, pub, "integracion-1").publicarPendientes(context.Background())

	if len(pub.publicados) != 2 {
		t.Fatalf("se publicaron %d comandos, se esperaban 2", len(pub.publicados))
	}
	if n := pendientes(t, repo); n != 0 {
		t.Fatalf("quedaron %d pendientes, se esperaban 0", n)
	}
	s := pub.publicados[0]
	if s.Tipo != TipoSolicitudRecibida || pub.colas[0] != ColaSolicitudesExternas {
		t.Fatalf("comando %q en %q, se esperaba %q en %q", s.Tipo, pub.colas[0], TipoSolicitudRecibida, ColaSolicitudesExternas)
	}
	var datos datosSolicitudRecibida
	if err := s.LeerDatos(&datos); err != nil {
		t.Fatal(err)
	}
	if datos.SolicitudID != s.AgregadoID || datos.OrganizacionID != "org-almacenamiento" || datos.Cosa.Cantidad != 4 {
		t.Fatalf("datos del comando inesperados: %+v", datos)
	}
}

// Con el broker caído la solicitud no se pierde: queda pendiente (outbox) y el relay no insiste
// en el mismo ciclo.
func TestRelayConBrokerCaidoDejaLaSolicitudPendiente(t *testing.T) {
	repo := repositories.NewSolicitudesMock()
	pendienteDePrueba(t, repo, "sol-1")
	pub := &publicadorFalso{Error: mensajeria.ErrSinConexion}
	relay := NewRelay(repo, pub, "integracion-1")

	relay.publicarPendientes(context.Background())

	if pub.intentos != 1 {
		t.Fatalf("hubo %d intentos de publicación, se esperaba 1 por ciclo", pub.intentos)
	}
	if n := pendientes(t, repo); n != 1 {
		t.Fatalf("hay %d pendientes, se esperaba 1: la solicitud no puede perderse", n)
	}

	// Mientras el reclamo está vigente (30 s), ninguna réplica la vuelve a tomar: no hay envío doble.
	pub.Error = nil
	relay.publicarPendientes(context.Background())
	NewRelay(repo, pub, "integracion-2").publicarPendientes(context.Background())
	if len(pub.publicados) != 0 {
		t.Fatalf("se publicó %d veces con el reclamo vigente", len(pub.publicados))
	}
}

func TestRelayConMongoCaidoNoPublica(t *testing.T) {
	repo := repositories.NewSolicitudesMock()
	repo.Error = errors.New("MongoDB no responde")
	pub := &publicadorFalso{}

	NewRelay(repo, pub, "integracion-1").publicarPendientes(context.Background())

	if pub.intentos != 0 {
		t.Fatalf("hubo %d intentos de publicación sin poder leer los pendientes", pub.intentos)
	}
}
