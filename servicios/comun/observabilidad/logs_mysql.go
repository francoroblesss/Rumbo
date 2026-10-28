package observabilidad

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	// Driver de MySQL para database/sql, el mismo que usan Operaciones y Recursos.
	_ "github.com/go-sql-driver/mysql"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Envío de logs a la tabla logs de la base observabilidad (ADR-023). Grafana los lee de ahí con
// un usuario de solo lectura, como en la clase práctica 8. A diferencia de la demo de esa clase
// (un INSERT por log, esperando a MySQL), acá el log se encola y una goroutine inserta en lotes:
// un MySQL de logs lento o caído no puede frenar una request (teórico 7). Si la cola se llena o
// MySQL falla, el log se descarta y se cuenta en rumbo_logs_descartados_total; igual queda en
// stdout (docker compose logs).
const (
	TamanioColaLogs  = 1000            // logs esperando a ser insertados
	TamanioLoteLogs  = 100             // filas por INSERT
	IntervaloLogs    = time.Second     // cada cuánto se inserta lo acumulado
	TimeoutLoteLogs  = 2 * time.Second // tope de cada INSERT
	largoMaxMensaje  = 1000            // columna logs.mensaje
	intervaloAvisos  = time.Minute     // como mucho un aviso por minuto de que se descartan logs
	conexionesMaxima = 2               // el MySQL de logs no puede quedarse sin conexiones (ADR-023)
)

// filaLog es una fila de la tabla logs.
type filaLog struct {
	CreadoEn      time.Time
	Nivel         string
	Servicio      string
	Instancia     string
	Mensaje       string
	CorrelationID string
	TraceID       string
	Atributos     []byte // JSON con el resto de los atributos del log
}

// escritorLogs inserta un lote de filas. Lo implementa MySQL; en los tests, un doble en memoria.
type escritorLogs interface {
	Insertar(ctx context.Context, filas []filaLog) error
}

// destinoLogs es la cola compartida por todas las copias del manejador (WithAttrs y WithGroup
// devuelven copias que escriben en la misma cola).
type destinoLogs struct {
	filas       chan filaLog
	escritor    escritorLogs
	avisos      *slog.Logger // stdout: los avisos del propio envío no pueden volver a esta cola
	descartados metric.Int64Counter
	total       atomic.Int64 // descartados desde el arranque (para los tests)
	ultimoAviso atomic.Int64
	terminado   chan struct{}
}

func nuevoDestinoLogs(escritor escritorLogs, avisos *slog.Logger) *destinoLogs {
	descartados, _ := Medidor().Int64Counter("rumbo_logs_descartados_total",
		metric.WithDescription("Logs que no se pudieron guardar en MySQL, por motivo"))
	return &destinoLogs{filas: make(chan filaLog, TamanioColaLogs), escritor: escritor, avisos: avisos,
		descartados: descartados, terminado: make(chan struct{})}
}

// encolar nunca bloquea: si la cola está llena, descarta.
func (d *destinoLogs) encolar(f filaLog) {
	select {
	case d.filas <- f:
	default:
		d.descartar(1, "cola_llena", nil)
	}
}

func (d *destinoLogs) descartar(n int, motivo string, err error) {
	d.total.Add(int64(n))
	d.descartados.Add(context.Background(), int64(n), metric.WithAttributes(attribute.String("motivo", motivo)))
	ahora := time.Now().UnixNano()
	if ultimo := d.ultimoAviso.Load(); ahora-ultimo >= int64(intervaloAvisos) && d.ultimoAviso.CompareAndSwap(ultimo, ahora) {
		atributos := []any{slog.Int("cantidad", n), slog.String("motivo", motivo)}
		if err != nil {
			atributos = append(atributos, slog.String("error", err.Error()))
		}
		d.avisos.Warn("se descartaron logs que iban a MySQL", atributos...)
	}
}

// ejecutar junta las filas en lotes y los inserta hasta que se cancele ctx; antes de terminar,
// inserta lo que quedó en la cola.
func (d *destinoLogs) ejecutar(ctx context.Context) {
	defer close(d.terminado)
	ticker := time.NewTicker(IntervaloLogs)
	defer ticker.Stop()
	lote := make([]filaLog, 0, TamanioLoteLogs)
	enviar := func() {
		if len(lote) == 0 {
			return
		}
		ctxLote, cancelar := context.WithTimeout(context.Background(), TimeoutLoteLogs)
		if err := d.escritor.Insertar(ctxLote, lote); err != nil {
			d.descartar(len(lote), "mysql", err)
		}
		cancelar()
		lote = lote[:0]
	}
	for {
		select {
		case f := <-d.filas:
			if lote = append(lote, f); len(lote) == TamanioLoteLogs {
				enviar()
			}
		case <-ticker.C:
			enviar()
		case <-ctx.Done():
			for {
				select {
				case f := <-d.filas:
					if lote = append(lote, f); len(lote) == TamanioLoteLogs {
						enviar()
					}
				default:
					enviar()
					return
				}
			}
		}
	}
}

