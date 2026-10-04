// Package limite implementa el control de tráfico por organización (token bucket) y responde
// 429 con Retry-After al superarlo (teórico 6, ADR-002).
// Se implementa con la librería golang.org/x/time/rate (token bucket), no con código propio.
// Pendiente de implementar.
package limite
