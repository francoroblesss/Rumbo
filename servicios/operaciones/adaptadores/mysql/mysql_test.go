package mysql

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"

	"github.com/francoroblesss/Rumbo/servicios/comun/mensajeria"
	"github.com/francoroblesss/Rumbo/servicios/operaciones/aplicacion"
	"github.com/francoroblesss/Rumbo/servicios/operaciones/dominio"
	"github.com/francoroblesss/Rumbo/servicios/operaciones/puertos"
)

// Tests de integración con un MySQL 8.4 real en un contenedor (testcontainers): prueban las
// migraciones, el SQL, los bloqueos y el relay, que con dobles en memoria no se verían.

var ahora = time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)

func baseDePrueba(t *testing.T) (*UnidadDeTrabajo, string) {
	t.Helper()
	ctx := context.Background()
	contenedor, err := tcmysql.Run(ctx, "mysql:8.4",
		tcmysql.WithDatabase("operaciones"), tcmysql.WithUsername("operaciones"), tcmysql.WithPassword("prueba"))
	if err != nil {
		t.Fatalf("levantar MySQL (¿Docker está corriendo?): %v", err)
	}
	t.Cleanup(func() { _ = contenedor.Terminate(context.Background()) })
	dsn, err := contenedor.ConnectionString(ctx, "parseTime=true", "multiStatements=true")
	if err != nil {
		t.Fatal(err)
	}
	if err := Migrar(ctx, dsn); err != nil {
		t.Fatalf("migrar: %v", err)
	}
	// Aplicar de nuevo no tiene que fallar (el servicio migra en cada arranque).
	if err := Migrar(ctx, dsn); err != nil {
		t.Fatalf("migrar por segunda vez: %v", err)
	}
	db, err := Conectar(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return NuevaUnidadDeTrabajo(db), dsn
}

type climaFijo struct {
	o   dominio.Operabilidad
	err error
}

func (c climaFijo) Operabilidad(context.Context, string) (dominio.Operabilidad, error) {
	return c.o, c.err
}

// publicadorFalso guarda lo publicado y puede fallar a partir del n-ésimo mensaje.
type publicadorFalso struct {
	mu        sync.Mutex
	recibidos []mensajeria.Sobre
	fallarEn  int // 0 = nunca
}

func (p *publicadorFalso) Publicar(_ context.Context, _, _ string, s mensajeria.Sobre) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.fallarEn > 0 && len(p.recibidos)+1 >= p.fallarEn {
		return errors.New("broker caído")
	}
	p.recibidos = append(p.recibidos, s)
	return nil
}

func comando(mensajeID, id string, hora *time.Time) puertos.ComandoSolicitudExterna {
	peso := 350.0
	ref := "TRF-2026-00123"
	return puertos.ComandoSolicitudExterna{MensajeID: mensajeID, SolicitudID: id, OrganizacionID: "org-1",
		Datos: dominio.DatosSolicitud{
			ReferenciaExterna:     &ref,
			Cosa:                  dominio.Cosa{Descripcion: "Cajas", Cantidad: 20, PesoKg: &peso},
			Desde:                 dominio.Ubicacion{Nombre: "Sucursal Centro", Direccion: "Av. Colón 1234"},
			Hasta:                 dominio.Ubicacion{Nombre: "Aeropuerto", Direccion: "Av. La Voz del Interior 8500"},
			HoraLlegadaSolicitada: hora,
			ToleranciaMinutos:     120,
		}}
}

