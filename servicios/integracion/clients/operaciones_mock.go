package clients

import (
	"context"
	"sync"

	"github.com/francoroblesss/Rumbo/servicios/integracion/models"
)

// LlamadaMock registra una llamada recibida por el mock.
type LlamadaMock struct {
	Operacion, Org, ClaveIdem, ID, Motivo string
	Acepta                                bool
}

// OperacionesMock implementa OperacionesClient para los tests de servicios: devuelve Vista o
// Error según se configure y registra las llamadas.
type OperacionesMock struct {
	mu       sync.Mutex
	Vista    models.VistaSolicitud
	Error    error
	Llamadas []LlamadaMock
}

var _ OperacionesClient = (*OperacionesMock)(nil)

func (m *OperacionesMock) ResponderContrapropuesta(_ context.Context, org, claveIdem, id string, acepta bool) (models.VistaSolicitud, error) {
	return m.registrar(LlamadaMock{Operacion: "respuesta-contrapropuesta", Org: org, ClaveIdem: claveIdem, ID: id, Acepta: acepta})
}

func (m *OperacionesMock) Cancelar(_ context.Context, org, claveIdem, id, motivo string) (models.VistaSolicitud, error) {
	return m.registrar(LlamadaMock{Operacion: "cancelacion", Org: org, ClaveIdem: claveIdem, ID: id, Motivo: motivo})
}

func (m *OperacionesMock) registrar(l LlamadaMock) (models.VistaSolicitud, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Llamadas = append(m.Llamadas, l)
	if m.Error != nil {
		return models.VistaSolicitud{}, m.Error
	}
	return m.Vista, nil
}
