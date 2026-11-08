package config

import (
	"log"
	"os"
)

// Config reúne la configuración del servicio. Se lee de variables de entorno para que
// funcione igual en Docker Compose (host = nombre del servicio) y en local (localhost).
type Config struct {
	Puerto       string // puerto HTTP
	MongoURI     string // base propia del servicio
	TokenSecreto string // clave para firmar los tokens (la misma que valida el gateway)

	AdminEmail          string // usuario administrador inicial
	AdminPassword       string // su contraseña (se guarda hasheada)
	ExternoClientID     string // client_id del sistema externo (grupo consumidor)
	ExternoClientSecret string // su secreto (se guarda hasheado)

	// Usuarios de demostración de la web (docs/interfaces-internas.md §15). Son opcionales: con el
	// email vacío ese usuario no se siembra (en la nube no hacen falta, la web corre en local).
	ExternoOrganizacionNombre string // nombre de la organización del sistema externo
	OperadorEmail             string
	OperadorPassword          string
	ClienteEmail              string
	ClientePassword           string
}

// Cargar lee la configuración del entorno. Las URIs, los secretos y las credenciales iniciales
// no tienen valor por defecto: los secretos nunca se escriben en el código (consigna §2.12) y
// salen de .env.
func Cargar() Config {
	return Config{
		Puerto:              valor("PUERTO", "8081"),
		MongoURI:            obligatorio("MONGO_URI"),
		TokenSecreto:        obligatorio("TOKEN_SECRETO"),
		AdminEmail:          obligatorio("ADMIN_EMAIL"),
		AdminPassword:       obligatorio("ADMIN_PASSWORD"),
		ExternoClientID:     obligatorio("EXTERNO_CLIENT_ID"),
		ExternoClientSecret: obligatorio("EXTERNO_CLIENT_SECRET"),

		ExternoOrganizacionNombre: valor("EXTERNO_ORGANIZACION_NOMBRE", "Gestión de almacenamiento"),
		OperadorEmail:             os.Getenv("OPERADOR_EMAIL"),
		OperadorPassword:          claveSiHayEmail("OPERADOR_EMAIL", "OPERADOR_PASSWORD"),
		ClienteEmail:              os.Getenv("CLIENTE_EMAIL"),
		ClientePassword:           claveSiHayEmail("CLIENTE_EMAIL", "CLIENTE_PASSWORD"),
	}
}

// claveSiHayEmail devuelve la contraseña de un usuario de demostración. Solo es obligatoria si
// su email está definido: un usuario sembrado con la contraseña vacía no podría entrar nunca.
func claveSiHayEmail(variableEmail, variableClave string) string {
	if os.Getenv(variableEmail) == "" {
		return os.Getenv(variableClave)
	}
	return obligatorio(variableClave)
}

// valor devuelve la variable de entorno o, si no está definida, el valor por defecto.
func valor(nombre, porDefecto string) string {
	if v := os.Getenv(nombre); v != "" {
		return v
	}
	return porDefecto
}

// obligatorio devuelve la variable de entorno o termina el servicio si no está definida.
func obligatorio(nombre string) string {
	v := os.Getenv(nombre)
	if v == "" {
		log.Fatalf("falta la variable de entorno %s (ver .env.example)", nombre)
	}
	return v
}
