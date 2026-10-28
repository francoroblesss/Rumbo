// Package messaging contiene la mensajería propia de integracion: cómo maneja cada mensaje que
// consume (los cambios de estado de Operaciones). Desde ADR-021 no publica nada: la reserva va a
// Operaciones por HTTP. La conexión, el sobre, los reintentos y la DLQ están en el módulo
// comun/mensajeria (docs/interfaces-internas.md §5).
package messaging
