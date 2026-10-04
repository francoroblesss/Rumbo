package config

import "os"

// Config reúne la configuración del servicio. Se lee de variables de entorno para que
// funcione igual en Docker Compose (host = nombre del servicio) y en local (localhost).
type Config struct {
	Puerto            string // puerto HTTP público
	TokenSecreto      string // clave para validar los tokens firmados
	IdentidadURL      string // servicio Identidad y clientes
	OperacionesURL    string // servicio Operaciones
	RecursosURL       string // servicio Recursos
	NotificacionesURL string // servicio Notificaciones
	IntegracionURL    string // servicio Integración de carga
}

// Cargar lee la configuración del entorno, con valores por defecto para desarrollo local.
func Cargar() Config {
	return Config{
		Puerto:            valor("PUERTO", "8080"),
		TokenSecreto:      valor("TOKEN_SECRETO", ""),
		IdentidadURL:      valor("IDENTIDAD_URL", "http://localhost:8081"),
		OperacionesURL:    valor("OPERACIONES_URL", "http://localhost:8082"),
		RecursosURL:       valor("RECURSOS_URL", "http://localhost:8083"),
		NotificacionesURL: valor("NOTIFICACIONES_URL", "http://localhost:8084"),
		IntegracionURL:    valor("INTEGRACION_URL", "http://localhost:8085"),
	}
}

// valor devuelve la variable de entorno o, si no está definida, el valor por defecto.
func valor(nombre, porDefecto string) string {
	if v := os.Getenv(nombre); v != "" {
		return v
	}
	return porDefecto
}
