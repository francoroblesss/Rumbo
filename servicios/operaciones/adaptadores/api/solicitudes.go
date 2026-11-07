package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/francoroblesss/Rumbo/servicios/comun/identidad"
	"github.com/francoroblesss/Rumbo/servicios/comun/problema"
	"github.com/francoroblesss/Rumbo/servicios/operaciones/dominio"
	"github.com/francoroblesss/Rumbo/servicios/operaciones/puertos"
)

const (
	headerIdempotencia = "Idempotency-Key"
	largoMaximoClave   = 128 // columna claves_idempotencia.clave
	paginaPorDefecto   = 1
	tamanioPorDefecto  = 20
)

// Handler es el adaptador de entrada HTTP: traduce requests a llamadas a los casos de uso y los
// resultados (o errores) a JSON. No tiene reglas de negocio.
type Handler struct {
	casos puertos.Solicitudes
}

func NuevoHandler(casos puertos.Solicitudes) *Handler {
	return &Handler{casos: casos}
}

// Registrar agrega las rutas (docs/interfaces-internas.md §4.1 y §12).
func (h *Handler) Registrar(r gin.IRouter) {
	// API de la aplicación web: llega por el gateway (/api/operaciones/...), que ya validó el
	// token. La autorización la decide Operaciones sobre sus datos (teórico 6).
	personal := identidad.RequiereRol(identidad.RolOperador, identidad.RolAdministrador)
	conCliente := identidad.RequiereRol(identidad.RolOperador, identidad.RolAdministrador, identidad.RolCliente)
	r.GET("/franjas", personal, h.franjas)
	r.GET("/solicitudes", conCliente, h.listar)
	r.GET("/solicitudes/:id", conCliente, h.obtener)
	r.POST("/solicitudes/:id/estado", personal, h.avanzarEstado)
	r.POST("/solicitudes/:id/cancelacion", conCliente, h.cancelarDesdeWeb)

	// Endpoints internos para Integración de carga (capacidad publicada, ADR-021): el gateway no
	// expone /internas/.
	internas := r.Group("/internas")
	internas.GET("/disponibilidad", h.disponibilidad)
	internas.POST("/reservas", h.reservar)
	internas.POST("/solicitudes/:id/cancelacion", h.cancelar)
}

// organizacionWeb es la organización con la que se filtra lo que pide la web (§11.1): "" para el
// personal de Rumbo (ve todas) y la del token para el cliente, nunca un parámetro. Un cliente sin
// organización no puede ver nada: 403.
func organizacionWeb(c *gin.Context) (string, bool) {
	id := identidad.Desde(c)
	if id.TieneRol(identidad.RolOperador, identidad.RolAdministrador) {
		return "", true
	}
	if strings.TrimSpace(id.Organizacion) == "" {
		problema.Escribir(c, http.StatusForbidden, "SIN_PERMISO", "Sin permiso",
			"La identidad no tiene una organización.")
		return "", false
	}
	return id.Organizacion, true
}

func (h *Handler) franjas(c *gin.Context) {
	fecha := c.Query("fecha")
	if fecha == "" {
		datosInvalidos(c, "fecha", "es obligatorio (AAAA-MM-DD)")
		return
	}
	d, err := h.casos.Franjas(c.Request.Context(), fecha)
	if errors.Is(err, dominio.ErrDatosInvalidos) {
		datosInvalidos(c, "fecha", "debe ser AAAA-MM-DD, entre hoy y el fin del período reservable")
		return
	}
	if err != nil {
		h.error(c, err)
		return
	}
	c.JSON(http.StatusOK, d)
}

func (h *Handler) listar(c *gin.Context) {
	org, ok := organizacionWeb(c)
	if !ok {
		return
	}
	if org == "" {
		// Solo el personal de Rumbo elige la organización; al cliente se le ignora el parámetro.
		org = c.Query("organizacion_id")
	}
	pagina, ok1 := entero(c, "pagina", paginaPorDefecto)
	tamanio, ok2 := entero(c, "tamanio", tamanioPorDefecto)
	if !ok1 || !ok2 {
		return
	}
	p, err := h.casos.Listar(c.Request.Context(), puertos.FiltroListado{Organizacion: org, Fecha: c.Query("fecha"),
		EstadoTraslado: c.Query("estado_traslado"), Pagina: pagina, Tamanio: tamanio})
	if errors.Is(err, dominio.ErrDatosInvalidos) {
		datosInvalidos(c, "fecha, estado_traslado, pagina o tamanio",
			"fecha AAAA-MM-DD; estado_traslado PROGRAMADO, EN_CURSO, COMPLETADO o CANCELADO; pagina desde 1; tamanio de 1 a 100")
		return
	}
	if err != nil {
		h.error(c, err)
		return
	}
	c.JSON(http.StatusOK, p)
}

