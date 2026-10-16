package repositories

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/francoroblesss/Rumbo/servicios/integracion/models"
)

// SolicitudesMock implementa SolicitudesRepository en memoria (como el mock de la clase
// práctica 2). Lo usan los tests de servicios y controladores.
type SolicitudesMock struct {
	mu          sync.Mutex
	solicitudes map[string]models.SolicitudExterna
	// Error, si no es nil, lo devuelven todas las operaciones (simula MongoDB caído).
	Error error
	// LecturasPorID cuenta las llamadas a BuscarPorID (para verificar la caché).
	LecturasPorID int
}

var _ SolicitudesRepository = (*SolicitudesMock)(nil)

// NewSolicitudesMock crea un repositorio vacío.
func NewSolicitudesMock() *SolicitudesMock {
	return &SolicitudesMock{solicitudes: map[string]models.SolicitudExterna{}}
}

func (m *SolicitudesMock) Crear(_ context.Context, s models.SolicitudExterna) (models.SolicitudExterna, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Error != nil {
		return models.SolicitudExterna{}, m.Error
	}
	for _, existente := range m.solicitudes {
		if existente.OrganizacionID == s.OrganizacionID && existente.Idempotencia.Clave == s.Idempotencia.Clave {
			return existente, ErrClaveExistente
		}
	}
	m.solicitudes[s.ID] = s
	return s, nil
}

func (m *SolicitudesMock) BuscarPorID(_ context.Context, organizacionID, id string) (models.SolicitudExterna, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.LecturasPorID++
	if m.Error != nil {
		return models.SolicitudExterna{}, m.Error
	}
	s, ok := m.solicitudes[id]
	if !ok || s.OrganizacionID != organizacionID {
		return models.SolicitudExterna{}, ErrNoEncontrada
	}
	return s, nil
}

func (m *SolicitudesMock) Listar(_ context.Context, f FiltroListado) ([]models.SolicitudExterna, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Error != nil {
		return nil, 0, m.Error
	}
	var filtradas []models.SolicitudExterna
	for _, s := range m.solicitudes {
		if s.OrganizacionID != f.OrganizacionID ||
			(f.ActualizadoDesde != nil && !s.ActualizadoEn.After(*f.ActualizadoDesde)) ||
			(f.Estado != "" && s.Vista.Estado != f.Estado) {
			continue
		}
		filtradas = append(filtradas, s)
	}
	sort.Slice(filtradas, func(i, j int) bool {
		if !filtradas[i].ActualizadoEn.Equal(filtradas[j].ActualizadoEn) {
			return filtradas[i].ActualizadoEn.Before(filtradas[j].ActualizadoEn)
		}
		return filtradas[i].ID < filtradas[j].ID
	})
	total := int64(len(filtradas))
	desde := min((f.Pagina-1)*f.Tamanio, len(filtradas))
	hasta := min(desde+f.Tamanio, len(filtradas))
	return append([]models.SolicitudExterna{}, filtradas[desde:hasta]...), total, nil
}

func (m *SolicitudesMock) ReclamarPendiente(_ context.Context, instancia string, ahora time.Time) (models.SolicitudExterna, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Error != nil {
		return models.SolicitudExterna{}, m.Error
	}
	for id, s := range m.solicitudes {
		if s.PendienteEnvio && (s.ReclamadoHasta == nil || s.ReclamadoHasta.Before(ahora)) {
			hasta := ahora.Add(DuracionReclamo)
			s.ReclamadoHasta, s.ReclamadoPor = &hasta, instancia
			m.solicitudes[id] = s
			return s, nil
		}
	}
	return models.SolicitudExterna{}, ErrNoHayPendientes
}

func (m *SolicitudesMock) MarcarEnviada(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Error != nil {
		return m.Error
	}
	if s, ok := m.solicitudes[id]; ok {
		s.PendienteEnvio, s.ReclamadoHasta, s.ReclamadoPor = false, nil, ""
		m.solicitudes[id] = s
	}
	return nil
}

func (m *SolicitudesMock) AplicarVista(_ context.Context, id string, version int64, vista models.VistaSolicitud, ahora time.Time) (string, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Error != nil {
		return "", false, m.Error
	}
	s, ok := m.solicitudes[id]
	if !ok || s.Version >= version {
		return "", false, nil
	}
	s.Vista, s.Version, s.ActualizadoEn = vista, version, ahora
	m.solicitudes[id] = s
	return s.OrganizacionID, true, nil
}

func (m *SolicitudesMock) ContarPendientes(_ context.Context) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Error != nil {
		return 0, m.Error
	}
	var n int64
	for _, s := range m.solicitudes {
		if s.PendienteEnvio {
			n++
		}
	}
	return n, nil
}