// manejadorMySQL es el slog.Handler que encola cada log de nivel INFO o mayor.
type manejadorMySQL struct {
	destino   *destinoLogs
	servicio  string
	instancia string
	atributos []slog.Attr // los de WithAttrs, ya con el prefijo de su grupo
	grupo     string      // prefijo de los atributos siguientes ("" o "grupo.")
}

func (m manejadorMySQL) Enabled(_ context.Context, nivel slog.Level) bool {
	return nivel >= slog.LevelInfo
}

func (m manejadorMySQL) Handle(_ context.Context, r slog.Record) error {
	f := filaLog{CreadoEn: r.Time.UTC(), Nivel: r.Level.String(), Servicio: m.servicio, Instancia: m.instancia,
		Mensaje: recortar(r.Message, largoMaxMensaje)}
	resto := map[string]any{}
	// correlation_id y trace_id los agrega manejadorContexto al registro: van a sus columnas sin
	// importar el grupo en el que se logueó.
	agregar := func(prefijo string, a slog.Attr) {
		switch v := a.Value.Resolve(); a.Key {
		case "correlation_id":
			f.CorrelationID = v.String()
		case "trace_id":
			f.TraceID = v.String()
		default:
			resto[prefijo+a.Key] = valor(v)
		}
	}
	for _, a := range m.atributos {
		agregar("", a)
	}
	r.Attrs(func(a slog.Attr) bool {
		agregar(m.grupo, a)
		return true
	})
	if len(resto) > 0 {
		f.Atributos, _ = json.Marshal(resto)
	}
	m.destino.encolar(f)
	return nil
}

func (m manejadorMySQL) WithAttrs(attrs []slog.Attr) slog.Handler {
	copia := m
	copia.atributos = append([]slog.Attr(nil), m.atributos...)
	for _, a := range attrs {
		copia.atributos = append(copia.atributos, slog.Attr{Key: m.grupo + a.Key, Value: a.Value})
	}
	return copia
}

func (m manejadorMySQL) WithGroup(nombre string) slog.Handler {
	if nombre == "" {
		return m
	}
	copia := m
	copia.grupo = m.grupo + nombre + "."
	return copia
}

// valor pasa un atributo de slog a algo que encoding/json sabe escribir.
func valor(v slog.Value) any {
	switch v.Kind() {
	case slog.KindGroup:
		grupo := map[string]any{}
		for _, a := range v.Group() {
			grupo[a.Key] = valor(a.Value.Resolve())
		}
		return grupo
	case slog.KindDuration:
		return v.Duration().String()
	case slog.KindTime:
		return v.Time().UTC().Format(time.RFC3339Nano)
	case slog.KindAny:
		if err, ok := v.Any().(error); ok {
			return err.Error()
		}
		return fmt.Sprint(v.Any())
	default:
		return v.Any()
	}
}

func recortar(s string, maximo int) string {
	if utf8.RuneCountInString(s) <= maximo {
		return s
	}
	return string([]rune(s)[:maximo])
}

// escritorMySQL inserta los lotes con un INSERT de varias filas.
type escritorMySQL struct{ db *sql.DB }

func (e escritorMySQL) Insertar(ctx context.Context, filas []filaLog) error {
	var sentencia strings.Builder
	sentencia.WriteString(`INSERT INTO logs (creado_en, nivel, servicio, instancia, mensaje, correlation_id, trace_id, atributos) VALUES `)
	args := make([]any, 0, 8*len(filas))
	for i, f := range filas {
		if i > 0 {
			sentencia.WriteString(", ")
		}
		sentencia.WriteString("(?, ?, ?, ?, ?, ?, ?, ?)")
		var atributos any
		if f.Atributos != nil {
			atributos = string(f.Atributos)
		}
		args = append(args, f.CreadoEn, f.Nivel, f.Servicio, f.Instancia, f.Mensaje, nulo(f.CorrelationID), nulo(f.TraceID), atributos)
	}
	_, err := e.db.ExecContext(ctx, sentencia.String(), args...)
	return err
}

func nulo(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// iniciarLogsMySQL abre la conexión (perezosa: no falla si MySQL todavía no está) y arranca el
// envío. Devuelve el manejador y la función que vacía la cola y cierra la conexión al apagar.
func iniciarLogsMySQL(dsn, servicio, instancia string, avisos *slog.Logger) (slog.Handler, func(context.Context) error, error) {
	if !strings.Contains(dsn, "parseTime") {
		separador := "?"
		if strings.Contains(dsn, "?") {
			separador = "&"
		}
		dsn += separador + "parseTime=true"
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("configurar la conexión al MySQL de logs: %w", err)
	}
	db.SetMaxOpenConns(conexionesMaxima)
	db.SetMaxIdleConns(conexionesMaxima)
	db.SetConnMaxLifetime(5 * time.Minute)

	destino := nuevoDestinoLogs(escritorMySQL{db: db}, avisos)
	ctx, detener := context.WithCancel(context.Background())
	go destino.ejecutar(ctx)
	var unaVez sync.Once
	apagar := func(ctx context.Context) error {
		unaVez.Do(detener)
		select {
		case <-destino.terminado:
		case <-ctx.Done():
		}
		return db.Close()
	}
	return manejadorMySQL{destino: destino, servicio: servicio, instancia: instancia}, apagar, nil
}
