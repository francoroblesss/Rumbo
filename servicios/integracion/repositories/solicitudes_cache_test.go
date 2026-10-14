package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/francoroblesss/Rumbo/servicios/integracion/models"
)

// cacheFalsa implementa ClienteCache en memoria y permite simular la caché caída.
type cacheFalsa struct {
	datos  map[string][]byte
	ttls   map[string]time.Duration
	caida  bool
	borros []string
}

func nuevaCacheFalsa() *cacheFalsa {
	return &cacheFalsa{datos: map[string][]byte{}, ttls: map[string]time.Duration{}}
}

var errCaida = errors.New("memcached no responde")

func (c *cacheFalsa) Get(clave string) ([]byte, error) {
	if c.caida {
		return nil, errCaida
	}
	return c.datos[clave], nil
}

func (c *cacheFalsa) Set(clave string, valor []byte, ttl time.Duration) error {
	if c.caida {
		return errCaida
	}
	c.datos[clave], c.ttls[clave] = valor, ttl
	return nil
}

func (c *cacheFalsa) Delete(clave string) error {
	if c.caida {
		return errCaida
	}
	c.borros = append(c.borros, clave)
	delete(c.datos, clave)
	return nil
}

func solicitudDePrueba() models.SolicitudExterna {
	return models.SolicitudExterna{ID: "sol-1", OrganizacionID: "org-1",
		Vista: models.VistaSolicitud{ID: "sol-1", Estado: models.EstadoRecibida}}
}

func TestCacheMissGuardaYHitNoConsultaMongo(t *testing.T) {
	mock, cache := NewSolicitudesMock(), nuevaCacheFalsa()
	_, _ = mock.Crear(context.Background(), solicitudDePrueba())
	repo := NewSolicitudesCache(mock, cache)

	if _, err := repo.BuscarPorID(context.Background(), "org-1", "sol-1"); err != nil {
		t.Fatal(err)
	}
	clave := "integracion:solicitud:org-1:sol-1:v1"
	if cache.datos[clave] == nil {
		t.Fatal("un miss tiene que guardar la solicitud en la caché")
	}
	if ttl := cache.ttls[clave]; ttl < TTLCache || ttl > TTLCache+JitterMaxCache {
		t.Fatalf("TTL fuera de 60 s + jitter de 0 a 10 s: %v", ttl)
	}
	s, err := repo.BuscarPorID(context.Background(), "org-1", "sol-1")
	if err != nil || s.Vista.Estado != models.EstadoRecibida {
		t.Fatalf("hit incorrecto: %+v %v", s, err)
	}
	if mock.LecturasPorID != 1 {
		t.Fatalf("un hit no tiene que consultar el repositorio siguiente (lecturas: %d)", mock.LecturasPorID)
	}
}

func TestCacheLaClaveIncluyeLaOrganizacion(t *testing.T) {
	mock, cache := NewSolicitudesMock(), nuevaCacheFalsa()
	_, _ = mock.Crear(context.Background(), solicitudDePrueba())
	repo := NewSolicitudesCache(mock, cache)
	_, _ = repo.BuscarPorID(context.Background(), "org-1", "sol-1")
	if _, err := repo.BuscarPorID(context.Background(), "org-otra", "sol-1"); !errors.Is(err, ErrNoEncontrada) {
		t.Fatalf("otra organización no puede leer la copia en caché: %v", err)
	}
}

func TestCacheCaidaDegradaAMongo(t *testing.T) {
	mock, cache := NewSolicitudesMock(), nuevaCacheFalsa()
	_, _ = mock.Crear(context.Background(), solicitudDePrueba())
	cache.caida = true
	repo := NewSolicitudesCache(mock, cache)
	s, err := repo.BuscarPorID(context.Background(), "org-1", "sol-1")
	if err != nil || s.ID != "sol-1" {
		t.Fatalf("con la caché caída se tiene que responder desde MongoDB, sin error: %v", err)
	}
	if mock.LecturasPorID != 1 {
		t.Fatal("se tenía que leer de MongoDB")
	}
}

func TestCacheAplicarVistaBorraLaClave(t *testing.T) {
	mock, cache := NewSolicitudesMock(), nuevaCacheFalsa()
	_, _ = mock.Crear(context.Background(), solicitudDePrueba())
	repo := NewSolicitudesCache(mock, cache)
	_, _ = repo.BuscarPorID(context.Background(), "org-1", "sol-1")

	_, aplicado, err := repo.AplicarVista(context.Background(), "sol-1", 2, models.VistaSolicitud{ID: "sol-1", Estado: models.EstadoAceptada}, time.Now())
	if err != nil || !aplicado {
		t.Fatalf("no se aplicó: %v", err)
	}
	if len(cache.borros) != 1 || cache.borros[0] != "integracion:solicitud:org-1:sol-1:v1" {
		t.Fatalf("aplicar la vista tiene que borrar la clave: %v", cache.borros)
	}
	s, _ := repo.BuscarPorID(context.Background(), "org-1", "sol-1")
	if s.Vista.Estado != models.EstadoAceptada {
		t.Fatal("después de invalidar se tiene que leer el estado nuevo")
	}

	// Un evento viejo no cambia nada y no borra.
	_, aplicado, _ = repo.AplicarVista(context.Background(), "sol-1", 1, models.VistaSolicitud{}, time.Now())
	if aplicado || len(cache.borros) != 1 {
		t.Fatal("un evento viejo no tiene que invalidar")
	}
}
