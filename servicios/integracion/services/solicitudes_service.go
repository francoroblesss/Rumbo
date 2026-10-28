package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/francoroblesss/Rumbo/servicios/comun/mensajeria"
	"github.com/francoroblesss/Rumbo/servicios/comun/problema"
	"github.com/francoroblesss/Rumbo/servicios/integracion/clients"
	"github.com/francoroblesss/Rumbo/servicios/integracion/models"
	"github.com/francoroblesss/Rumbo/servicios/integracion/repositories"
)

// ReintentarNoDisponible son los segundos de Retry-After cuando Operaciones está caída o el
// breaker está abierto (ADR-008).
const ReintentarNoDisponible = 30

const (
	origenExterno        = "externo"
	codigoNoDisponible   = "SERVICIO_NO_DISPONIBLE"
	tituloNoDisponible   = "Servicio no disponible"
	codigoDatosInvalidos = "DATOS_INVALIDOS"
	tituloDatosInvalidos = "Datos inválidos"
	codigoNoEncontrada   = "NO_ENCONTRADA"
	tituloNoEncontrada   = "Traslado no encontrado"
)

// ErrorServicio es un error con su traducción al contrato público: el controlador lo escribe
// como Problem Details sin conocer las reglas.
type ErrorServicio struct {
	Estado     int
	Codigo     string
	Titulo     string
	Detalle    string
	Errores    []problema.ErrorCampo
	ReintentoS int // segundos de Retry-After (solo para 503; 0 = sin header)
}

func (e *ErrorServicio) Error() string { return e.Codigo + ": " + e.Detalle }

func nuevoError(estado int, codigo, titulo, detalle string) *ErrorServicio {
	return &ErrorServicio{Estado: estado, Codigo: codigo, Titulo: titulo, Detalle: detalle}
}

func noDisponible(detalle string, reintento int) *ErrorServicio {
	e := nuevoError(http.StatusServiceUnavailable, codigoNoDisponible, tituloNoDisponible, detalle)
	e.ReintentoS = reintento
	return e
}

// errorInterno traduce un error de infraestructura (MongoDB) a 503: el pedido es seguro de
// reintentar con la misma clave (ADR-008). El detalle técnico va al log, no al consumidor.
func errorInterno(ctx context.Context, operacion string, err error) *ErrorServicio {
	slog.ErrorContext(ctx, "falló "+operacion, slog.String("error", err.Error()))
	return noDisponible("No se pudo completar la operación. Reintentar más tarde con la misma Idempotency-Key.", 0)
}

// SolicitudesService traduce el contrato público al modelo interno. No decide nada de negocio:
// valida la forma del pedido, le pregunta a Operaciones (que decide si hay lugar) y guarda la
// copia del resultado para las consultas (ADR-001, ADR-021).
type SolicitudesService struct {
	repo    repositories.SolicitudesRepository
	cliente clients.OperacionesClient
	reloj   func() time.Time
}

// NewSolicitudesService crea el servicio. reloj permite fijar la hora en los tests.
func NewSolicitudesService(repo repositories.SolicitudesRepository, cliente clients.OperacionesClient, reloj func() time.Time) *SolicitudesService {
	return &SolicitudesService{repo: repo, cliente: cliente, reloj: reloj}
}

// ahora devuelve la hora en UTC, truncada a milisegundos (la precisión que guarda MongoDB).
func (s *SolicitudesService) ahora() time.Time {
	return s.reloj().UTC().Truncate(time.Millisecond)
}

