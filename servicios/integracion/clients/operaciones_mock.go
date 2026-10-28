package clients

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/francoroblesss/Rumbo/servicios/integracion/models"
)

// LlamadaMock registra una llamada recibida por el mock.
type LlamadaMock struct {
	Operacion, Org, ClaveIdem, ID, Motivo, Fecha string
	Pedido                                       models.PedidoTraslado
}

// OperacionesMock implementa OperacionesClient para los tests de servicios: devuelve Vista,
// Disponibilidad o Error según se configure y registra las llamadas.
type OperacionesMock struct {
	mu         sync.Mutex
	Vista      models.VistaSolicitud
	Disponible models.Disponibilidad
	Error      error
	Llamadas   []LlamadaMock
}

var _ OperacionesClient = (*OperacionesMock)(nil)

func (m *OperacionesMock) Disponibilidad(_ context.Context, fecha string, _ *float64) (models.Disponibilidad, error) {
	_, err := m.registrar(LlamadaMock{Operacion: "disponibilidad", Fecha: fecha})
	return m.Disponible, err
}

// Reservar devuelve Vista si está configurada; si no, arma una reserva nueva (sol-1, sol-2…)
// PROGRAMADA en la franja pedida, como haría Operaciones.
func (m *OperacionesMock) Reservar(_ context.Context, org, claveIdem string, pedido models.PedidoTraslado) (models.VistaSolicitud, error) {
	v, err := m.registrar(LlamadaMock{Operacion: "reserva", Org: org, ClaveIdem: claveIdem, Pedido: pedido})
	if err != nil || v.ID != "" {
		return v, err
	}
	m.mu.Lock()
	n := len(m.Llamadas)
	m.mu.Unlock()
	inicio := time.Time{}
	if pedido.FranjaInicio != nil {
		inicio = *pedido.FranjaInicio
	}
	fin := inicio.Add(2 * time.Hour)
	return models.VistaSolicitud{
		ID: fmt.Sprintf("sol-%d", n), ReferenciaExterna: pedido.ReferenciaExterna, Estado: "ACEPTADA",
		Cosa: pedido.Cosa, Origen: pedido.Origen, Destino: pedido.Destino, Observaciones: pedido.Observaciones,
		Traslado: &models.Traslado{ID: fmt.Sprintf("tra-%d", n), Estado: models.EstadoProgramado,
			HoraLlegadaComprometida: fin, Franja: &models.Franja{Inicio: inicio, Fin: fin}},
		Version: 1, CreadoEn: inicio.Add(-24 * time.Hour), ActualizadoEn: inicio.Add(-24 * time.Hour),
	}, nil
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
