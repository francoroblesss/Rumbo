package services

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/francoroblesss/Rumbo/servicios/comun/identidad"
	"github.com/francoroblesss/Rumbo/servicios/comun/mensajeria"
	"github.com/francoroblesss/Rumbo/servicios/notificaciones/models"
	"github.com/francoroblesss/Rumbo/servicios/notificaciones/repositories"
)

// ErrNoEncontrado: el aviso no existe o no es para quien llama (para el que llama es lo mismo:
// no se revela que existe un aviso de otra organización).
var ErrNoEncontrado = repositories.ErrNoEncontrado

// sinLastEventID es lo que se reenvía al abrir el stream con un Last-Event-ID que no existe.
const sinLastEventID = time.Minute

// AvisosService arma los avisos a partir de los eventos y atiende la bandeja y el stream.
type AvisosService struct {
	repo  repositories.AvisosRepository
	ahora func() time.Time
}

// NewAvisosService crea el servicio. ahora se inyecta para poder fijar el reloj en los tests.
func NewAvisosService(repo repositories.AvisosRepository, ahora func() time.Time) *AvisosService {
	return &AvisosService{repo: repo, ahora: ahora}
}

// ProcesarEvento es el manejador del consumidor (mensajeria.Manejador). Es idempotente: el _id
// de cada aviso deriva del id del mensaje, así una entrega repetida (at-least-once) encuentra el
// aviso ya guardado y no lo duplica. Devuelve nil recién después de guardar todos los avisos:
// comun/mensajeria confirma (ACK) después del efecto. Un error de MongoDB se devuelve tal cual
// (transitorio, se reintenta); los avisos que ya se guardaron en el intento anterior dan
// "ya registrado" en el siguiente.
func (s *AvisosService) ProcesarEvento(ctx context.Context, sobre mensajeria.Sobre) error {
	avisos, err := avisosPara(sobre)
	if err != nil {
		return err // permanente: va a la DLQ
	}
	// MongoDB guarda las fechas con precisión de milisegundos: se trunca antes para que el cursor
	// del stream compare lo mismo que quedó guardado.
	creadoEn := s.ahora().UTC().Truncate(time.Millisecond)
	for _, a := range avisos {
		a.CreadoEn = creadoEn
		err := s.repo.Insertar(ctx, a)
		if errors.Is(err, repositories.ErrAvisoExistente) {
			slog.InfoContext(ctx, "aviso ya registrado", atributos(sobre, a)...)
			continue
		}
		if err != nil {
			return err
		}
		slog.InfoContext(ctx, "aviso registrado", atributos(sobre, a)...)
	}
	return nil
}

func atributos(sobre mensajeria.Sobre, a models.Aviso) []any {
	return []any{slog.String("aviso_id", a.ID), slog.String("tipo_mensaje", sobre.Tipo),
		slog.String("destinatario", a.Destinatario.Tipo+" "+a.Destinatario.Valor)}
}

// Visibles devuelve los destinatarios cuyos avisos ve la identidad (§14): (ROL, r) por cada uno
// de sus roles, (ROL, operador) si es administrador, y (ORGANIZACION, org) si tiene organización.
// La autorización es de este servicio, con la identidad que propagó el gateway (teórico 6).
func Visibles(id identidad.Identidad) []models.Destinatario {
	var visibles []models.Destinatario
	for _, r := range id.Roles {
		visibles = append(visibles, models.Destinatario{Tipo: models.DestinatarioRol, Valor: r})
	}
	if id.TieneRol(identidad.RolAdministrador) && !id.TieneRol(identidad.RolOperador) {
		visibles = append(visibles, models.Destinatario{Tipo: models.DestinatarioRol, Valor: identidad.RolOperador})
	}
	if id.Organizacion != "" {
		visibles = append(visibles, models.Destinatario{Tipo: models.DestinatarioOrganizacion, Valor: id.Organizacion})
	}
	return visibles
}

// Listar devuelve una página de la bandeja, más nuevos primero.
func (s *AvisosService) Listar(ctx context.Context, id identidad.Identidad, soloNoLeidos bool, pagina, tamanio int) (models.PaginaAvisos, error) {
	avisos, total, err := s.repo.Listar(ctx, Visibles(id), soloNoLeidos, pagina, tamanio)
	if err != nil {
		return models.PaginaAvisos{}, err
	}
	return models.PaginaAvisos{Resultados: avisos, Pagina: pagina, Tamanio: tamanio, Total: total}, nil
}

// MarcarLeido marca el aviso como leído para todo su destinatario (ADR-027). ErrNoEncontrado si
// no existe o no es visible para quien llama.
func (s *AvisosService) MarcarLeido(ctx context.Context, id identidad.Identidad, avisoID string) error {
	return s.repo.MarcarLeido(ctx, Visibles(id), avisoID)
}

// CursorInicial es desde dónde arranca un stream (§14): con un Last-Event-ID que existe (y que
// es visible para quien llama), después de ese aviso; si no existe, desde hace un minuto; sin el
// header, desde ahora (ningún aviso viejo).
func (s *AvisosService) CursorInicial(ctx context.Context, id identidad.Identidad, ultimoID string) (repositories.Cursor, error) {
	ahora := s.ahora().UTC()
	if ultimoID == "" {
		return repositories.Cursor{CreadoEn: ahora}, nil
	}
	a, err := s.repo.Buscar(ctx, Visibles(id), ultimoID)
	if errors.Is(err, repositories.ErrNoEncontrado) {
		return repositories.Cursor{CreadoEn: ahora.Add(-sinLastEventID)}, nil
	}
	if err != nil {
		return repositories.Cursor{}, err
	}
	return repositories.Cursor{CreadoEn: a.CreadoEn, ID: a.ID}, nil
}

// Posteriores devuelve los avisos visibles posteriores al cursor, en orden.
func (s *AvisosService) Posteriores(ctx context.Context, id identidad.Identidad, desde repositories.Cursor, limite int) ([]models.Aviso, error) {
	return s.repo.Posteriores(ctx, Visibles(id), desde, limite)
}
