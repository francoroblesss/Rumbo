// Package clients contiene el cliente HTTP hacia Operaciones (responder contrapropuestas y
// cancelar), con timeout de 1 s por intento, circuit breaker y hasta 3 intentos con backoff
// exponencial y jitter ante fallas transitorias (ADR-035).
package clients