func TestMySQL(t *testing.T) {
	if testing.Short() {
		t.Skip("test de integración con contenedores")
	}
	uow, _ := baseDePrueba(t)
	ctx := context.Background()
	reloj := ahora
	clima := &climaFijo{o: dominio.Operabilidad{Operable: false, Motivos: []string{"HIGH_WIND"}, ObservadoEn: ahora}}
	casos := aplicacion.NuevasSolicitudes(uow, clima, "SACO", func() time.Time { return reloj }, "test")

	hora := ahora.Add(3 * time.Hour)
	t.Run("registrar es idempotente", func(t *testing.T) {
		for range 2 {
			if err := casos.RegistrarExterna(ctx, comando("msg-1", "sol-1", &hora)); err != nil {
				t.Fatal(err)
			}
		}
		if err := casos.RegistrarExterna(ctx, comando("msg-otro", "sol-1", &hora)); err != nil {
			t.Fatal(err)
		}
		p, err := casos.Listar(ctx, "", 1, 20)
		if err != nil || p.Total != 1 {
			t.Fatalf("total = %d, err = %v", p.Total, err)
		}
		v := p.Resultados[0]
		if v.Estado != "RECIBIDA" || *v.Cosa.PesoKg != 350 || *v.ReferenciaExterna != "TRF-2026-00123" ||
			*v.HoraLlegadaSolicitada != "2026-10-15T12:00:00-03:00" || v.ToleranciaMinutos != 120 {
			t.Fatalf("vista inesperada: %+v", v)
		}
	})

	t.Run("aceptar al aeropuerto guarda traslado y alerta", func(t *testing.T) {
		destino := dominio.ExtremoDestino
		v, err := casos.Aceptar(ctx, "", "sol-1", &destino)
		if err != nil {
			t.Fatal(err)
		}
		leida, err := casos.Obtener(ctx, "org-1", "sol-1")
		if err != nil {
			t.Fatal(err)
		}
		a, b := mustJSON(t, v), mustJSON(t, leida)
		if a != b {
			t.Fatalf("lo leído no coincide con lo guardado:\n%s\n%s", a, b)
		}
		if leida.Version != 3 || leida.Traslado.AlertaClimatica.Estado != "NO_OPERABLE" ||
			leida.Traslado.AlertaClimatica.Motivos[0] != "HIGH_WIND" {
			t.Fatalf("vista inesperada: %+v", leida.Traslado)
		}
	})

	t.Run("cancelación tardía queda pendiente y se resuelve", func(t *testing.T) {
		idem := &puertos.Idempotencia{Endpoint: "cancelacion", Clave: "clave-0001", Hash: "abc"}
		v, err := casos.Cancelar(ctx, "org-1", "sol-1", "cambio de planes", idem)
		if err != nil || v.Cancelacion.Estado != "PENDIENTE_APROBACION" {
			t.Fatalf("v = %+v, err = %v", v.Cancelacion, err)
		}
		repetida, err := casos.Cancelar(ctx, "org-1", "sol-1", "cambio de planes", idem)
		if err != nil || mustJSON(t, repetida) != mustJSON(t, v) {
			t.Fatalf("la misma clave tiene que dar la misma respuesta: %v", err)
		}
		idem.Hash = "otro"
		if _, err := casos.Cancelar(ctx, "org-1", "sol-1", "otro motivo", idem); !errors.Is(err, puertos.ErrClaveReutilizada) {
			t.Fatalf("error = %v", err)
		}
		v, err = casos.ResolverCancelacionTardia(ctx, "sol-1", false)
		if err != nil || v.Cancelacion.Estado != "RECHAZADA" || v.Traslado.Estado != "PROGRAMADO" {
			t.Fatalf("v = %+v, err = %v", v, err)
		}
	})

	t.Run("organización ajena", func(t *testing.T) {
		if _, err := casos.Obtener(ctx, "org-otra", "sol-1"); !errors.Is(err, puertos.ErrNoEncontrada) {
			t.Fatalf("error = %v", err)
		}
		if _, err := casos.Obtener(ctx, "", "sol-no-existe"); !errors.Is(err, puertos.ErrNoEncontrada) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("vencimiento de contrapropuestas", func(t *testing.T) {
		if err := casos.RegistrarExterna(ctx, comando("msg-2", "sol-2", nil)); err != nil {
			t.Fatal(err)
		}
		if _, err := casos.Contraproponer(ctx, "sol-2", ahora.Add(30*time.Hour)); err != nil {
			t.Fatal(err)
		}
		if n, err := casos.VencerContrapropuestas(ctx); err != nil || n != 0 {
			t.Fatalf("n = %d, err = %v", n, err)
		}
		reloj = ahora.Add(24 * time.Hour)
		if n, err := casos.VencerContrapropuestas(ctx); err != nil || n != 1 {
			t.Fatalf("n = %d, err = %v", n, err)
		}
		v, _ := casos.Obtener(ctx, "", "sol-2")
		if v.Estado != "RECHAZADA_POR_CLIENTE" {
			t.Fatalf("estado = %s", v.Estado)
		}
	})

	t.Run("relay: publica en orden, corta ante un error y no repite", func(t *testing.T) {
		pendientes := contarPendientes(t, uow)
		if pendientes < 5 {
			t.Fatalf("pendientes = %d", pendientes)
		}
		pub := &publicadorFalso{fallarEn: 3}
		relay, err := NuevoRelay(uow.db, pub)
		if err != nil {
			t.Fatal(err)
		}
		n, err := relay.PublicarPendientes(ctx)
		if err == nil || n != 2 {
			t.Fatalf("n = %d, err = %v: tenía que cortar en el tercero", n, err)
		}
		if quedan := contarPendientes(t, uow); quedan != pendientes-2 {
			t.Fatalf("quedan %d, se esperaban %d", quedan, pendientes-2)
		}
		pub.fallarEn = 0
		if _, err := relay.PublicarPendientes(ctx); err != nil {
			t.Fatal(err)
		}
		if quedan := contarPendientes(t, uow); quedan != 0 {
			t.Fatalf("quedan %d pendientes", quedan)
		}
		if len(pub.recibidos) != pendientes {
			t.Fatalf("publicados = %d, se esperaban %d", len(pub.recibidos), pendientes)
		}
		if pub.recibidos[0].Tipo != string(dominio.EventoRecibida) || pub.recibidos[1].Tipo != string(dominio.EventoAceptada) ||
			pub.recibidos[2].Tipo != string(dominio.EventoAlertaClimatica) {
			t.Fatalf("orden inesperado: %s, %s, %s", pub.recibidos[0].Tipo, pub.recibidos[1].Tipo, pub.recibidos[2].Tipo)
		}
		if n, _ := relay.PublicarPendientes(ctx); n != 0 {
			t.Fatalf("no tenía que volver a publicar: %d", n)
		}
	})
}

func TestTransaccionSeDeshace(t *testing.T) {
	if testing.Short() {
		t.Skip("test de integración con contenedores")
	}
	uow, _ := baseDePrueba(t)
	ctx := context.Background()
	casos := aplicacion.NuevasSolicitudes(uow, climaFijo{}, "SACO", func() time.Time { return ahora }, "test")
	if err := casos.RegistrarExterna(ctx, comando("msg-1", "sol-1", nil)); err != nil {
		t.Fatal(err)
	}
	// Sin hora pedida no se acepta: ni la solicitud ni el outbox cambian.
	if _, err := casos.Aceptar(ctx, "", "sol-1", nil); !errors.Is(err, dominio.ErrSinHoraPedida) {
		t.Fatalf("error = %v", err)
	}
	if n := contarPendientes(t, uow); n != 1 {
		t.Fatalf("outbox = %d, se esperaba 1", n)
	}
	// Un comando con datos inválidos tampoco deja la marca de mensaje procesado.
	cmd := comando("msg-mal", "sol-mal", nil)
	cmd.Datos.Cosa.Descripcion = ""
	if err := casos.RegistrarExterna(ctx, cmd); !mensajeria.EsPermanente(err) {
		t.Fatalf("error = %v", err)
	}
	var procesados int
	if err := uow.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM mensajes_procesados WHERE mensaje_id = 'msg-mal'`).Scan(&procesados); err != nil || procesados != 0 {
		t.Fatalf("procesados = %d, err = %v", procesados, err)
	}
}

func contarPendientes(t *testing.T, uow *UnidadDeTrabajo) int {
	t.Helper()
	var n int
	if err := uow.db.QueryRow(`SELECT COUNT(*) FROM outbox WHERE publicado_en IS NULL`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
