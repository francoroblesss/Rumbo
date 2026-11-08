package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/francoroblesss/Rumbo/servicios/comun/identidad"
	"github.com/francoroblesss/Rumbo/servicios/comun/observabilidad"
	"github.com/francoroblesss/Rumbo/servicios/notificaciones/repositories"
)

// stream atiende GET /avisos/stream con Server-Sent Events (ADR-027, teórico 9): el cliente solo
// recibe, así que alcanza con un flujo HTTP servidor → cliente, sin WebSockets.
//
// No hay un hub en memoria: cada conexión consulta MongoDB cada segundo buscando los avisos
// visibles más nuevos que el último enviado. Así cualquier réplica sirve cualquier stream y un
// reinicio no pierde nada; lo que se paga es una consulta indexada por segundo por conexión. Los
// avisos ya están persistidos antes de salir por acá: si el stream se corta, la bandeja
// (GET /avisos) sigue siendo la fuente de verdad y la web reconecta con Last-Event-ID.
func (ac *AvisosController) stream(c *gin.Context) {
	ctx := c.Request.Context()
	// La ruta no pasa por Instrumentar (ver main.go): el correlation id del gateway se toma acá
	// para que los logs del stream lo lleven.
	if id := c.GetHeader(observabilidad.HeaderCorrelacion); id != "" {
		ctx = observabilidad.ConCorrelacion(ctx, id)
	}
	quien := identidad.Desde(c)
	cursor, err := ac.servicio.CursorInicial(ctx, quien, c.GetHeader("Last-Event-ID"))
	if err != nil {
		noDisponible(c, err)
		return
	}

	h := c.Writer.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("X-Accel-Buffering", "no") // que ningún proxy (NGINX) lo retenga en un buffer
	c.Status(http.StatusOK)
	c.Writer.Flush() // manda los headers ya: el cliente sabe que el stream está abierto

	consulta := time.NewTicker(ac.intervaloConsulta)
	defer consulta.Stop()
	ping := time.NewTicker(ac.intervaloPing)
	defer ping.Stop()

	// Primera consulta inmediata: lo que faltaba desde Last-Event-ID sale sin esperar el primer tic.
	var ok bool
	if cursor, ok = ac.enviarNuevos(ctx, c.Writer, quien, cursor); !ok {
		return
	}
	for {
		select {
		case <-ctx.Done(): // el cliente cerró la conexión (o se apaga el servidor)
			return
		case <-ping.C:
			if _, err := io.WriteString(c.Writer, ": ping\n\n"); err != nil {
				return
			}
			c.Writer.Flush()
		case <-consulta.C:
			if cursor, ok = ac.enviarNuevos(ctx, c.Writer, quien, cursor); !ok {
				return
			}
		}
	}
}

// enviarNuevos manda los avisos posteriores al cursor y devuelve el cursor nuevo. Devuelve false
// si hay que cortar el stream (el cliente se fue). Una falla de MongoDB no corta: se loguea y se
// reintenta en la consulta siguiente desde el mismo cursor, así no se saltea ningún aviso.
func (ac *AvisosController) enviarNuevos(ctx context.Context, w gin.ResponseWriter, quien identidad.Identidad, cursor repositories.Cursor) (repositories.Cursor, bool) {
	avisos, err := ac.servicio.Posteriores(ctx, quien, cursor, avisosPorConsulta)
	if err != nil {
		if ctx.Err() != nil {
			return cursor, false
		}
		slog.WarnContext(ctx, "no se pudieron consultar los avisos del stream", slog.String("error", err.Error()))
		return cursor, true
	}
	for _, a := range avisos {
		datos, err := json.Marshal(a.ParaRespuesta()) // json.Marshal no deja saltos de línea: data va en una sola
		if err != nil {
			slog.ErrorContext(ctx, "no se pudo serializar el aviso", slog.String("aviso_id", a.ID), slog.String("error", err.Error()))
			continue
		}
		if _, err := fmt.Fprintf(w, "event: aviso\nid: %s\ndata: %s\n\n", a.ID, datos); err != nil {
			return cursor, false
		}
		w.Flush()
		cursor = repositories.Cursor{CreadoEn: a.CreadoEn, ID: a.ID}
		// Evidencia de CA-41 (menos de 2 s): demora desde que el aviso se guardó hasta que salió.
		slog.InfoContext(ctx, "aviso enviado por stream", slog.String("aviso_id", a.ID),
			slog.Int64("demora_ms", time.Since(a.CreadoEn).Milliseconds()))
	}
	return cursor, true
}