// Crear reserva un traslado en una franja. La respuesta es definitiva en el momento: 201 con el
// traslado PROGRAMADO, o el rechazo de Operaciones (409 SIN_DISPONIBILIDAD, 422…). No hay
// estado intermedio ni decisión manual (ADR-021).
func (s *SolicitudesService) Crear(ctx context.Context, org, claveIdem string, cuerpoCrudo []byte) (models.VistaSolicitud, int, error) {
	var pedido models.PedidoTraslado
	if err := json.Unmarshal(cuerpoCrudo, &pedido); err != nil {
		return models.VistaSolicitud{}, 0, nuevoError(http.StatusBadRequest, "JSON_MAL_FORMADO", "Pedido mal formado",
			"El cuerpo no es JSON válido o un campo tiene un tipo incorrecto.")
	}
	if errores := validarPedido(pedido); len(errores) > 0 {
		e := nuevoError(http.StatusUnprocessableEntity, codigoDatosInvalidos, tituloDatosInvalidos, "Hay datos que no cumplen el contrato.")
		e.Errores = errores
		return models.VistaSolicitud{}, 0, e
	}

	// Hash del cuerpo normalizado (decodificado y vuelto a codificar): el mismo pedido con otro
	// orden de campos o espacios es el mismo pedido. Operaciones recibe ese mismo cuerpo, así que
	// calcula el mismo hash para la misma clave.
	normalizado, err := json.Marshal(pedido)
	if err != nil {
		return models.VistaSolicitud{}, 0, errorInterno(ctx, "normalizar el pedido", err)
	}
	suma := sha256.Sum256(normalizado)
	hash := hex.EncodeToString(suma[:])

	// 1. Reintento: si la clave ya tiene una reserva guardada, se responde lo mismo sin llamar a
	// Operaciones (sirve aunque Operaciones esté caída).
	existente, err := s.repo.BuscarPorClave(ctx, org, claveIdem)
	switch {
	case err == nil:
		return s.repetida(ctx, existente, hash)
	case !errors.Is(err, repositories.ErrNoEncontrada):
		return models.VistaSolicitud{}, 0, errorInterno(ctx, "buscar la clave de idempotencia", err)
	}

	// 2. Operaciones decide: bloquea la franja, verifica el lugar y crea la reserva en una sola
	// transacción (D4). Si se corta la respuesta, el consumidor reintenta con la misma clave y
	// Operaciones devuelve la misma reserva (también guarda la clave).
	vista, err := s.cliente.Reservar(ctx, org, claveIdem, pedido)
	if err != nil {
		return models.VistaSolicitud{}, 0, s.traducir(ctx, err)
	}

	// 3. Copia local para las consultas y para los reintentos.
	ahora := s.ahora()
	guardada, err := s.repo.Crear(ctx, models.SolicitudExterna{
		ID:             vista.ID,
		OrganizacionID: org,
		Pedido:         pedido,
		Idempotencia:   models.ClaveIdempotencia{Clave: claveIdem, HashCuerpo: hash, Respuesta: vista},
		Vista:          vista,
		Version:        vista.Version,
		CreadoEn:       ahora,
		ActualizadoEn:  ahora,
	})
	switch {
	case errors.Is(err, repositories.ErrClaveExistente):
		// Otra réplica guardó la misma clave a la vez (el mismo pedido repetido).
		return s.repetida(ctx, guardada, hash)
	case err != nil:
		// La reserva ya está hecha en Operaciones: el consumidor reintenta con la misma clave,
		// recibe la misma reserva y esta vez se guarda la copia.
		return models.VistaSolicitud{}, 0, errorInterno(ctx, "guardar la copia de la reserva", err)
	}
	slog.InfoContext(ctx, "traslado reservado", slog.String("solicitud_id", vista.ID))
	return vista, http.StatusCreated, nil
}

// repetida responde un reintento: la misma reserva si el cuerpo es el mismo, 409 si no.
func (s *SolicitudesService) repetida(ctx context.Context, existente models.SolicitudExterna, hash string) (models.VistaSolicitud, int, error) {
	if existente.Idempotencia.HashCuerpo != hash {
		return models.VistaSolicitud{}, 0, nuevoError(http.StatusConflict, "CLAVE_IDEMPOTENCIA_REUTILIZADA",
			"Clave de idempotencia reutilizada", "La Idempotency-Key ya se usó con un cuerpo distinto.")
	}
	slog.InfoContext(ctx, "reintento idempotente", slog.String("solicitud_id", existente.ID))
	return existente.Idempotencia.Respuesta, http.StatusCreated, nil
}

