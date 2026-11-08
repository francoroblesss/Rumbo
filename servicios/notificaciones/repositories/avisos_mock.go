package repositories

import (
	"context"
	"slices"
	"sort"
	"sync"

	"github.com/francoroblesss/Rumbo/servicios/notificaciones/models"
)

// AvisosMock implementa AvisosRepository en memoria (como el mock de la clase práctica 2). Lo
// usan los tests de servicios y controladores.
type AvisosMock struct {
	mu     sync.Mutex
	avisos map[string]models.Aviso
	// Error, si no es nil, lo devuelven todas las operaciones (simula MongoDB caído).
	Error error
}

var _ AvisosRepository = (*AvisosMock)(nil)

// NewAvisosMock crea un repositorio vacío.
func NewAvisosMock() *AvisosMock {
	return &AvisosMock{avisos: map[string]models.Aviso{}}
}

func (m *AvisosMock) Insertar(_ context.Context, a models.Aviso) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Error != nil {
		return m.Error
	}
	if _, ok := m.avisos[a.ID]; ok {
		return ErrAvisoExistente
	}
	m.avisos[a.ID] = a
	return nil
}

func (m *AvisosMock) Listar(_ context.Context, visibles []models.Destinatario, soloNoLeidos bool, pagina, tamanio int) ([]models.Aviso, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Error != nil {
		return nil, 0, m.Error
	}
	var filtrados []models.Aviso
	for _, a := range m.avisos {
		if slices.Contains(visibles, a.Destinatario) && (!soloNoLeidos || !a.Leido) {
			filtrados = append(filtrados, a)
		}
	}
	sort.Slice(filtrados, func(i, j int) bool { return posterior(filtrados[i], filtrados[j]) })
	total := int64(len(filtrados))
	desde := min((pagina-1)*tamanio, len(filtrados))
	hasta := min(desde+tamanio, len(filtrados))
	return append([]models.Aviso{}, filtrados[desde:hasta]...), total, nil
}

func (m *AvisosMock) Buscar(_ context.Context, visibles []models.Destinatario, id string) (models.Aviso, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Error != nil {
		return models.Aviso{}, m.Error
	}
	a, ok := m.avisos[id]
	if !ok || !slices.Contains(visibles, a.Destinatario) {
		return models.Aviso{}, ErrNoEncontrado
	}
	return a, nil
}

func (m *AvisosMock) MarcarLeido(_ context.Context, visibles []models.Destinatario, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Error != nil {
		return m.Error
	}
	a, ok := m.avisos[id]
	if !ok || !slices.Contains(visibles, a.Destinatario) {
		return ErrNoEncontrado
	}
	a.Leido = true
	m.avisos[id] = a
	return nil
}

func (m *AvisosMock) Posteriores(_ context.Context, visibles []models.Destinatario, desde Cursor, limite int) ([]models.Aviso, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Error != nil {
		return nil, m.Error
	}
	cursor := models.Aviso{ID: desde.ID, CreadoEn: desde.CreadoEn}
	var filtrados []models.Aviso
	for _, a := range m.avisos {
		if slices.Contains(visibles, a.Destinatario) && posterior(a, cursor) {
			filtrados = append(filtrados, a)
		}
	}
	sort.Slice(filtrados, func(i, j int) bool { return posterior(filtrados[j], filtrados[i]) })
	return filtrados[:min(limite, len(filtrados))], nil
}

// posterior indica si a va después de b en el orden (creado_en, _id).
func posterior(a, b models.Aviso) bool {
	if !a.CreadoEn.Equal(b.CreadoEn) {
		return a.CreadoEn.After(b.CreadoEn)
	}
	return a.ID > b.ID
}
