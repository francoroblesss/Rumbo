package messaging

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"

	"github.com/francoroblesss/Rumbo/servicios/comun/mensajeria"
	"github.com/francoroblesss/Rumbo/servicios/comun/observabilidad"
	"github.com/francoroblesss/Rumbo/servicios/integracion/models"
	"github.com/francoroblesss/Rumbo/servicios/integracion/repositories"
	"github.com/francoroblesss/Rumbo/servicios/integracion/services"
)

// Comando que se envía a Operaciones (interfaces-internas §5.3).
const (
	ColaSolicitudesExternas = "operaciones.solicitudes-externas"
	TipoSolicitudRecibida   = "integracion.solicitud_carga.recibida"
	IntervaloRelay          = time.Second
)

// Publicador es lo que el relay necesita de comun/mensajeria.Publicador.
type Publicador interface {
	DeclararCola(nombre string) error
	Publicar(ctx context.Context, exchange, routingKey string, s mensajeria.Sobre) error
}

// datosSolicitudRecibida son los datos del comando. Los opcionales viajan como null.
type datosSolicitudRecibida struct {
	SolicitudID           string           `json:"solicitud_id"`
	OrganizacionID        string           `json:"organizacion_id"`
	ReferenciaExterna     *string          `json:"referencia_externa"`
	Cosa                  cosaComando      `json:"cosa"`
	Origen                ubicacionComando `json:"origen"`
	Destino               ubicacionComando `json:"destino"`
	HoraLlegadaSolicitada *time.Time       `json:"hora_llegada_solicitada"`
	ToleranciaMinutos     *int             `json:"tolerancia_minutos"`
	Observaciones         *string          `json:"observaciones"`
	RecibidaEn            time.Time        `json:"recibida_en"`
}

type cosaComando struct {
	Descripcion string   `json:"descripcion"`
	Cantidad    int      `json:"cantidad"`
	PesoKg      *float64 `json:"peso_kg"`
	VolumenM3   *float64 `json:"volumen_m3"`
}

type ubicacionComando struct {
	Nombre    string  `json:"nombre"`
	Direccion string  `json:"direccion"`
	Contacto  *string `json:"contacto"`
}

// Relay publica las solicitudes pendientes de envío (polling publisher, teórico 4). Con dos
// réplicas, cada una reclama una solicitud de forma atómica antes de publicarla, así no se
// envía dos veces; si una réplica no llega a marcarla, el reclamo vence y se reintenta, y
// Operaciones descarta el duplicado por id.
type Relay struct {
	repo       repositories.SolicitudesRepository
	publicador Publicador
	instancia  string
}

// NewRelay crea el relay y registra el gauge rumbo_outbox_pendientes.
func NewRelay(repo repositories.SolicitudesRepository, publicador Publicador, instancia string) *Relay {
	_, err := observabilidad.Medidor().Int64ObservableGauge("rumbo_outbox_pendientes",
		metric.WithDescription("Solicitudes pendientes de envío a Operaciones"),
		metric.WithInt64Callback(func(ctx context.Context, o metric.Int64Observer) error {
			n, err := repo.ContarPendientes(ctx)
			if err != nil {
				return err
			}
			o.Observe(n, metric.WithAttributes(attribute.String("servicio", "integracion")))
			return nil
		}))
	if err != nil {
		slog.Warn("no se pudo registrar el gauge de pendientes", slog.String("error", err.Error()))
	}
	return &Relay{repo: repo, publicador: publicador, instancia: instancia}
}

// Ejecutar declara la cola del comando y, cada segundo, publica las pendientes hasta que no
// quede ninguna. Termina cuando se cancela ctx.
func (r *Relay) Ejecutar(ctx context.Context) error {
	// Si el broker no está, la cola se declara cuando el publicador abra su próximo canal.
	if err := r.publicador.DeclararCola(ColaSolicitudesExternas); err != nil {
		slog.WarnContext(ctx, "no se pudo declarar la cola del comando, se reintenta al publicar",
			slog.String("error", err.Error()))
	}
	ticker := time.NewTicker(IntervaloRelay)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			r.publicarPendientes(ctx)
		}
	}
}

func (r *Relay) publicarPendientes(ctx context.Context) {
	for ctx.Err() == nil {
		s, err := r.repo.ReclamarPendiente(ctx, r.instancia, time.Now().UTC())
		if errors.Is(err, repositories.ErrNoHayPendientes) {
			return
		}
		if err != nil {
			slog.ErrorContext(ctx, "no se pudo reclamar una solicitud pendiente", slog.String("error", err.Error()))
			return
		}
		if !r.publicar(ctx, s) {
			// Probablemente el broker está caído: se espera al próximo ciclo. El reclamo vence a
			// los 30 s y la solicitud se vuelve a intentar.
			return
		}
	}
}

func (r *Relay) publicar(ctx context.Context, s models.SolicitudExterna) bool {
	// Se continúa la traza del POST que creó la solicitud (interfaces-internas §9).
	ctxMensaje := otel.GetTextMapPropagator().Extract(ctx, propagation.MapCarrier(s.ContextoTraza))
	if correlacion := s.ContextoTraza[services.ClaveCorrelacionTraza]; correlacion != "" {
		ctxMensaje = observabilidad.ConCorrelacion(ctxMensaje, correlacion)
	}
	p := s.Pedido
	datos := datosSolicitudRecibida{
		SolicitudID:           s.ID,
		OrganizacionID:        s.OrganizacionID,
		ReferenciaExterna:     p.ReferenciaExterna,
		Cosa:                  cosaComando{Descripcion: p.Cosa.Descripcion, Cantidad: p.Cosa.Cantidad, PesoKg: p.Cosa.PesoKg, VolumenM3: p.Cosa.VolumenM3},
		Origen:                ubicacionComando{Nombre: p.Origen.Nombre, Direccion: p.Origen.Direccion, Contacto: p.Origen.Contacto},
		Destino:               ubicacionComando{Nombre: p.Destino.Nombre, Direccion: p.Destino.Direccion, Contacto: p.Destino.Contacto},
		HoraLlegadaSolicitada: models.HoraLocalPtr(p.HoraLlegadaSolicitada),
		ToleranciaMinutos:     p.ToleranciaMinutos,
		Observaciones:         p.Observaciones,
		RecibidaEn:            models.HoraLocal(s.CreadoEn),
	}
	sobre, err := mensajeria.NuevoSobre(ctxMensaje, TipoSolicitudRecibida, s.ID, 1, datos)
	if err != nil {
		slog.ErrorContext(ctxMensaje, "no se pudo armar el comando", slog.String("solicitud_id", s.ID), slog.String("error", err.Error()))
		return false
	}
	if err := r.publicador.Publicar(ctxMensaje, "", ColaSolicitudesExternas, sobre); err != nil {
		slog.ErrorContext(ctxMensaje, "no se pudo publicar la solicitud, se reintenta al vencer el reclamo",
			slog.String("solicitud_id", s.ID), slog.String("error", err.Error()))
		return false
	}
	if err := r.repo.MarcarEnviada(ctxMensaje, s.ID); err != nil {
		// Se publicó pero no se marcó: se volverá a publicar y Operaciones lo ignora por id.
		slog.ErrorContext(ctxMensaje, "no se pudo marcar la solicitud como enviada", slog.String("solicitud_id", s.ID),
			slog.String("error", err.Error()))
		return false
	}
	slog.InfoContext(ctxMensaje, "solicitud enviada a Operaciones", slog.String("solicitud_id", s.ID))
	return true
}
