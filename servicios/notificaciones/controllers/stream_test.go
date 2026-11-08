package controllers

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/francoroblesss/Rumbo/servicios/notificaciones/models"
	"github.com/francoroblesss/Rumbo/servicios/notificaciones/repositories"
)

// Tests del stream SSE con un servidor HTTP de verdad (httptest.NewServer) y el repositorio en
// memoria. Todos tienen plazo: si el handler no termina o no manda lo esperado, fallan en lugar
// de quedar colgados.

const plazo = 3 * time.Second

// abrirStream abre GET /avisos/stream como operador y devuelve las líneas que llegan y la
// función que cierra la conexión del cliente.
func abrirStream(t *testing.T, url, ultimoID string) (*http.Response, <-chan string, context.CancelFunc) {
	t.Helper()
	ctx, cerrar := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url+"/avisos/stream", nil)
	req.Header.Set("X-Rumbo-Roles", "operador")
	if ultimoID != "" {
		req.Header.Set("Last-Event-ID", ultimoID)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cerrar()
		t.Fatal(err)
	}
	lineas := make(chan string, 100)
	go func() {
		defer close(lineas)
		lector := bufio.NewScanner(resp.Body)
		for lector.Scan() {
			select {
			case lineas <- lector.Text():
			case <-ctx.Done(): // nadie lee más: no quedar bloqueado
				return
			}
		}
	}()
	t.Cleanup(func() { cerrar(); resp.Body.Close() })
	return resp, lineas, cerrar
}

// servidor levanta el router en un servidor HTTP de verdad. Se cierra con t.Cleanup y no con
// defer: así corre después de cerrar el cliente (Cleanup es LIFO); si no, Close esperaría a que
// termine un stream que nadie cerró.
func servidor(t *testing.T, r *gin.Engine) *httptest.Server {
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv
}

type evento struct {
	nombre, id string
	aviso      models.Aviso
}

// siguienteEvento lee hasta la próxima línea en blanco que cierre un evento "aviso" (los
// comentarios ": ping" se saltean).
func siguienteEvento(t *testing.T, lineas <-chan string) evento {
	t.Helper()
	var e evento
	limite := time.After(plazo)
	for {
		select {
		case l, ok := <-lineas:
			if !ok {
				t.Fatal("el stream se cerró antes de recibir el aviso")
			}
			switch {
			case strings.HasPrefix(l, "event: "):
				e.nombre = strings.TrimPrefix(l, "event: ")
			case strings.HasPrefix(l, "id: "):
				e.id = strings.TrimPrefix(l, "id: ")
			case strings.HasPrefix(l, "data: "):
				if err := json.Unmarshal([]byte(strings.TrimPrefix(l, "data: ")), &e.aviso); err != nil {
					t.Fatalf("data no es un aviso JSON en una línea: %q", l)
				}
			case l == "" && e.nombre != "":
				return e
			}
		case <-limite:
			t.Fatal("no llegó ningún aviso a tiempo")
		}
	}
}

func TestStreamMandaElAvisoNuevo(t *testing.T) {
	repo := repositories.NewAvisosMock()
	srv := servidor(t, routerDePrueba(repo))

	resp, lineas, _ := abrirStream(t, srv.URL, "")
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "text/event-stream" ||
		resp.Header.Get("Cache-Control") != "no-cache" || resp.Header.Get("X-Accel-Buffering") != "no" {
		t.Fatalf("estado o headers del stream: %d %v", resp.StatusCode, resp.Header)
	}
	// Uno viejo (anterior a abrir) no se manda; el nuevo sí, y uno de otra organización no.
	_ = repo.Insertar(context.Background(), models.Aviso{ID: "viejo", Destinatario: alOperador, CreadoEn: time.Now().Add(-time.Hour)})
	_ = repo.Insertar(context.Background(), models.Aviso{ID: "ajeno", Destinatario: aLaOrg, CreadoEn: time.Now()})
	_ = repo.Insertar(context.Background(), models.Aviso{ID: "nuevo", Destinatario: alOperador, Titulo: "Reserva nueva", CreadoEn: time.Now()})

	e := siguienteEvento(t, lineas)
	if e.nombre != "aviso" || e.id != "nuevo" || e.aviso.ID != "nuevo" || e.aviso.Titulo != "Reserva nueva" {
		t.Fatalf("evento recibido: %+v", e)
	}
}

func TestStreamReenviaDesdeLastEventID(t *testing.T) {
	repo := repositories.NewAvisosMock()
	ahora := time.Now()
	for i, id := range []string{"a-1", "a-2", "a-3"} {
		_ = repo.Insertar(context.Background(), models.Aviso{ID: id, Destinatario: alOperador, CreadoEn: ahora.Add(time.Duration(i-10) * time.Second)})
	}
	srv := servidor(t, routerDePrueba(repo))

	_, lineas, _ := abrirStream(t, srv.URL, "a-1")
	if e := siguienteEvento(t, lineas); e.id != "a-2" {
		t.Fatalf("primero llega a-2: %+v", e)
	}
	if e := siguienteEvento(t, lineas); e.id != "a-3" {
		t.Fatalf("después llega a-3: %+v", e)
	}
}

func TestStreamMandaPing(t *testing.T) {
	srv := servidor(t, routerDePrueba(repositories.NewAvisosMock()))

	_, lineas, _ := abrirStream(t, srv.URL, "")
	limite := time.After(plazo)
	for {
		select {
		case l := <-lineas:
			if l == ": ping" {
				return
			}
		case <-limite:
			t.Fatal("no llegó el comentario de ping")
		}
	}
}

func TestStreamTerminaAlCerrarElCliente(t *testing.T) {
	termino := make(chan struct{})
	marcarFin := func(c *gin.Context) {
		c.Next()
		close(termino) // corre cuando el handler del stream volvió
	}
	srv := servidor(t, routerDePrueba(repositories.NewAvisosMock(), marcarFin))

	_, _, cerrar := abrirStream(t, srv.URL, "")
	cerrar()
	select {
	case <-termino:
	case <-time.After(plazo):
		t.Fatal("el handler del stream no terminó al irse el cliente")
	}
}
