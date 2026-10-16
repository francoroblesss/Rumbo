package mensajeria

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	tcrabbit "github.com/testcontainers/testcontainers-go/modules/rabbitmq"
)

// Tests de integración con un RabbitMQ real (rabbitmq:3-management, la misma imagen que
// compose.yaml). Necesitan Docker corriendo. Prueban lo que promete ADR-004: el consumidor y el
// publicador sobreviven a la caída de la conexión, los errores transitorios se reintentan y los
// permanentes van a la DLQ.

func rabbitDePrueba(t *testing.T) (*tcrabbit.RabbitMQContainer, string) {
	t.Helper()
	if testing.Short() {
		t.Skip("test de integración con contenedores")
	}
	ctx := context.Background()
	contenedor, err := tcrabbit.Run(ctx, "rabbitmq:3-management")
	if err != nil {
		t.Fatalf("levantar RabbitMQ: %v", err)
	}
	t.Cleanup(func() { _ = contenedor.Terminate(context.Background()) })
	uri, err := contenedor.AmqpURL(ctx)
	if err != nil {
		t.Fatalf("URL de RabbitMQ: %v", err)
	}
	return contenedor, uri
}

// recibidos junta los ids de los sobres que llegan al manejador.
type recibidos struct {
	mu  sync.Mutex
	ids []string
	ch  chan string
}

func nuevosRecibidos() *recibidos { return &recibidos{ch: make(chan string, 100)} }

func (r *recibidos) manejar(_ context.Context, s Sobre) error {
	r.mu.Lock()
	r.ids = append(r.ids, s.ID)
	r.mu.Unlock()
	r.ch <- s.ID
	return nil
}

func esperarID(t *testing.T, ch <-chan string, id string, limite time.Duration) {
	t.Helper()
	tope := time.After(limite)
	for {
		select {
		case llegado := <-ch:
			if llegado == id {
				return
			}
		case <-tope:
			t.Fatalf("no llegó el mensaje %s en %s", id, limite)
		}
	}
}

// publicarHasta reintenta la publicación (como hace el relay del outbox) hasta que el broker la
// confirme o se agote el tiempo.
func publicarHasta(t *testing.T, p *Publicador, cola string, s Sobre, limite time.Duration) {
	t.Helper()
	tope := time.Now().Add(limite)
	for {
		err := p.Publicar(context.Background(), "", cola, s)
		if err == nil {
			return
		}
		if time.Now().After(tope) {
			t.Fatalf("no se pudo publicar %s: %v", s.ID, err)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func sobreDePrueba(t *testing.T, id string) Sobre {
	t.Helper()
	s, err := NuevoSobre(context.Background(), "prueba.algo_paso", id, 1, map[string]string{"id": id})
	if err != nil {
		t.Fatal(err)
	}
	s.ID = id
	return s
}

func TestSinBrokerPublicarFallaSinBloquear(t *testing.T) {
	ctx, cancelar := context.WithCancel(context.Background())
	defer cancelar()
	conexion := Conectar(ctx, "amqp://nadie:nada@127.0.0.1:1/")
	defer conexion.Cerrar()
	p := NuevoPublicador(conexion)
	if err := p.DeclararCola("cola"); err != nil {
		t.Fatalf("declarar sin conexión no tiene que fallar: %v", err)
	}

	inicio := time.Now()
	err := p.Publicar(ctx, "", "cola", sobreDePrueba(t, "m-1"))
	if !errors.Is(err, ErrSinConexion) {
		t.Fatalf("se esperaba ErrSinConexion, llegó %v", err)
	}
	if time.Since(inicio) > time.Second {
		t.Fatalf("Publicar sin broker tardó %s: tiene que fallar enseguida", time.Since(inicio))
	}
}

func TestReconectaDespuesDeQueElBrokerCierraLaConexion(t *testing.T) {
	contenedor, uri := rabbitDePrueba(t)
	ctx, cancelar := context.WithCancel(context.Background())
	defer cancelar()

	conexion := Conectar(ctx, uri)
	defer conexion.Cerrar()
	const cola = "prueba.reconexion"
	p := NuevoPublicador(conexion)
	defer p.Cerrar()
	if err := p.DeclararCola(cola); err != nil {
		t.Fatal(err)
	}
	r := nuevosRecibidos()
	go Consumir(ctx, conexion, ConfigConsumidor{Cola: cola}, r.manejar)

	publicarHasta(t, p, cola, sobreDePrueba(t, "antes"), 30*time.Second)
	esperarID(t, r.ch, "antes", 30*time.Second)

	// El broker corta todas las conexiones, como cuando se reinicia.
	if codigo, _, err := contenedor.Exec(ctx, []string{"rabbitmqctl", "close_all_connections", "prueba"}); err != nil || codigo != 0 {
		t.Fatalf("cerrar conexiones: código %d, %v", codigo, err)
	}

	// Sin reiniciar nada, el publicador y el consumidor vuelven a funcionar.
	publicarHasta(t, p, cola, sobreDePrueba(t, "despues"), 60*time.Second)
	esperarID(t, r.ch, "despues", 60*time.Second)
}

func TestErrorTransitorioSeReintentaYPermanenteVaALaDLQ(t *testing.T) {
	_, uri := rabbitDePrueba(t)
	ctx, cancelar := context.WithCancel(context.Background())
	defer cancelar()

	conexion := Conectar(ctx, uri)
	defer conexion.Cerrar()
	const cola = "prueba.fallas"
	p := NuevoPublicador(conexion)
	defer p.Cerrar()
	if err := p.DeclararCola(cola); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	intentos := map[string]int{}
	listo := make(chan string, 10)
	manejar := func(_ context.Context, s Sobre) error {
		mu.Lock()
		intentos[s.ID]++
		n := intentos[s.ID]
		mu.Unlock()
		switch s.ID {
		case "transitorio":
			if n == 1 {
				return errors.New("la base no respondió")
			}
			listo <- s.ID
			return nil
		default:
			return Permanente(errors.New("datos imposibles"))
		}
	}
	go Consumir(ctx, conexion, ConfigConsumidor{Cola: cola}, manejar)

	// Transitorio: falla una vez, espera en la cola de reintento (~1 s) y se procesa.
	publicarHasta(t, p, cola, sobreDePrueba(t, "transitorio"), 30*time.Second)
	esperarID(t, listo, "transitorio", 30*time.Second)

	// Permanente: va directo a la DLQ, sin reintentos.
	publicarHasta(t, p, cola, sobreDePrueba(t, "permanente"), 30*time.Second)
	dlq := nuevosRecibidos()
	go Consumir(ctx, conexion, ConfigConsumidor{Cola: cola + ".dlq"}, dlq.manejar)
	esperarID(t, dlq.ch, "permanente", 30*time.Second)

	mu.Lock()
	defer mu.Unlock()
	if intentos["transitorio"] != 2 {
		t.Errorf("transitorio: %d intentos, se esperaban 2", intentos["transitorio"])
	}
	if intentos["permanente"] != 1 {
		t.Errorf("permanente: %d intentos, se esperaba 1 (sin reintentos)", intentos["permanente"])
	}
}
