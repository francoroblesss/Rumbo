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

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"

	"github.com/francoroblesss/Rumbo/servicios/comun/mensajeria"
	"github.com/francoroblesss/Rumbo/servicios/comun/observabilidad"
	"github.com/francoroblesss/Rumbo/servicios/comun/problema"
	"github.com/francoroblesss/Rumbo/servicios/integracion/clients"
	"github.com/francoroblesss/Rumbo/servicios/integracion/models"
	"github.com/francoroblesss/Rumbo/servicios/integracion/repositories"
)

// ClaveCorrelacionTraza es la clave con la que se guarda el correlation id junto al contexto de
// la traza, para que el relay publique el comando con el mismo correlation id (interfaces-internas §5.1).
const ClaveCorrelacionTraza = mensajeria.HeaderCorrelacion

// Segundos de Retry-After para cada 503 (interfaces-internas §4.1 y ADR-008).
const (
	ReintentarSinRecibir   = 5  // Operaciones todavía no recibió la solicitud
	ReintentarNoDisponible = 30 // Operaciones caída o breaker abierto
)

const (
	origenExterno        = "externo"
	codigoNoDisponible   = "SERVICIO_NO_DISPONIBLE"
	tituloNoDisponible   = "Servicio no disponible"
	codigoDatosInvalidos = "DATOS_INVALIDOS"
	tituloDatosInvalidos = "Datos inválidos"
	codigoNoEncontrada   = "NO_ENCONTRADA"
	tituloNoEncontrada   = "Solicitud no encontrada"
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
// recibe, guarda, avisa a Operaciones y responde la copia del estado (ADR-001).
type SolicitudesService struct {
	repo    repositories.SolicitudesRepository
	cliente clients.OperacionesClient
	reloj   func() time.Time
}

// NewSolicitudesService crea el servicio. reloj permite fijar la hora en los tests.
func NewSolicitudesService(repo repositories.SolicitudesRepository, cliente clients.OperacionesClient, reloj func() time.Time) *SolicitudesService {
	return &SolicitudesService{repo: repo, cliente: cliente, reloj: reloj}
}

// ahora devuelve la hora en UTC, truncada a milisegundos (la precisión que guarda MongoDB), así la
// primera respuesta y las siguientes muestran exactamente las mismas horas.
func (s *SolicitudesService) ahora() time.Time {
	return s.reloj().UTC().Truncate(time.Millisecond)
}

// Crear registra una solicitud nueva con estado RECIBIDA y la deja pendiente de envío a
// Operaciones. Devuelve la vista y el código HTTP (202: recibir no es confirmar, teórico 4).
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
	ahora := s.ahora()
	if pedido.HoraLlegadaSolicitada != nil && pedido.HoraLlegadaSolicitada.Before(ahora) {
		return models.VistaSolicitud{}, 0, nuevoError(http.StatusUnprocessableEntity, "HORARIO_EN_EL_PASADO",
			"Horario en el pasado", "La hora_llegada_solicitada es anterior al momento actual.")
	}

	// Hash del cuerpo normalizado (decodificado y vuelto a codificar): el mismo pedido con otro
	// orden de campos o espacios es el mismo pedido.
	normalizado, err := json.Marshal(pedido)
	if err != nil {
		return models.VistaSolicitud{}, 0, errorInterno(ctx, "normalizar el pedido", err)
	}
	suma := sha256.Sum256(normalizado)
	hash := hex.EncodeToString(suma[:])

	if pedido.HoraLlegadaSolicitada != nil {
		utc := pedido.HoraLlegadaSolicitada.UTC()
		pedido.HoraLlegadaSolicitada = &utc
	}
	tolerancia := 0
	if pedido.ToleranciaMinutos != nil {
		tolerancia = *pedido.ToleranciaMinutos
	}
	id := "sol-" + uuid.NewString()
	vista := models.VistaSolicitud{
		ID:                    id,
		ReferenciaExterna:     pedido.ReferenciaExterna,
		Estado:                models.EstadoRecibida,
		Cosa:                  pedido.Cosa,
		Origen:                pedido.Origen,
		Destino:               pedido.Destino,
		HoraLlegadaSolicitada: pedido.HoraLlegadaSolicitada,
		ToleranciaMinutos:     tolerancia,
		Observaciones:         pedido.Observaciones,
		Version:               0,
		CreadoEn:              ahora,
		ActualizadoEn:         ahora,
	}
	// Se guarda el contexto de la traza para que el relay publique dentro de la misma traza
	// (interfaces-internas §9), y el correlation id para el sobre del comando.
	contexto := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, contexto)
	if correlacion := observabilidad.Correlacion(ctx); correlacion != "" {
		contexto[ClaveCorrelacionTraza] = correlacion
	}
	solicitud := models.SolicitudExterna{
		ID:             id,
		OrganizacionID: org,
		Pedido:         pedido,
		Idempotencia:   models.ClaveIdempotencia{Clave: claveIdem, HashCuerpo: hash, Respuesta: vista},
		Vista:          vista,
		Version:        0,
		PendienteEnvio: true,
		ContextoTraza:  contexto,
		CreadoEn:       ahora,
		ActualizadoEn:  ahora,
	}

	guardada, err := s.repo.Crear(ctx, solicitud)
	if errors.Is(err, repositories.ErrClaveExistente) {
		if guardada.Idempotencia.HashCuerpo == hash {
			// Reintento idempotente: la misma respuesta original, sin duplicar (CA-27).
			slog.InfoContext(ctx, "reintento idempotente", slog.String("solicitud_id", guardada.ID))
			return guardada.Idempotencia.Respuesta, http.StatusAccepted, nil
		}
		return models.VistaSolicitud{}, 0, nuevoError(http.StatusConflict, "CLAVE_IDEMPOTENCIA_REUTILIZADA",
			"Clave de idempotencia reutilizada", "La Idempotency-Key ya se usó con un cuerpo distinto.")
	}
	if err != nil {
		return models.VistaSolicitud{}, 0, errorInterno(ctx, "guardar la solicitud", err)
	}
	slog.InfoContext(ctx, "solicitud recibida", slog.String("solicitud_id", id))
	return vista, http.StatusAccepted, nil
}