func (h *Handler) obtener(c *gin.Context) {
	org, ok := organizacionWeb(c)
	if !ok {
		return
	}
	h.responder(c)(h.casos.Obtener(c.Request.Context(), org, c.Param("id")))
}

func (h *Handler) avanzarEstado(c *gin.Context) {
	var cuerpo struct {
		Estado *string `json:"estado"`
	}
	if _, ok := leerCuerpo(c, &cuerpo); !ok {
		return
	}
	if cuerpo.Estado == nil || (*cuerpo.Estado != string(dominio.TrasladoEnCurso) && *cuerpo.Estado != string(dominio.TrasladoCompletado)) {
		datosInvalidos(c, "estado", "debe ser EN_CURSO o COMPLETADO")
		return
	}
	h.responder(c)(h.casos.AvanzarEstado(c.Request.Context(), "", c.Param("id"), dominio.EstadoTraslado(*cuerpo.Estado)))
}

func (h *Handler) cancelarDesdeWeb(c *gin.Context) {
	org, ok := organizacionWeb(c)
	if !ok {
		return
	}
	motivo, crudo, ok := leerMotivo(c)
	if !ok {
		return
	}
	idem, ok := idempotencia(c, "cancelacion", crudo)
	if !ok {
		return
	}
	h.responder(c)(h.casos.CancelarDesdeWeb(c.Request.Context(), org, c.Param("id"), motivo, idem))
}

func (h *Handler) disponibilidad(c *gin.Context) {
	fecha := c.Query("fecha")
	if fecha == "" {
		datosInvalidos(c, "fecha", "es obligatorio (AAAA-MM-DD)")
		return
	}
	peso := 0.0
	if v := c.Query("peso_kg"); v != "" {
		var err error
		if peso, err = strconv.ParseFloat(v, 64); err != nil || peso <= 0 {
			datosInvalidos(c, "peso_kg", "debe ser un número mayor que 0")
			return
		}
	}
	d, err := h.casos.Disponibilidad(c.Request.Context(), fecha, peso)
	if errors.Is(err, dominio.ErrFranjaInvalida) || errors.Is(err, dominio.ErrDatosInvalidos) {
		datosInvalidos(c, "fecha", "debe ser AAAA-MM-DD, desde hoy y dentro del período reservable")
		return
	}
	if err != nil {
		h.error(c, err)
		return
	}
	c.JSON(http.StatusOK, d)
}

// pedidoReserva es el cuerpo de POST /internas/reservas: lo que mandó el consumidor externo, tal
// como lo reenvía Integración.
type pedidoReserva struct {
	ReferenciaExterna *string `json:"referencia_externa"`
	Cosa              struct {
		Descripcion string   `json:"descripcion"`
		Cantidad    int      `json:"cantidad"`
		PesoKg      *float64 `json:"peso_kg"`
		VolumenM3   *float64 `json:"volumen_m3"`
	} `json:"cosa"`
	Origen        ubicacion `json:"origen"`
	Destino       ubicacion `json:"destino"`
	FranjaInicio  *string   `json:"franja_inicio"`
	Observaciones *string   `json:"observaciones"`
}

type ubicacion struct {
	Nombre    string  `json:"nombre"`
	Direccion string  `json:"direccion"`
	Contacto  *string `json:"contacto"`
}

func (u ubicacion) dominio() dominio.Ubicacion {
	return dominio.Ubicacion{Nombre: u.Nombre, Direccion: u.Direccion, Contacto: u.Contacto}
}

