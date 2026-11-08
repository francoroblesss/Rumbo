package mysql

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

// calendario: franjas de 2 h entre las 8 y las 20, con lugar para 3 traslados y 1000 kg.
var calendario = dominio.Calendario{AperturaHora: 8, CierreHora: 20, DuracionHoras: 2, CupoTraslados: 3, CupoKg: 1000,
	Anticipacion: 2 * time.Hour, Horizonte: 30 * 24 * time.Hour, Zona: aplicacion.ZonaRumbo()}

// palabras son las de AEROPUERTO_PALABRAS por defecto.
var palabras = []string{"aeropuerto", "saco", "pajas blancas"}

// franja16 es la franja del 16/10 de 10 a 12 (hora de Córdoba).
var franja16 = time.Date(2026, 10, 16, 13, 0, 0, 0, time.UTC)

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

func datos(hora *time.Time) dominio.DatosSolicitud {
	peso := 350.0
	ref := "TRF-2026-00123"
	return dominio.DatosSolicitud{
		ReferenciaExterna:     &ref,
		Cosa:                  dominio.Cosa{Descripcion: "Cajas", Cantidad: 20, PesoKg: &peso},
		Desde:                 dominio.Ubicacion{Nombre: "Sucursal Centro", Direccion: "Av. Colón 1234"},
		Hasta:                 dominio.Ubicacion{Nombre: "Aeropuerto", Direccion: "Av. La Voz del Interior 8500"},
		HoraLlegadaSolicitada: hora,
		ToleranciaMinutos:     120,
	}
}

func idem(clave string) *puertos.Idempotencia {
	return &puertos.Idempotencia{Endpoint: "reserva", Clave: clave, Hash: "h"}
}