// validarPedido junta todos los errores por campo del cuerpo (DATOS_INVALIDOS).
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
	if p.ToleranciaMinutos != nil && (*p.ToleranciaMinutos < 0 || *p.ToleranciaMinutos > 720) {
		agregar("tolerancia_minutos", "debe estar entre 0 y 720")
	}
	textoOpcional("referencia_externa", p.ReferenciaExterna, 64)
	textoOpcional("observaciones", p.Observaciones, 500)
	return errores
}

// Obtener devuelve la copia del estado de una solicitud de la organización.
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
			"La solicitud no existe o pertenece a otra organización.")
	}
	if err != nil {
		return models.SolicitudExterna{}, errorInterno(ctx, "buscar la solicitud", err)
	}
	return solicitud, nil
}

// Listar devuelve una página de solicitudes de la organización, por actualizado_en ascendente.
func (s *SolicitudesService) Listar(ctx context.Context, filtro repositories.FiltroListado) (models.PaginaSolicitudes, error) {
	solicitudes, total, err := s.repo.Listar(ctx, filtro)
	if err != nil {
		return models.PaginaSolicitudes{}, errorInterno(ctx, "listar solicitudes", err)
	}
	pagina := models.PaginaSolicitudes{Resultados: make([]models.VistaSolicitud, 0, len(solicitudes)),
		Pagina: filtro.Pagina, Tamanio: filtro.Tamanio, Total: total}
	for _, sol := range solicitudes {
		pagina.Resultados = append(pagina.Resultados, sol.Vista)
	}
	return pagina, nil
}

// ResponderContrapropuesta reenvía la respuesta del consumidor a Operaciones, que es quien decide.
func (s *SolicitudesService) ResponderContrapropuesta(ctx context.Context, org, claveIdem, id string, acepta bool) (models.VistaSolicitud, error) {
	return s.reenviar(ctx, org, id, func() (models.VistaSolicitud, error) {
		return s.cliente.ResponderContrapropuesta(ctx, org, claveIdem, id, acepta)
	})
}

// Cancelar reenvía el pedido de cancelación a Operaciones (las reglas de 24 h son de Operaciones).
func (s *SolicitudesService) Cancelar(ctx context.Context, org, claveIdem, id, motivo string) (models.VistaSolicitud, error) {
	return s.reenviar(ctx, org, id, func() (models.VistaSolicitud, error) {
		return s.cliente.Cancelar(ctx, org, claveIdem, id, motivo)
	})
}

func (s *SolicitudesService) reenviar(ctx context.Context, org, id string, llamar func() (models.VistaSolicitud, error)) (models.VistaSolicitud, error) {
	local, err := s.buscar(ctx, org, id)
	if err != nil {
		return models.VistaSolicitud{}, err
	}
	vista, err := llamar()
	var conflicto clients.ErrConflicto
	switch {
	case err == nil:
		// Se aplica la vista que devolvió Operaciones (pasa por el decorador e invalida la caché).
		// Si falla, la operación igual ocurrió: se responde la verdad y el evento corregirá la copia.
		if _, _, errVista := s.repo.AplicarVista(ctx, id, vista.Version, vista, s.ahora()); errVista != nil {
			slog.WarnContext(ctx, "no se pudo actualizar la copia local", slog.String("solicitud_id", id),
				slog.String("error", errVista.Error()))
		}
		return vista, nil
	case errors.Is(err, clients.ErrNoEncontrada) && local.Vista.Estado == models.EstadoRecibida:
		// Operaciones todavía no procesó el comando: es transitorio (interfaces-internas §4.1).
		return models.VistaSolicitud{}, noDisponible("La solicitud todavía se está registrando. Reintentar con la misma Idempotency-Key.", ReintentarSinRecibir)
	case errors.Is(err, clients.ErrNoEncontrada):
		return models.VistaSolicitud{}, nuevoError(http.StatusNotFound, codigoNoEncontrada, tituloNoEncontrada,
			"La solicitud no existe o pertenece a otra organización.")
	case errors.As(err, &conflicto):
		return models.VistaSolicitud{}, nuevoError(http.StatusConflict, conflicto.Codigo,
			"El estado actual no permite la operación", "Consultar la solicitud para conocer su estado.")
	case errors.Is(err, clients.ErrDatosInvalidos):
		return models.VistaSolicitud{}, nuevoError(http.StatusUnprocessableEntity, codigoDatosInvalidos, tituloDatosInvalidos,
			"Operaciones rechazó los datos del pedido.")
	case errors.Is(err, clients.ErrNoDisponible):
		return models.VistaSolicitud{}, noDisponible("Rumbo no puede procesar el pedido en este momento.", ReintentarNoDisponible)
	default:
		return models.VistaSolicitud{}, errorInterno(ctx, "la llamada a Operaciones", err)
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
