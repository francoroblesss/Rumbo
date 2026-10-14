package puertos

import "errors"

// Errores de los puertos (no son reglas de negocio sino resultados de acceder a algo).
var (
	// ErrNoEncontrada: la solicitud no existe o es de otra organización (no se revela cuál).
	ErrNoEncontrada = errors.New("la solicitud no existe")
	// ErrClaveReutilizada: la Idempotency-Key ya se usó con otro cuerpo.
	ErrClaveReutilizada = errors.New("la clave de idempotencia ya se usó con otro cuerpo")
	// ErrClimaNoDisponible: no se pudo saber la operabilidad del aeropuerto.
	ErrClimaNoDisponible = errors.New("el servicio de clima no está disponible")
)