// validarPedido junta todos los errores por campo del cuerpo (DATOS_INVALIDOS). Solo la forma:
// si la franja existe y tiene lugar lo decide Operaciones.
func validarPedido(p models.PedidoTraslado) []problema.ErrorCampo {
	var errores []problema.ErrorCampo
	agregar := func(campo, mensaje string) {
		errores = append(errores, problema.ErrorCampo{Campo: campo, Mensaje: mensaje})
	}
	textoObligatorio := func(campo, valor string, maximo int) {
		if strings.TrimSpace(valor) == "" {
			agregar(campo, "es obligatorio")
		} else if utf8.RuneCountInString(valor) > maximo {
			agregar(campo, fmt.Sprintf("debe tener como máximo %d caracteres", maximo))
		}
	}
	textoOpcional := func(campo string, valor *string, maximo int) {
		if valor != nil && utf8.RuneCountInString(*valor) > maximo {
			agregar(campo, fmt.Sprintf("debe tener como máximo %d caracteres", maximo))
		}
	}

	textoObligatorio("cosa.descripcion", p.Cosa.Descripcion, 250)
	if p.Cosa.Cantidad < 1 {
		agregar("cosa.cantidad", "es obligatorio y debe ser mayor o igual a 1")
	}
	if p.Cosa.PesoKg != nil && *p.Cosa.PesoKg <= 0 {
		agregar("cosa.peso_kg", "debe ser mayor que 0")
	}
	if p.Cosa.VolumenM3 != nil && *p.Cosa.VolumenM3 <= 0 {
		agregar("cosa.volumen_m3", "debe ser mayor que 0")
	}
	for _, u := range []struct {
		nombre    string
		ubicacion models.Ubicacion
	}{{"origen", p.Origen}, {"destino", p.Destino}} {
		textoObligatorio(u.nombre+".nombre", u.ubicacion.Nombre, 120)
		textoObligatorio(u.nombre+".direccion", u.ubicacion.Direccion, 250)
		textoOpcional(u.nombre+".contacto", u.ubicacion.Contacto, 120)
	}
	if p.FranjaInicio == nil {
		agregar("franja_inicio", "es obligatorio (el inicio de una franja de GET /disponibilidad)")
	}
	textoOpcional("referencia_externa", p.ReferenciaExterna, 64)
	textoOpcional("observaciones", p.Observaciones, 500)
	return errores
}

// Disponibilidad consulta a Operaciones las franjas de la fecha. Es una lectura sin efecto: si
// Operaciones no responde, 503 y el consumidor vuelve a consultar.
func (s *SolicitudesService) Disponibilidad(ctx context.Context, fecha string, pesoKg *float64) (models.Disponibilidad, error) {
	d, err := s.cliente.Disponibilidad(ctx, fecha, pesoKg)
	if err != nil {
		return models.Disponibilidad{}, s.traducir(ctx, err)
	}
	return d, nil
}

// Obtener devuelve la copia del estado de una reserva de la organización.
func (s *SolicitudesService) Obtener(ctx context.Context, org, id string) (models.VistaSolicitud, error) {
	solicitud, err := s.buscar(ctx, org, id)
	if err != nil {
		return models.VistaSolicitud{}, err
	}
	return solicitud.Vista, nil
}

func (s *SolicitudesService) buscar(ctx context.Context, org, id string) (models.SolicitudExterna, error) {
	solicitud, err := s.repo.BuscarPorID(ctx, org, id)
	if errors.Is(err, repositories.ErrNoEncontrada) {
		return models.SolicitudExterna{}, nuevoError(http.StatusNotFound, codigoNoEncontrada, tituloNoEncontrada,
			"El traslado no existe o pertenece a otra organización.")
	}
	if err != nil {
		return models.SolicitudExterna{}, errorInterno(ctx, "buscar el traslado", err)
	}
	return solicitud, nil
}

