package aplicacion

import (
	"context"
	"encoding/json"
	"sort"
	"sync"
	"time"

	"github.com/francoroblesss/Rumbo/servicios/comun/mensajeria"
	"github.com/francoroblesss/Rumbo/servicios/operaciones/dominio"
	"github.com/francoroblesss/Rumbo/servicios/operaciones/puertos"
)

// uowMemoria es una UnidadDeTrabajo en memoria para probar los casos de uso sin MySQL. Imita la
// atomicidad: trabaja sobre una copia del estado y solo la confirma si fn no devuelve error.
type uowMemoria struct {
	mu     sync.Mutex
	estado estadoMemoria
}

type mensajeOutbox struct {
	Exchange, RoutingKey string
	Sobre                mensajeria.Sobre
}

type estadoMemoria struct {
	Solicitudes map[string]*dominio.Solicitud
	Outbox      []mensajeOutbox
	Claves      map[string]puertos.RespuestaGuardada
	Franjas     map[string]dominio.Franja // por inicio en RFC 3339 (UTC)
}

func nuevaUoW() *uowMemoria {
	return &uowMemoria{estado: estadoMemoria{
		Solicitudes: map[string]*dominio.Solicitud{},
		Claves:      map[string]puertos.RespuestaGuardada{},
		Franjas:     map[string]dominio.Franja{},
	}}
}

// copiar hace una copia profunda (el dominio no tiene tags, pero se serializa igual).
func copiar[T any](v T) T {
	crudo, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	var c T
	if err := json.Unmarshal(crudo, &c); err != nil {
		panic(err)
	}
	return c
}

func (u *uowMemoria) Ejecutar(ctx context.Context, fn func(ctx context.Context, tx puertos.Transaccion) error) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	tx := &txMemoria{e: copiar(u.estado)}
	if err := fn(ctx, tx); err != nil {
		return err
	}
	u.estado = tx.e
	return nil
}

func (u *uowMemoria) outbox() []mensajeOutbox {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]mensajeOutbox(nil), u.estado.Outbox...)
}

type txMemoria struct{ e estadoMemoria }

func (t *txMemoria) ObtenerParaActualizar(ctx context.Context, id string) (*dominio.Solicitud, error) {
	return t.Obtener(ctx, id)
}

func (t *txMemoria) Obtener(_ context.Context, id string) (*dominio.Solicitud, error) {
	s, ok := t.e.Solicitudes[id]
	if !ok {
		return nil, puertos.ErrNoEncontrada
	}
	return copiar(s), nil
}

func (t *txMemoria) Listar(_ context.Context, f puertos.FiltroSolicitudes, desde, limite int) ([]*dominio.Solicitud, int, error) {
	var todas []*dominio.Solicitud
	for _, s := range t.e.Solicitudes {
		tr := s.Traslado
		switch {
		case f.Organizacion != "" && s.OrganizacionID != f.Organizacion,
			f.EstadoTraslado != "" && (tr == nil || string(tr.Estado) != f.EstadoTraslado),
			f.FranjaDesde != nil && (tr == nil || tr.FranjaInicio == nil || tr.FranjaInicio.Before(*f.FranjaDesde) || !tr.FranjaInicio.Before(*f.FranjaHasta)):
			continue
		}
		todas = append(todas, copiar(s))
	}
	// Mismo orden que MySQL: inicio de la franja y después creación.
	sort.Slice(todas, func(i, j int) bool {
		a, b := todas[i].Traslado.FranjaInicio, todas[j].Traslado.FranjaInicio
		if !a.Equal(*b) {
			return a.Before(*b)
		}
		return todas[i].CreadoEn.Before(todas[j].CreadoEn)
	})
	total := len(todas)
	if desde > total {
		desde = total
	}
	return todas[desde:min(desde+limite, total)], total, nil
}

func (t *txMemoria) Insertar(_ context.Context, s *dominio.Solicitud) error {
	t.e.Solicitudes[s.ID] = copiar(s)
	return nil
}

func (t *txMemoria) Actualizar(_ context.Context, s *dominio.Solicitud) error {
	t.e.Solicitudes[s.ID] = copiar(s)
	return nil
}

func (t *txMemoria) AgregarAlOutbox(_ context.Context, exchange, routingKey string, sobre mensajeria.Sobre) error {
	t.e.Outbox = append(t.e.Outbox, mensajeOutbox{Exchange: exchange, RoutingKey: routingKey, Sobre: sobre})
	return nil
}

// FranjaParaActualizar no necesita bloquear: uowMemoria ya ejecuta una transacción por vez. La
// concurrencia real se prueba contra MySQL (adaptadores/mysql).
func (t *txMemoria) FranjaParaActualizar(_ context.Context, plantilla dominio.Franja) (*dominio.Franja, error) {
	clave := plantilla.Inicio.UTC().Format(time.RFC3339)
	f, ok := t.e.Franjas[clave]
	if !ok {
		f = plantilla
		t.e.Franjas[clave] = f
	}
	return &f, nil
}

func (t *txMemoria) GuardarFranja(_ context.Context, f *dominio.Franja) error {
	t.e.Franjas[f.Inicio.UTC().Format(time.RFC3339)] = *f
	return nil
}

func (t *txMemoria) Franjas(_ context.Context, desde, hasta time.Time) ([]dominio.Franja, error) {
	var franjas []dominio.Franja
	for _, f := range t.e.Franjas {
		if !f.Inicio.Before(desde) && !f.Inicio.After(hasta) {
			franjas = append(franjas, f)
		}
	}
	return franjas, nil
}

func (t *txMemoria) BuscarIdempotencia(_ context.Context, org, endpoint, clave string) (*puertos.RespuestaGuardada, error) {
	r, ok := t.e.Claves[org+"|"+endpoint+"|"+clave]
	if !ok {
		return nil, nil
	}
	return &r, nil
}

func (t *txMemoria) GuardarIdempotencia(_ context.Context, org, endpoint, clave, _ string, r puertos.RespuestaGuardada) error {
	t.e.Claves[org+"|"+endpoint+"|"+clave] = r
	return nil
}

// climaFalso responde lo configurado y cuenta las consultas. Si mientras se consulta tiene que
// pasar algo en paralelo (otra transacción), lo hace durante.
type climaFalso struct {
	operabilidad dominio.Operabilidad
	err          error
	consultas    int
	durante      func()
}

func (c *climaFalso) Operabilidad(context.Context, string) (dominio.Operabilidad, error) {
	c.consultas++
	if c.durante != nil {
		c.durante()
	}
	return c.operabilidad, c.err
}
