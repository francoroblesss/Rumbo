package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/bradfitz/gomemcache/memcache"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/francoroblesss/Rumbo/servicios/comun/observabilidad"
	"github.com/francoroblesss/Rumbo/servicios/integracion/models"
)

// Parámetros de la caché de solicitudes (ADR-007).
const (
	TTLCache       = 60 * time.Second
	JitterMaxCache = 10 * time.Second
	TimeoutCache   = 100 * time.Millisecond
	nombreCache    = "solicitudes"
)

// ClienteCache es lo mínimo que el decorador necesita de una caché clave-valor. Get devuelve
// (nil, nil) cuando la clave no está (miss) y un error solo si la caché falló.
type ClienteCache interface {
	Get(clave string) ([]byte, error)
	Set(clave string, valor []byte, ttl time.Duration) error
	Delete(clave string) error
}

// ClaveSolicitud arma la clave de una solicitud. Incluye la organización para que nunca se sirva
// la solicitud de una organización a otra (teórico 3: la clave incluye todo lo que cambia la
// respuesta); el sufijo v1 permite cambiar el formato del valor sin leer datos viejos.
func ClaveSolicitud(organizacionID, id string) string {
	return "integracion:solicitud:" + organizacionID + ":" + id + ":v1"
}

// SolicitudesCache es el decorador cache-aside del repositorio (clase práctica 3): cumple la
// misma interfaz, atiende BuscarPorID desde Memcached y delega todo lo demás. La caché no es
// fuente de verdad: si falla, se lee de MongoDB y el consumidor nunca ve el error.
type SolicitudesCache struct {
	siguiente   SolicitudesRepository
	cache       ClienteCache
	operaciones metric.Int64Counter
}

var _ SolicitudesRepository = (*SolicitudesCache)(nil)

// NewSolicitudesCache envuelve al repositorio siguiente con la caché.
func NewSolicitudesCache(siguiente SolicitudesRepository, cache ClienteCache) *SolicitudesCache {
	contador, _ := observabilidad.Medidor().Int64Counter("rumbo_cache_operaciones_total",
		metric.WithDescription("Operaciones de caché por resultado"))
	return &SolicitudesCache{siguiente: siguiente, cache: cache, operaciones: contador}
}

func (c *SolicitudesCache) BuscarPorID(ctx context.Context, organizacionID, id string) (models.SolicitudExterna, error) {
	clave := ClaveSolicitud(organizacionID, id)
	valor, err := c.cache.Get(clave)
	switch {
	case err != nil:
		c.degradar(ctx, "leer", clave, err)
	case valor != nil:
		var s models.SolicitudExterna
		if errJSON := json.Unmarshal(valor, &s); errJSON != nil {
			c.degradar(ctx, "decodificar", clave, errJSON)
			break
		}
		c.contar(ctx, "hit")
		return s, nil
	default:
		c.contar(ctx, "miss")
	}

	s, err := c.siguiente.BuscarPorID(ctx, organizacionID, id)
	if err != nil {
		return s, err
	}
	if crudo, errJSON := json.Marshal(s); errJSON == nil {
		// Jitter en el TTL para que las claves no venzan todas juntas (cache stampede).
		ttl := TTLCache + time.Duration(rand.Int64N(int64(JitterMaxCache)+1))
		if errSet := c.cache.Set(clave, crudo, ttl); errSet != nil {
			// No guardar es aceptable: la próxima lectura vuelve a ser un miss.
			slog.WarnContext(ctx, "no se pudo guardar en la caché", slog.String("clave", clave), slog.String("error", errSet.Error()))
		}
	}
	return s, nil
}

// AplicarVista actualiza la fuente y, si cambió, borra la clave: primero la fuente y recién
// después la caché (teórico 3). Borrar es más seguro que actualizar dos lugares.
func (c *SolicitudesCache) AplicarVista(ctx context.Context, id string, version int64, vista models.VistaSolicitud, ahora time.Time) (string, bool, error) {
	organizacionID, aplicado, err := c.siguiente.AplicarVista(ctx, id, version, vista, ahora)
	if err != nil || !aplicado {
		return organizacionID, aplicado, err
	}
	clave := ClaveSolicitud(organizacionID, id)
	if errDel := c.cache.Delete(clave); errDel != nil {
		// El TTL acota cuánto puede durar la copia vieja.
		c.degradar(ctx, "invalidar", clave, errDel)
	}
	return organizacionID, aplicado, nil
}

func (c *SolicitudesCache) Crear(ctx context.Context, s models.SolicitudExterna) (models.SolicitudExterna, error) {
	return c.siguiente.Crear(ctx, s)
}

func (c *SolicitudesCache) Listar(ctx context.Context, f FiltroListado) ([]models.SolicitudExterna, int64, error) {
	return c.siguiente.Listar(ctx, f)
}

func (c *SolicitudesCache) BuscarPorClave(ctx context.Context, organizacionID, clave string) (models.SolicitudExterna, error) {
	return c.siguiente.BuscarPorClave(ctx, organizacionID, clave)
}

// degradar registra un fallo de la caché sin cortar la operación (degradación elegante).
func (c *SolicitudesCache) degradar(ctx context.Context, operacion, clave string, err error) {
	slog.WarnContext(ctx, "la caché falló, se sigue con MongoDB", slog.String("operacion", operacion),
		slog.String("clave", clave), slog.String("error", err.Error()))
	c.contar(ctx, "error")
}

func (c *SolicitudesCache) contar(ctx context.Context, resultado string) {
	c.operaciones.Add(ctx, 1, metric.WithAttributes(attribute.String("cache", nombreCache),
		attribute.String("resultado", resultado)))
}

// Memcached adapta github.com/bradfitz/gomemcache a ClienteCache.
type Memcached struct {
	cliente *memcache.Client
}

// NewMemcached crea el cliente con un timeout corto: una caché lenta no puede frenar la consulta.
func NewMemcached(direccion string) *Memcached {
	cliente := memcache.New(direccion)
	cliente.Timeout = TimeoutCache
	return &Memcached{cliente: cliente}
}

func (m *Memcached) Get(clave string) ([]byte, error) {
	item, err := m.cliente.Get(clave)
	if errors.Is(err, memcache.ErrCacheMiss) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("memcached get: %w", err)
	}
	return item.Value, nil
}

func (m *Memcached) Set(clave string, valor []byte, ttl time.Duration) error {
	if err := m.cliente.Set(&memcache.Item{Key: clave, Value: valor, Expiration: int32(ttl.Seconds())}); err != nil {
		return fmt.Errorf("memcached set: %w", err)
	}
	return nil
}

func (m *Memcached) Delete(clave string) error {
	err := m.cliente.Delete(clave)
	if err != nil && !errors.Is(err, memcache.ErrCacheMiss) {
		return fmt.Errorf("memcached delete: %w", err)
	}
	return nil
}