func (h *Handler) reservar(c *gin.Context) {
	org, ok := organizacion(c)
	if !ok {
		return
	}
	var p pedidoReserva
	crudo, ok := leerCuerpo(c, &p)
	if !ok {
		return
	}
	if p.FranjaInicio == nil {
		datosInvalidos(c, "franja_inicio", "es obligatorio")
		return
	}
	inicio, err := time.Parse(time.RFC3339, *p.FranjaInicio)
	if err != nil {
		datosInvalidos(c, "franja_inicio", "debe ser una fecha y hora RFC 3339 con zona")
		return
	}
	idem, ok := idempotencia(c, "reserva", crudo)
	if !ok {
		return
	}
	datos := dominio.DatosSolicitud{
		ReferenciaExterna: p.ReferenciaExterna,
		Cosa: dominio.Cosa{Descripcion: p.Cosa.Descripcion, Cantidad: p.Cosa.Cantidad,
			PesoKg: p.Cosa.PesoKg, VolumenM3: p.Cosa.VolumenM3},
		Desde:         p.Origen.dominio(),
		Hasta:         p.Destino.dominio(),
		Observaciones: p.Observaciones,
	}
	v, err := h.casos.Reservar(c.Request.Context(), org, datos, inicio.UTC(), idem)
	if err != nil {
		h.error(c, err)
		return
	}
	c.JSON(http.StatusCreated, v)
}

func (h *Handler) cancelar(c *gin.Context) {
	org, ok := organizacion(c)
	if !ok {
		return
	}
	motivo, crudo, ok := leerMotivo(c)
	if !ok {
		return
	}
	idem, ok := idempotencia(c, "cancelacion", crudo)
	if !ok {
		return
	}
	h.responder(c)(h.casos.Cancelar(c.Request.Context(), org, c.Param("id"), motivo, idem))
}

// responder escribe la vista o traduce el error.
func (h *Handler) responder(c *gin.Context) func(puertos.Vista, error) {
	return func(v puertos.Vista, err error) {
		if err != nil {
			h.error(c, err)
			return
		}
		c.JSON(http.StatusOK, v)
	}
}

// error traduce los errores del núcleo a Problem Details con los códigos del contrato. Lo que no
// es un error conocido es una falla de infraestructura (base caída): 503, reintentable.
func (h *Handler) error(c *gin.Context, err error) {
	switch {
	case errors.Is(err, puertos.ErrNoEncontrada):
		problema.Escribir(c, http.StatusNotFound, "NO_ENCONTRADA", "Solicitud no encontrada",
			"La solicitud no existe o es de otra organización.")
	case errors.Is(err, puertos.ErrClaveReutilizada):
		problema.Escribir(c, http.StatusConflict, "CLAVE_IDEMPOTENCIA_REUTILIZADA", "Clave de idempotencia reutilizada",
			"La Idempotency-Key ya se usó con otro cuerpo.")
	case errors.Is(err, dominio.ErrEstadoNoPermite):
		problema.Escribir(c, http.StatusConflict, "ESTADO_NO_PERMITE_OPERACION", "El estado actual no permite la operación",
			"La solicitud o su traslado no están en un estado que admita esta operación.")
	case errors.Is(err, dominio.ErrHorarioEnElPasado):
		problema.Escribir(c, http.StatusUnprocessableEntity, "HORARIO_EN_EL_PASADO", "Horario en el pasado",
			"La hora indicada ya pasó.")
	case errors.Is(err, dominio.ErrSinDisponibilidad):
		problema.Escribir(c, http.StatusConflict, "SIN_DISPONIBILIDAD", "Sin disponibilidad",
			"La franja ya no tiene lugar para esta carga. Consultar la disponibilidad y elegir otra.")
	case errors.Is(err, dominio.ErrExcedeCapacidad):
		problema.Escribir(c, http.StatusUnprocessableEntity, "EXCEDE_CAPACIDAD", "Excede la capacidad",
			"El peso supera el cupo total de una franja: dividir la carga en varios traslados.")
	case errors.Is(err, dominio.ErrAnticipacionInsuficiente):
		problema.Escribir(c, http.StatusUnprocessableEntity, "ANTICIPACION_INSUFICIENTE", "Anticipación insuficiente",
			"La franja empieza demasiado pronto para reservarla. Elegir una franja posterior.")
	case errors.Is(err, dominio.ErrFranjaInvalida):
		problema.Escribir(c, http.StatusUnprocessableEntity, "DATOS_INVALIDOS", "Datos inválidos",
			"No es una franja que se pueda reservar: consultar la disponibilidad.",
			problema.ErrorCampo{Campo: "franja_inicio", Mensaje: "no es el inicio de una franja reservable"})
	case errors.Is(err, dominio.ErrDatosInvalidos):
		problema.Escribir(c, http.StatusUnprocessableEntity, "DATOS_INVALIDOS", "Datos inválidos",
			"Falta un dato obligatorio o un valor no es válido.")
	default:
		slog.ErrorContext(c.Request.Context(), "error al atender la request", slog.String("solicitud_id", c.Param("id")),
			slog.String("error", err.Error()))
		problema.Escribir(c, http.StatusServiceUnavailable, "SERVICIO_NO_DISPONIBLE", "Servicio no disponible",
			"Operaciones no puede procesar el pedido ahora. Reintentar más tarde.")
	}
}

