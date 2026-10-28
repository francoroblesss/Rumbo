package config

import (
	"log"
	"os"
	"strconv"
	"time"

	"github.com/francoroblesss/Rumbo/servicios/operaciones/dominio"
)

// Config reúne la configuración del servicio. Se lee de variables de entorno para que
// funcione igual en Docker Compose (host = nombre del servicio) y en local (localhost).
type Config struct {
	Puerto       string // puerto HTTP
	MySQLDSN     string // base propia del servicio
	RabbitURI    string // broker de mensajes
	RecursosURL  string // servicio Recursos
	IdentidadURL string // servicio Identidad y clientes
	ClimaURL     string // API de Clima del sistema de aeropuerto (ADR-012)
	ClimaAPIKey  string // clave de consumidor del proveedor de clima (secreto)
	ClimaICAO    string // aeropuerto consultado

	// Calendario de franjas de la capacidad publicada (ADR-021). No son secretos: tienen valores
	// por defecto y se pueden cambiar por entorno (por ejemplo, un cupo alto para los tests de carga).
	Calendario dominio.Calendario
}

// Cargar lee la configuración del entorno. Las URIs con credenciales no tienen valor por
// defecto: los secretos nunca se escriben en el código (consigna §2.12) y salen de .env.
func Cargar() Config {
	return Config{
		Puerto:       valor("PUERTO", "8082"),
		MySQLDSN:     obligatorio("MYSQL_DSN"),
		RabbitURI:    obligatorio("RABBITMQ_URI"),
		RecursosURL:  valor("RECURSOS_URL", "http://localhost:8083"),
		IdentidadURL: valor("IDENTIDAD_URL", "http://localhost:8081"),
		ClimaURL:     obligatorio("CLIMA_URL"),
		ClimaAPIKey:  obligatorio("CLIMA_API_KEY"),
		ClimaICAO:    valor("CLIMA_ICAO", "SACO"),
		Calendario: dominio.Calendario{
			AperturaHora:  entero("FRANJA_APERTURA_HORA", 8),
			CierreHora:    entero("FRANJA_CIERRE_HORA", 20),
			DuracionHoras: entero("FRANJA_DURACION_HORAS", 2),
			CupoTraslados: entero("FRANJA_CUPO_TRASLADOS", 4),
			CupoKg:        float64(entero("FRANJA_CUPO_KG", 2000)),
			Anticipacion:  time.Duration(entero("FRANJA_ANTICIPACION_HORAS", 2)) * time.Hour,
			Horizonte:     time.Duration(entero("FRANJA_HORIZONTE_DIAS", 30)) * 24 * time.Hour,
		},
	}
}

// entero lee una variable de entorno numérica positiva; si no está, usa el valor por defecto.
func entero(nombre string, porDefecto int) int {
	v := os.Getenv(nombre)
	if v == "" {
		return porDefecto
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		log.Fatalf("la variable de entorno %s tiene que ser un entero positivo", nombre)
	}
	return n
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