// Listar devuelve una página de reservas de la organización, por actualizado_en ascendente.
func (s *SolicitudesService) Listar(ctx context.Context, filtro repositories.FiltroListado) (models.PaginaSolicitudes, error) {
	solicitudes, total, err := s.repo.Listar(ctx, filtro)
	if err != nil {
		return models.PaginaSolicitudes{}, errorInterno(ctx, "listar traslados", err)
	}
	pagina := models.PaginaSolicitudes{Resultados: make([]models.VistaSolicitud, 0, len(solicitudes)),
		Pagina: filtro.Pagina, Tamanio: filtro.Tamanio, Total: total}
	for _, sol := range solicitudes {
		pagina.Resultados = append(pagina.Resultados, sol.Vista)
	}
	return pagina, nil
}

// Cancelar reenvía la cancelación a Operaciones, que la aplica en el momento y devuelve el lugar
// de la franja (las reglas de 24 h son de Operaciones).
func (s *SolicitudesService) Cancelar(ctx context.Context, org, claveIdem, id, motivo string) (models.VistaSolicitud, error) {
	if _, err := s.buscar(ctx, org, id); err != nil {
		return models.VistaSolicitud{}, err
	}
	vista, err := s.cliente.Cancelar(ctx, org, claveIdem, id, motivo)
	if err != nil {
		return models.VistaSolicitud{}, s.traducir(ctx, err)
	}
	// Se aplica la vista que devolvió Operaciones (pasa por el decorador e invalida la caché).
	// Si falla, la operación igual ocurrió: se responde la verdad y el evento corregirá la copia.
	if _, _, errVista := s.repo.AplicarVista(ctx, id, vista.Version, vista, s.ahora()); errVista != nil {
		slog.WarnContext(ctx, "no se pudo actualizar la copia local", slog.String("solicitud_id", id),
			slog.String("error", errVista.Error()))
	}
	return vista, nil
}

// traducir pasa un error del cliente de Operaciones al contrato público. Los rechazos de negocio
// ya traen un código del contrato y se reenvían tal cual.
func (s *SolicitudesService) traducir(ctx context.Context, err error) *ErrorServicio {
	var rechazo clients.ErrRechazo
	switch {
	case errors.As(err, &rechazo):
		titulo := rechazo.Titulo
		if titulo == "" {
			titulo = "Pedido rechazado"
		}
		return &ErrorServicio{Estado: rechazo.Estado, Codigo: rechazo.Codigo, Titulo: titulo, Detalle: rechazo.Detalle, Errores: rechazo.Errores}
	case errors.Is(err, clients.ErrNoEncontrada):
		return nuevoError(http.StatusNotFound, codigoNoEncontrada, tituloNoEncontrada,
			"El traslado no existe o pertenece a otra organización.")
	case errors.Is(err, clients.ErrNoDisponible):
		return noDisponible("Rumbo no puede procesar el pedido en este momento. Reintentar con la misma Idempotency-Key.", ReintentarNoDisponible)
	default:
		return errorInterno(ctx, "la llamada a Operaciones", err)
	}
}

// datosEvento es la forma de datos de los eventos de rumbo.operaciones (interfaces-internas §5.4).
type datosEvento struct {
	OrganizacionID string                `json:"organizacion_id"`
	Origen         string                `json:"origen"`
	Solicitud      models.VistaSolicitud `json:"solicitud"`
}

// AplicarEvento actualiza la copia del estado con un evento de Operaciones. Es idempotente: solo
// se aplica si agregado_version es mayor que la guardada (read model, teórico 5).
func (s *SolicitudesService) AplicarEvento(ctx context.Context, sobre mensajeria.Sobre) error {
	var datos datosEvento
	if err := sobre.LeerDatos(&datos); err != nil {
		return err // ya viene marcado como permanente: va a la DLQ
	}
	if datos.Origen != origenExterno {
		return nil // las solicitudes internas no son de este servicio
	}
	_, aplicado, err := s.repo.AplicarVista(ctx, sobre.AgregadoID, sobre.AgregadoVersion, datos.Solicitud, s.ahora())
	if err != nil {
		return err // MongoDB: transitorio, se reintenta
	}
	slog.InfoContext(ctx, "evento de Operaciones procesado", slog.String("solicitud_id", sobre.AgregadoID),
		slog.String("tipo_mensaje", sobre.Tipo), slog.Bool("aplicado", aplicado))
	return nil
}