// leerCuerpo decodifica el cuerpo JSON y devuelve los bytes crudos (para el hash de idempotencia).
// JSON mal formado → 400; un campo con un tipo equivocado → 422.
func leerCuerpo(c *gin.Context, destino any) ([]byte, bool) {
	crudo, err := io.ReadAll(c.Request.Body)
	if err != nil {
		jsonMalFormado(c)
		return nil, false
	}
	if err := json.Unmarshal(crudo, destino); err != nil {
		var tipo *json.UnmarshalTypeError
		if errors.As(err, &tipo) {
			datosInvalidos(c, tipo.Field, "tiene un tipo inválido")
		} else {
			jsonMalFormado(c)
		}
		return nil, false
	}
	return crudo, true
}

// leerMotivo lee el cuerpo {"motivo": "…"} de una cancelación y devuelve también los bytes
// crudos, para el hash de la clave de idempotencia.
func leerMotivo(c *gin.Context) (string, []byte, bool) {
	var cuerpo struct {
		Motivo *string `json:"motivo"`
	}
	crudo, ok := leerCuerpo(c, &cuerpo)
	if !ok {
		return "", nil, false
	}
	if cuerpo.Motivo == nil || strings.TrimSpace(*cuerpo.Motivo) == "" {
		datosInvalidos(c, "motivo", "es obligatorio")
		return "", nil, false
	}
	return *cuerpo.Motivo, crudo, true
}

// organizacion lee la organización que manda Integración en los endpoints internos (la del
// consumidor externo).
func organizacion(c *gin.Context) (string, bool) {
	org := strings.TrimSpace(c.GetHeader(identidad.HeaderOrganizacion))
	if org == "" {
		datosInvalidos(c, identidad.HeaderOrganizacion, "es obligatorio")
		return "", false
	}
	return org, true
}

// idempotencia arma la clave del pedido: endpoint, Idempotency-Key y hash del cuerpo.
func idempotencia(c *gin.Context, endpoint string, cuerpo []byte) (*puertos.Idempotencia, bool) {
	clave := strings.TrimSpace(c.GetHeader(headerIdempotencia))
	if clave == "" || len(clave) > largoMaximoClave {
		datosInvalidos(c, headerIdempotencia, "es obligatorio y tiene hasta 128 caracteres")
		return nil, false
	}
	hash := sha256.Sum256(cuerpo)
	return &puertos.Idempotencia{Endpoint: endpoint, Clave: clave, Hash: hex.EncodeToString(hash[:])}, true
}

// entero lee un parámetro de query numérico; si falta, usa el valor por defecto.
func entero(c *gin.Context, nombre string, porDefecto int) (int, bool) {
	texto := c.Query(nombre)
	if texto == "" {
		return porDefecto, true
	}
	n, err := strconv.Atoi(texto)
	if err != nil {
		problema.Escribir(c, http.StatusBadRequest, "JSON_MAL_FORMADO", "Parámetro mal formado",
			"El parámetro "+nombre+" debe ser un número entero.")
		return 0, false
	}
	return n, true
}

func jsonMalFormado(c *gin.Context) {
	problema.Escribir(c, http.StatusBadRequest, "JSON_MAL_FORMADO", "JSON mal formado", "El cuerpo no es JSON válido.")
}

func datosInvalidos(c *gin.Context, campo, mensaje string) {
	problema.Escribir(c, http.StatusUnprocessableEntity, "DATOS_INVALIDOS", "Datos inválidos",
		"Falta un dato obligatorio o un valor no es válido.", problema.ErrorCampo{Campo: campo, Mensaje: mensaje})
}
