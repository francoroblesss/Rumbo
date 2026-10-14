package puertos

import (
	"context"
	"time"

	"github.com/francoroblesss/Rumbo/servicios/comun/mensajeria"
	"github.com/francoroblesss/Rumbo/servicios/operaciones/dominio"
)

// UnidadDeTrabajo ejecuta fn en UNA transacción: o se guarda todo (cambio + outbox + idempotencia) o nada.
type UnidadDeTrabajo interface {
	Ejecutar(ctx context.Context, fn func(ctx context.Context, tx Transaccion) error) error
}

// Transaccion es lo que el núcleo puede hacer dentro de una unidad de trabajo.
type Transaccion interface {
	ObtenerParaActualizar(ctx context.Context, id string) (*dominio.Solicitud, error) // SELECT … FOR UPDATE; ErrNoEncontrada
	Obtener(ctx context.Context, id string) (*dominio.Solicitud, error)               // lectura sin bloqueo; ErrNoEncontrada
	Listar(ctx context.Context, estado string, desde, limite int) ([]*dominio.Solicitud, int, error)
	Existe(ctx context.Context, id string) (bool, error)
	Insertar(ctx context.Context, s *dominio.Solicitud) error
	Actualizar(ctx context.Context, s *dominio.Solicitud) error
	IDsContrapropuestasVencidas(ctx context.Context, ahora time.Time, limite int) ([]string, error) // FOR UPDATE SKIP LOCKED
	AgregarAlOutbox(ctx context.Context, exchange, routingKey string, sobre mensajeria.Sobre) error // guarda también el contexto de traza
	RegistrarMensaje(ctx context.Context, mensajeID string) (yaProcesado bool, err error)
	BuscarIdempotencia(ctx context.Context, org, endpoint, clave string) (*RespuestaGuardada, error)
	GuardarIdempotencia(ctx context.Context, org, endpoint, clave, hash string, r RespuestaGuardada) error
}

// RespuestaGuardada es la respuesta de un endpoint interno conservada para su clave de
// idempotencia.
type RespuestaGuardada struct {
	Hash       string // hash del cuerpo con el que se usó la clave
	EstadoHTTP int
	Cuerpo     []byte // JSON de la respuesta
}

// ConsultaClima es la operabilidad del aeropuerto según el proveedor de clima (ADR-012).
type ConsultaClima interface {
	Operabilidad(ctx context.Context, icao string) (dominio.Operabilidad, error) // ErrClimaNoDisponible si no se pudo saber
}
