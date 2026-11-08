package config

import (
	"log"
	"os"
	"strconv"
	"strings"
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
	ClimaPrefer  string // solo desarrollo: header Prefer para elegir el ejemplo del mock (ADR-025)
	// SolrURL es el core de la búsqueda (ADR-011). Opcional: vacío en la nube (ADR-026), donde la
	// búsqueda responde 503 y nada más cambia.
	SolrURL string

	// AeropuertoPalabras marcan qué extremo del traslado es el aeropuerto (R-46, ADR-025): ya en
	// minúsculas y sin tildes.
	AeropuertoPalabras []string

	// Calendario de franjas de la capacidad publicada (ADR-021). No son secretos: tienen valores
	// por defecto y se pueden cambiar por entorno (por ejemplo, un cupo alto para los tests de carga).
	Calendario dominio.Calendario
}

// Cargar lee la configuración del entorno. Las URIs con credenciales no tienen valor por
// defecto: los secretos nunca se escriben en el código (consigna §2.12) y salen de .env.
func Cargar() Config {
	return Config{
		Puerto:             valor("PUERTO", "8082"),
		MySQLDSN:           obligatorio("MYSQL_DSN"),
		RabbitURI:          obligatorio("RABBITMQ_URI"),
		RecursosURL:        valor("RECURSOS_URL", "http://localhost:8083"),
		IdentidadURL:       valor("IDENTIDAD_URL", "http://localhost:8081"),
		ClimaURL:           obligatorio("CLIMA_URL"),
		ClimaAPIKey:        obligatorio("CLIMA_API_KEY"),
		ClimaICAO:          valor("CLIMA_ICAO", "SACO"),
		ClimaPrefer:        os.Getenv("CLIMA_PREFER"),
		SolrURL:            os.Getenv("SOLR_URL"),
		AeropuertoPalabras: lista("AEROPUERTO_PALABRAS", "aeropuerto,saco,pajas blancas"),
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

// lista lee una variable separada por comas y normaliza cada elemento como lo compara el dominio
// (sin espacios sobrantes, en minúsculas y sin tildes). Los elementos vacíos se descartan.
func lista(nombre, porDefecto string) []string {
	var r []string
	for _, p := range strings.Split(valor(nombre, porDefecto), ",") {
		if p = dominio.Normalizar(p); p != "" {
			r = append(r, p)
		}
	}
	return r
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
