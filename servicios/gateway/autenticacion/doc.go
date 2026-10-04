// Package autenticacion valida en el borde el token firmado de cada request (firma, vigencia,
// emisor y audiencia) y responde 401 si es inválido (teórico 6, ADR-002).
// Se implementa con la librería github.com/golang-jwt/jwt/v5, no con código propio.
// Pendiente de implementar.
package autenticacion
