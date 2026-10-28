package observabilidad

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"
)

// escritorFalso guarda los lotes que recibe; puede fallar o quedarse trabado.
type escritorFalso struct {
	mu     sync.Mutex
	lotes  [][]filaLog
	err    error
	trabar chan struct{} // si no es nil, Insertar espera a que se cierre
}

func (e *escritorFalso) Insertar(_ context.Context, filas []filaLog) error {
	if e.trabar != nil {
		<-e.trabar
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.lotes = append(e.lotes, append([]filaLog(nil), filas...))
	return e.err
}

func (e *escritorFalso) filas() []filaLog {
	e.mu.Lock()
	defer e.mu.Unlock()
	var todas []filaLog
	for _, l := range e.lotes {
		todas = append(todas, l...)
	}
	return todas
}

var sinAvisos = slog.New(slog.NewTextHandler(io.Discard, nil))

// nuevoLogger arma un logger como el de Iniciar (contexto + MySQL) sobre un escritor falso y
// devuelve la función que vacía la cola.
func nuevoLogger(e escritorLogs) (*slog.Logger, *destinoLogs, func()) {
	d := nuevoDestinoLogs(e, sinAvisos)
	ctx, cancelar := context.WithCancel(context.Background())
	go d.ejecutar(ctx)
	log := slog.New(manejadorContexto{manejadorMySQL{destino: d, servicio: "operaciones", instancia: "operaciones-1"}})
	return log, d, func() { cancelar(); <-d.terminado }
}

func TestLogsMySQLArmaLaFila(t *testing.T) {
	e := &escritorFalso{}
	log, _, vaciar := nuevoLogger(e)
	ctx := ConCorrelacion(context.Background(), "corr-1")
	log.With(slog.String("cola", "integracion.estados")).WithGroup("http").
		ErrorContext(ctx, "falló la reserva", slog.Int("estado", 503), slog.Any("error", errors.New("timeout")))
	log.Debug("no se guarda")
	log.Info(strings.Repeat("x", largoMaxMensaje+50))
	vaciar()

	filas := e.filas()
	if len(filas) != 2 {
		t.Fatalf("filas = %d, se esperaban 2 (DEBUG no se guarda)", len(filas))
	}
	f := filas[0]
	if f.Nivel != "ERROR" || f.Servicio != "operaciones" || f.Instancia != "operaciones-1" ||
		f.Mensaje != "falló la reserva" || f.CorrelationID != "corr-1" || f.CreadoEn.IsZero() {
		t.Fatalf("fila inesperada: %+v", f)
	}
	var atributos map[string]any
	if err := json.Unmarshal(f.Atributos, &atributos); err != nil {
		t.Fatal(err)
	}
	if atributos["cola"] != "integracion.estados" || atributos["http.estado"] != float64(503) || atributos["http.error"] != "timeout" {
		t.Fatalf("atributos = %v", atributos)
	}
	if len([]rune(filas[1].Mensaje)) != largoMaxMensaje || filas[1].Atributos != nil {
		t.Fatalf("el mensaje se recorta a %d caracteres: %d", largoMaxMensaje, len([]rune(filas[1].Mensaje)))
	}
}

func TestLogsMySQLEnLotes(t *testing.T) {
	e := &escritorFalso{}
	log, _, vaciar := nuevoLogger(e)
	for range TamanioLoteLogs + 10 {
		log.Info("evento")
	}
	vaciar()
	if n := len(e.filas()); n != TamanioLoteLogs+10 {
		t.Fatalf("se insertaron %d filas", n)
	}
	for _, lote := range e.lotes {
		if len(lote) > TamanioLoteLogs {
			t.Fatalf("lote de %d filas: el máximo es %d", len(lote), TamanioLoteLogs)
		}
	}
}

func TestLogsMySQLNoFrenaSiMySQLEstaTrabado(t *testing.T) {
	e := &escritorFalso{trabar: make(chan struct{})}
	log, d, vaciar := nuevoLogger(e)
	inicio := time.Now()
	for range TamanioColaLogs + TamanioLoteLogs + 50 {
		log.Info("evento")
	}
	if demora := time.Since(inicio); demora > time.Second {
		t.Fatalf("loguear esperó a MySQL: %v", demora)
	}
	if d.total.Load() == 0 {
		t.Fatal("con la cola llena los logs se tienen que descartar (y contar)")
	}
	close(e.trabar)
	vaciar()
}

func TestLogsMySQLCuentaLosLotesQueFallan(t *testing.T) {
	e := &escritorFalso{err: errors.New("MySQL caído")}
	log, d, vaciar := nuevoLogger(e)
	log.Info("uno")
	log.Info("dos")
	vaciar()
	if d.total.Load() != 2 {
		t.Fatalf("descartados = %d, se esperaban 2", d.total.Load())
	}
}

// TestLogsMySQLContraMySQLReal usa el esquema de infra/mysql-logs: si el código y la tabla no
// coinciden (columnas, tipos, largo del mensaje, JSON), falla acá y no en la demo.
func TestLogsMySQLContraMySQLReal(t *testing.T) {
	if testing.Short() {
		t.Skip("test de integración con contenedores")
	}
	ctx := context.Background()
	esquema, _ := filepath.Abs("../../../infra/mysql-logs/01-esquema.sql")
	contenedor, err := tcmysql.Run(ctx, "mysql:8.4", tcmysql.WithDatabase("observabilidad"),
		tcmysql.WithUsername("rumbo_logs"), tcmysql.WithPassword("prueba"), tcmysql.WithScripts(esquema))
	if err != nil {
		t.Fatalf("levantar MySQL (¿Docker está corriendo?): %v", err)
	}
	t.Cleanup(func() { _ = contenedor.Terminate(context.Background()) })
	dsn, err := contenedor.ConnectionString(ctx)
	if err != nil {
		t.Fatal(err)
	}

	manejador, apagar, err := iniciarLogsMySQL(dsn, "integracion", "integracion-2", sinAvisos)
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(manejadorContexto{manejador})
	log.WarnContext(ConCorrelacion(ctx, "corr-real"), "Operaciones tardó", slog.Duration("demora", 1500*time.Millisecond))
	log.Info("sin correlación")
	if err := apagar(ctx); err != nil {
		t.Fatal(err)
	}

	db, err := sql.Open("mysql", dsn+"?parseTime=true")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var nivel, servicio, instancia, mensaje, atributos string
	var correlacion sql.NullString
	var creado time.Time
	err = db.QueryRow(`SELECT nivel, servicio, instancia, mensaje, correlation_id, atributos, creado_en
		FROM logs ORDER BY id LIMIT 1`).Scan(&nivel, &servicio, &instancia, &mensaje, &correlacion, &atributos, &creado)
	if err != nil {
		t.Fatal(err)
	}
	if nivel != "WARN" || servicio != "integracion" || instancia != "integracion-2" || mensaje != "Operaciones tardó" ||
		correlacion.String != "corr-real" || !strings.Contains(atributos, `"demora": "1.5s"`) || time.Since(creado) > time.Minute {
		t.Fatalf("fila guardada: %s %s %s %q %v %s %v", nivel, servicio, instancia, mensaje, correlacion, atributos, creado)
	}
	var total int
	if err := db.QueryRow(`SELECT COUNT(*) FROM logs WHERE correlation_id IS NULL`).Scan(&total); err != nil || total != 1 {
		t.Fatalf("logs sin correlación = %d (%v)", total, err)
	}
}