func TestMySQL(t *testing.T) {
	if testing.Short() {
		t.Skip("test de integración con contenedores")
	}
	uow, _ := baseDePrueba(t)
	ctx := context.Background()
	clima := &climaFijo{o: dominio.Operabilidad{Operable: false, Motivos: []string{"HIGH_WIND"}, ObservadoEn: ahora}}
	reloj := ahora
	casos := aplicacion.NuevasSolicitudes(uow, clima, "SACO", palabras, calendario, func() time.Time { return reloj }, "test")

	t.Run("el listado por fecha tiene el índice de la migración 0002", func(t *testing.T) {
		var n int
		err := uow.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.statistics
			WHERE table_schema = DATABASE() AND table_name = 'traslados' AND index_name = 'idx_franja_inicio'`).Scan(&n)
		if err != nil || n != 1 {
			t.Fatalf("índice idx_franja_inicio: %d, err = %v", n, err)
		}
	})

	// franjaHoy: hoy (15/10) de 12 a 14; termina dentro de 5 h, en la ventana del clima.
	franjaHoy := time.Date(2026, 10, 15, 15, 0, 0, 0, time.UTC)
	franja17 := franja16.AddDate(0, 0, 1)
	reservar := func(t *testing.T, org string, inicio time.Time, clave string) puertos.Vista {
		t.Helper()
		reloj = reloj.Add(time.Second) // cada una con su creado_en, para el orden del listado
		v, err := casos.Reservar(ctx, org, datos(nil), inicio, idem(clave))
		if err != nil {
			t.Fatalf("reservar: %v", err)
		}
		return v
	}
	alAeropuerto := reservar(t, "org-1", franjaHoy, "clave-hoy")
	b := reservar(t, "org-1", franja16, "clave-16a")
	c := reservar(t, "org-2", franja16, "clave-16b")
	d := reservar(t, "org-1", franja17, "clave-17")

	t.Run("se guarda y se lee la reserva con aeropuerto_extremo", func(t *testing.T) {
		var extremo string
		if err := uow.db.QueryRowContext(ctx, `SELECT aeropuerto_extremo FROM traslados WHERE solicitud_id = ?`,
			alAeropuerto.ID).Scan(&extremo); err != nil || extremo != "DESTINO" {
			t.Fatalf("aeropuerto_extremo = %q, err = %v", extremo, err)
		}
		leida, err := casos.Obtener(ctx, "org-1", alAeropuerto.ID)
		if err != nil || mustJSON(t, leida) != mustJSON(t, alAeropuerto) {
			t.Fatalf("lo leído no coincide con lo guardado (err = %v):\n%s\n%s", err, mustJSON(t, leida), mustJSON(t, alAeropuerto))
		}
		if *leida.Cosa.PesoKg != 350 || *leida.ReferenciaExterna != "TRF-2026-00123" || leida.HoraLlegadaSolicitada != nil ||
			leida.PropuestaDeHorario != nil || leida.MotivoRechazo != nil {
			t.Fatalf("vista inesperada: %+v", leida)
		}
	})

	t.Run("clima después de reservar: la alerta se guarda una sola vez", func(t *testing.T) {
		for range 2 {
			if err := casos.RevisarClimaDeReserva(ctx, alAeropuerto.ID); err != nil {
				t.Fatal(err)
			}
		}
		leida, _ := casos.Obtener(ctx, "", alAeropuerto.ID)
		if a := leida.Traslado.AlertaClimatica; a == nil || a.Estado != "NO_OPERABLE" || a.Motivos[0] != "HIGH_WIND" || leida.Version != 2 {
			t.Fatalf("vista inesperada: %+v", leida.Traslado)
		}
	})

	t.Run("avance de estado y finalizado_en", func(t *testing.T) {
		if _, err := casos.AvanzarEstado(ctx, "", alAeropuerto.ID, dominio.TrasladoEnCurso); err != nil {
			t.Fatal(err)
		}
		v, err := casos.AvanzarEstado(ctx, "", alAeropuerto.ID, dominio.TrasladoCompletado)
		if err != nil {
			t.Fatal(err)
		}
		leida, _ := casos.Obtener(ctx, "", alAeropuerto.ID)
		if mustJSON(t, leida) != mustJSON(t, v) || leida.Traslado.Estado != "COMPLETADO" || leida.Traslado.FinalizadoEn == nil || leida.Version != 4 {
			t.Fatalf("vista inesperada: %+v", leida.Traslado)
		}
	})

	t.Run("listado por fecha y organización con paginación", func(t *testing.T) {
		ids := func(p puertos.Pagina) string {
			var r []string
			for _, v := range p.Resultados {
				r = append(r, v.ID)
			}
			return fmt.Sprint(r)
		}
		casosListado := []struct {
			nombre string
			filtro puertos.FiltroListado
			total  int
			ids    []string
		}{
			{"todas, por franja y creación", puertos.FiltroListado{Pagina: 1, Tamanio: 20}, 4, []string{alAeropuerto.ID, b.ID, c.ID, d.ID}},
			{"del 16/10", puertos.FiltroListado{Fecha: "2026-10-16", Pagina: 1, Tamanio: 20}, 2, []string{b.ID, c.ID}},
			{"del 16/10 de org-1", puertos.FiltroListado{Organizacion: "org-1", Fecha: "2026-10-16", Pagina: 1, Tamanio: 20}, 1, []string{b.ID}},
			{"org-1, segunda página de 2", puertos.FiltroListado{Organizacion: "org-1", Pagina: 2, Tamanio: 2}, 3, []string{d.ID}},
			{"completados", puertos.FiltroListado{EstadoTraslado: "COMPLETADO", Pagina: 1, Tamanio: 20}, 1, []string{alAeropuerto.ID}},
		}
		for _, cl := range casosListado {
			p, err := casos.Listar(ctx, cl.filtro)
			if err != nil || p.Total != cl.total || ids(p) != fmt.Sprint(cl.ids) {
				t.Fatalf("%s: total = %d, ids = %s, err = %v; se esperaba %d %v", cl.nombre, p.Total, ids(p), err, cl.total, cl.ids)
			}
		}
	})

	t.Run("cancelar desde la web devuelve el lugar y respeta la clave", func(t *testing.T) {
		clave := &puertos.Idempotencia{Endpoint: "cancelacion", Clave: "clave-0001", Hash: "abc"}
		v, err := casos.CancelarDesdeWeb(ctx, "", b.ID, "cambio de planes", clave)
		if err != nil || v.Cancelacion.Estado != "APROBADA" || v.Traslado.Estado != "CANCELADO" {
			t.Fatalf("v = %+v, err = %v", v.Cancelacion, err)
		}
		repetida, err := casos.CancelarDesdeWeb(ctx, "org-1", b.ID, "cambio de planes", clave)
		if err != nil || mustJSON(t, repetida) != mustJSON(t, v) {
			t.Fatalf("la misma clave tiene que dar la misma respuesta: %v", err)
		}
		clave.Hash = "otro"
		if _, err := casos.CancelarDesdeWeb(ctx, "org-1", b.ID, "otro motivo", clave); !errors.Is(err, puertos.ErrClaveReutilizada) {
			t.Fatalf("error = %v", err)
		}
		f, err := casos.Franjas(ctx, "2026-10-16")
		if err != nil || f.Franjas[1].Inicio != "2026-10-16T10:00:00-03:00" || f.Franjas[1].TrasladosReservados != 1 || f.Franjas[0].TrasladosReservados != 0 {
			t.Fatalf("franjas = %+v, err = %v", f, err)
		}
	})

	t.Run("organización ajena", func(t *testing.T) {
		if _, err := casos.Obtener(ctx, "org-otra", c.ID); !errors.Is(err, puertos.ErrNoEncontrada) {
			t.Fatalf("error = %v", err)
		}
		if _, err := casos.Obtener(ctx, "", "sol-no-existe"); !errors.Is(err, puertos.ErrNoEncontrada) {
			t.Fatalf("error = %v", err)
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
		// Las 4 reservas, después la alerta de la primera y sus dos avances.
		esperados := []dominio.Evento{dominio.EventoAceptada, dominio.EventoAceptada, dominio.EventoAceptada, dominio.EventoAceptada,
			dominio.EventoAlertaClimatica, dominio.EventoTrasladoEstado, dominio.EventoTrasladoEstado, dominio.EventoCancelacionRegistrada}
		for i, tipo := range esperados {
			if pub.recibidos[i].Tipo != string(tipo) {
				t.Fatalf("mensaje %d: %s, se esperaba %s", i, pub.recibidos[i].Tipo, tipo)
			}
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
	casos := aplicacion.NuevasSolicitudes(uow, climaFijo{}, "SACO", palabras, calendario, func() time.Time { return ahora }, "test")
	// Una reserva con datos inválidos falla después de crear y bloquear la franja: el rollback
	// tiene que deshacer también la franja (no queda ni la fila ni un lugar tomado).
	mal := datos(nil)
	mal.Cosa.Descripcion = ""
	if _, err := casos.Reservar(ctx, "org-1", mal, franja16, idem("clave-mal")); !errors.Is(err, dominio.ErrDatosInvalidos) {
		t.Fatalf("error = %v", err)
	}
	var franjas int
	if err := uow.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM franjas`).Scan(&franjas); err != nil || franjas != 0 {
		t.Fatalf("franjas = %d, err = %v", franjas, err)
	}
}

// TestReservasConcurrentes es la evidencia de D4 (ADR-022): muchas reservas a la vez sobre una
// franja con lugar para 3. El bloqueo de la fila (SELECT … FOR UPDATE) las ordena: entran
// exactamente 3, el resto recibe SIN_DISPONIBILIDAD y ninguna termina en deadlock ni en otro error.
func TestReservasConcurrentes(t *testing.T) {
	if testing.Short() {
		t.Skip("test de integración con contenedores")
	}
	uow, _ := baseDePrueba(t)
	ctx := context.Background()
	casos := aplicacion.NuevasSolicitudes(uow, climaFijo{}, "SACO", palabras, calendario, func() time.Time { return ahora }, "test")

	// 300 kg cada una: por peso entrarían 3 (900 de 1000 kg), así que el límite es el de traslados.
	carga := datos(nil)
	peso := 300.0
	carga.Cosa.PesoKg = &peso
	const pedidos = 20
	var wg sync.WaitGroup
	errores := make([]error, pedidos)
	for i := range pedidos {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errores[i] = casos.Reservar(ctx, "org-1", carga, franja16, idem(fmt.Sprintf("clave-%02d", i)))
		}()
	}
	wg.Wait()
	ok, sinLugar := 0, 0
	for _, err := range errores {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, dominio.ErrSinDisponibilidad):
			sinLugar++
		default:
			t.Fatalf("error inesperado: %v", err)
		}
	}
	if ok != 3 || sinLugar != pedidos-3 {
		t.Fatalf("reservas = %d, sin lugar = %d; se esperaban 3 y %d", ok, sinLugar, pedidos-3)
	}
	var reservados, traslados int
	var kg float64
	if err := uow.db.QueryRowContext(ctx, `SELECT traslados_reservados, kg_reservados FROM franjas`).Scan(&reservados, &kg); err != nil {
		t.Fatal(err)
	}
	if err := uow.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM traslados WHERE franja_inicio IS NOT NULL`).Scan(&traslados); err != nil {
		t.Fatal(err)
	}
	if reservados != 3 || kg != 900 || traslados != 3 {
		t.Fatalf("franja: %d traslados y %v kg; traslados guardados: %d", reservados, kg, traslados)
	}

	t.Run("el mismo pedido repetido a la vez no reserva dos veces", func(t *testing.T) {
		otra := time.Date(2026, 10, 16, 15, 0, 0, 0, time.UTC) // de 12 a 14
		var wg sync.WaitGroup
		ids := make([]string, 5)
		for i := range ids {
			wg.Add(1)
			go func() {
				defer wg.Done()
				// Un reintento que choca con el original en la misma clave puede fallar (y el
				// consumidor vuelve a reintentar); lo que no puede pasar es que reserve dos veces.
				if v, err := casos.Reservar(ctx, "org-1", datos(nil), otra, idem("clave-repetida")); err == nil {
					ids[i] = v.ID
				}
			}()
		}
		wg.Wait()
		var n int
		if err := uow.db.QueryRowContext(ctx, `SELECT traslados_reservados FROM franjas WHERE inicio = ?`, otra).Scan(&n); err != nil || n != 1 {
			t.Fatalf("reservados = %d, err = %v", n, err)
		}
		primero := ""
		for _, id := range ids {
			if id != "" && primero != "" && id != primero {
				t.Fatalf("dos reservas distintas para la misma clave: %v", ids)
			}
			if id != "" {
				primero = id
			}
		}
	})

	t.Run("cancelar tarde devuelve el lugar y queda marcada", func(t *testing.T) {
		hoy := time.Date(2026, 10, 15, 17, 0, 0, 0, time.UTC) // de 14 a 16
		v, err := casos.Reservar(ctx, "org-2", datos(nil), hoy, idem("clave-hoy"))
		if err != nil {
			t.Fatal(err)
		}
		c, err := casos.Cancelar(ctx, "org-2", v.ID, "ya no hace falta", nil)
		if err != nil || !c.Cancelacion.Tardia || c.Cancelacion.Estado != "APROBADA" || c.Traslado.Estado != "CANCELADO" {
			t.Fatalf("c = %+v, err = %v", c.Cancelacion, err)
		}
		leida, _ := casos.Obtener(ctx, "org-2", v.ID)
		if mustJSON(t, leida) != mustJSON(t, c) || leida.Traslado.Franja == nil {
			t.Fatalf("lo leído no coincide con lo guardado:\n%s\n%s", mustJSON(t, leida), mustJSON(t, c))
		}
		d, err := casos.Disponibilidad(ctx, "2026-10-15", 0)
		if err != nil || len(d.Franjas) == 0 || d.Franjas[1].Inicio != "2026-10-15T14:00:00-03:00" || d.Franjas[1].TrasladosDisponibles != 3 {
			t.Fatalf("disponibilidad = %+v, err = %v", d, err)
		}
	})
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
