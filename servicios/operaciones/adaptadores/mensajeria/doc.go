// Package mensajeria es el adaptador de mensajería de Operaciones (ADR-004), sobre el módulo
// comun/mensajeria (docs/interfaces-internas.md §5):
//   - de salida: publica los eventos del outbox;
//   - de entrada: consume su propio evento operaciones.solicitud.aceptada para revisar el clima
//     después de la reserva (§13.2, ADR-025).
//
// Desde ADR-021 no consume el comando de solicitudes externas: la reserva llega por HTTP y es
// síncrona.
package mensajeria
