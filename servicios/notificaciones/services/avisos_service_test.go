package services

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/francoroblesss/Rumbo/servicios/comun/identidad"
	"github.com/francoroblesss/Rumbo/servicios/comun/mensajeria"
	"github.com/francoroblesss/Rumbo/servicios/notificaciones/models"
	"github.com/francoroblesss/Rumbo/servicios/notificaciones/repositories"
)

var relojFijo = time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)

func TestProcesarEventoRepetidoNoEsError(t *testing.T) {
	repo := repositories.NewAvisosMock()
	s := NewAvisosService(repo, func() time.Time { return relojFijo })
	ev := sobre(t, "operaciones.solicitud.aceptada", vista("PROGRAMADO", nil))
	for i := 0; i < 2; i++ {
		if err := s.ProcesarEvento(context.Background(), ev); err != nil {
			t.Fatalf("entrega %d: %v", i+1, err)
		}
	}
	avisos, total, _ := repo.Listar(context.Background(), []models.Destinatario{alOperador}, false, 1, 20)
	if total != 1 || !avisos[0].CreadoEn.Equal(relojFijo) {
		t.Fatalf("se esperaba un solo aviso con creado_en del reloj: %+v", avisos)
	}
}

func TestProcesarEventoMongoCaidoEsTransitorio(t *testing.T) {
	repo := repositories.NewAvisosMock()
	repo.Error = errors.New("mongo caído")
	s := NewAvisosService(repo, time.Now)
	err := s.ProcesarEvento(context.Background(), sobre(t, "operaciones.solicitud.aceptada", vista("PROGRAMADO", nil)))
	if err == nil || mensajeria.EsPermanente(err) {
		t.Fatalf("un error de MongoDB se reintenta (no es permanente): %v", err)
	}
}

func TestVisibles(t *testing.T) {
	casos := []struct {
		nombre string
		id     identidad.Identidad
		quiere []models.Destinatario
	}{
		{"operador", identidad.Identidad{Roles: []string{"operador"}}, []models.Destinatario{alOperador}},
		{"administrador ve lo del operador", identidad.Identidad{Roles: []string{"administrador"}},
			[]models.Destinatario{{Tipo: "ROL", Valor: "administrador"}, alOperador}},
		{"cliente con organización", identidad.Identidad{Organizacion: "org-almacenamiento", Roles: []string{"cliente"}},
			[]models.Destinatario{{Tipo: "ROL", Valor: "cliente"}, aLaOrg}},
	}
	for _, c := range casos {
		if got := Visibles(c.id); !slices.Equal(got, c.quiere) {
			t.Errorf("%s: %+v, se esperaba %+v", c.nombre, got, c.quiere)
		}
	}
}

func TestCursorInicial(t *testing.T) {
	repo := repositories.NewAvisosMock()
	_ = repo.Insertar(context.Background(), models.Aviso{ID: "a-1", Destinatario: alOperador, CreadoEn: relojFijo.Add(-time.Hour)})
	s := NewAvisosService(repo, func() time.Time { return relojFijo })
	operador := identidad.Identidad{Roles: []string{"operador"}}
	ctx := context.Background()

	if c, _ := s.CursorInicial(ctx, operador, ""); !c.CreadoEn.Equal(relojFijo) {
		t.Errorf("sin Last-Event-ID arranca desde ahora: %+v", c)
	}
	if c, _ := s.CursorInicial(ctx, operador, "a-1"); c.ID != "a-1" || !c.CreadoEn.Equal(relojFijo.Add(-time.Hour)) {
		t.Errorf("con un Last-Event-ID que existe arranca después de ese aviso: %+v", c)
	}
	if c, _ := s.CursorInicial(ctx, operador, "no-existe"); !c.CreadoEn.Equal(relojFijo.Add(-time.Minute)) {
		t.Errorf("con un Last-Event-ID que no existe arranca hace un minuto: %+v", c)
	}
	cliente := identidad.Identidad{Organizacion: "org-otra", Roles: []string{"cliente"}}
	if c, _ := s.CursorInicial(ctx, cliente, "a-1"); !c.CreadoEn.Equal(relojFijo.Add(-time.Minute)) {
		t.Errorf("un aviso que no es para quien llama cuenta como inexistente: %+v", c)
	}
}
