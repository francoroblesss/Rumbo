// Package mensajeria es el adaptador de mensajería de Operaciones: publica los eventos del outbox
// (ADR-004). Usa el módulo comun/mensajeria (docs/interfaces-internas.md §5). Desde ADR-021 no
// consume el comando de solicitudes externas: la reserva llega por HTTP y es síncrona.
package mensajeria
