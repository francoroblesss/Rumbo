// Package mensajeria es el adaptador de mensajería de Operaciones: publica los eventos del outbox
// y consume los mensajes de otros servicios (solicitudes externas, ausencias y bajas), con ACK
// después del efecto (ADR-004). Usa el módulo comun/mensajeria (docs/interfaces-internas.md §5).
package mensajeria
