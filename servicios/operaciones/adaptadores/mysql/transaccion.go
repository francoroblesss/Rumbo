package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"

	"github.com/francoroblesss/Rumbo/servicios/comun/mensajeria"
	"github.com/francoroblesss/Rumbo/servicios/operaciones/dominio"
	"github.com/francoroblesss/Rumbo/servicios/operaciones/puertos"
)

// transaccion implementa puertos.Transaccion con SQL explícito (ADR-005). La traducción entre
// el agregado (sin tags) y las filas vive acá: el dominio no sabe cómo se guarda.
type transaccion struct {
	tx *sql.Tx
}

var _ puertos.Transaccion = (*transaccion)(nil)

// Filas JSON de los objetos de valor (columnas cosa, origen y destino).
type cosaFila struct {
	Descripcion string   `json:"descripcion"`
	Cantidad    int      `json:"cantidad"`
	PesoKg      *float64 `json:"peso_kg"`
	VolumenM3   *float64 `json:"volumen_m3"`
}

type ubicacionFila struct {
	Nombre    string  `json:"nombre"`
	Direccion string  `json:"direccion"`
	Contacto  *string `json:"contacto"`
}

const columnas = `s.id, s.organizacion_id, s.origen_solicitud, s.referencia_externa, s.cosa, s.origen, s.destino,
	s.hora_llegada_solicitada, s.tolerancia_minutos, s.observaciones, s.estado, s.contrapropuesta_hora,
	s.contrapropuesta_vence, s.motivo_rechazo, s.cancelacion_estado, s.cancelacion_motivo,
	s.cancelacion_solicitada_en, s.version, s.creado_en, s.actualizado_en,
	t.id, t.estado, t.hora_llegada_comprometida, t.demora_minutos, t.finalizado_en, t.detalle_resultado,
	t.aeropuerto_extremo, t.alerta_estado, t.alerta_motivos, t.alerta_observado_en`

const desdeSolicitudes = ` FROM solicitudes s LEFT JOIN traslados t ON t.solicitud_id = s.id`

type escaner interface{ Scan(dest ...any) error }

// leerSolicitud arma el agregado desde una fila de solicitudes + traslados.
func leerSolicitud(fila escaner) (*dominio.Solicitud, error) {
	var (
		s                                     dominio.Solicitud
		origen, estado                        string
		cosa, desde, hasta                    []byte
		contraHora, contraVence               sql.NullTime
		cancEstado, cancMotivo                sql.NullString
		cancEn                                sql.NullTime
		tID, tEstado                          sql.NullString
		tHora, tFinalizado, tObservado        sql.NullTime
		tDemora                               sql.NullInt64
		tDetalle, tExtremo, tAlerta           sql.NullString
		tMotivos                              []byte
		referencia, observaciones, motivoRech sql.NullString
		horaPedida                            sql.NullTime
	)
	err := fila.Scan(&s.ID, &s.OrganizacionID, &origen, &referencia, &cosa, &desde, &hasta,
		&horaPedida, &s.ToleranciaMinutos, &observaciones, &estado, &contraHora,
		&contraVence, &motivoRech, &cancEstado, &cancMotivo,
		&cancEn, &s.Version, &s.CreadoEn, &s.ActualizadoEn,
		&tID, &tEstado, &tHora, &tDemora, &tFinalizado, &tDetalle,
		&tExtremo, &tAlerta, &tMotivos, &tObservado)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, puertos.ErrNoEncontrada
	}
	if err != nil {
		return nil, fmt.Errorf("leer solicitud: %w", err)
	}
	s.Origen = dominio.OrigenSolicitud(origen)
	s.Estado = dominio.EstadoSolicitud(estado)
	s.ReferenciaExterna = texto(referencia)
	s.Observaciones = texto(observaciones)
	s.MotivoRechazo = texto(motivoRech)
	s.HoraLlegadaSolicitada = hora(horaPedida)
	s.CreadoEn, s.ActualizadoEn = s.CreadoEn.UTC(), s.ActualizadoEn.UTC()

	var c cosaFila
	var o, d ubicacionFila
	if err := errors.Join(json.Unmarshal(cosa, &c), json.Unmarshal(desde, &o), json.Unmarshal(hasta, &d)); err != nil {
		return nil, fmt.Errorf("leer datos de la solicitud %s: %w", s.ID, err)
	}
	s.Cosa = dominio.Cosa{Descripcion: c.Descripcion, Cantidad: c.Cantidad, PesoKg: c.PesoKg, VolumenM3: c.VolumenM3}
	s.Desde = dominio.Ubicacion{Nombre: o.Nombre, Direccion: o.Direccion, Contacto: o.Contacto}
	s.Hasta = dominio.Ubicacion{Nombre: d.Nombre, Direccion: d.Direccion, Contacto: d.Contacto}

	if contraHora.Valid && contraVence.Valid {
		s.Contrapropuesta = &dominio.Contrapropuesta{Hora: contraHora.Time.UTC(), VenceEn: contraVence.Time.UTC()}
	}
	if cancEstado.Valid {
		s.Cancelacion = &dominio.Cancelacion{Estado: dominio.EstadoCancelacion(cancEstado.String),
			Motivo: cancMotivo.String, SolicitadaEn: cancEn.Time.UTC()}
	}
	if tID.Valid {
		t := &dominio.Traslado{
			ID:                      tID.String,
			Estado:                  dominio.EstadoTraslado(tEstado.String),
			HoraLlegadaComprometida: tHora.Time.UTC(),
			FinalizadoEn:            hora(tFinalizado),
			DetalleResultado:        texto(tDetalle),
		}
		if tDemora.Valid {
			demora := int(tDemora.Int64)
			t.DemoraMinutos = &demora
		}
		if tExtremo.Valid {
			extremo := dominio.Extremo(tExtremo.String)
			t.AeropuertoExtremo = &extremo
		}
		if tAlerta.Valid {
			alerta := &dominio.AlertaClimatica{Estado: dominio.EstadoAlerta(tAlerta.String), ObservadoEn: hora(tObservado)}
			if len(tMotivos) > 0 {
				if err := json.Unmarshal(tMotivos, &alerta.Motivos); err != nil {
					return nil, fmt.Errorf("leer motivos de la alerta de %s: %w", s.ID, err)
				}
			}
			t.Alerta = alerta
		}
		s.Traslado = t
	}
	return &s, nil
}

func (t *transaccion) ObtenerParaActualizar(ctx context.Context, id string) (*dominio.Solicitud, error) {
	// FOR UPDATE: bloquea la fila hasta el fin de la transacción, así dos operadores (o el
	// operador y el cliente) no aplican transiciones a la vez sobre la misma solicitud.
	return leerSolicitud(t.tx.QueryRowContext(ctx, `SELECT `+columnas+desdeSolicitudes+` WHERE s.id = ? FOR UPDATE`, id))
}

func (t *transaccion) Obtener(ctx context.Context, id string) (*dominio.Solicitud, error) {
	return leerSolicitud(t.tx.QueryRowContext(ctx, `SELECT `+columnas+desdeSolicitudes+` WHERE s.id = ?`, id))
}

func (t *transaccion) Listar(ctx context.Context, estado string, desde, limite int) ([]*dominio.Solicitud, int, error) {
	filtro, args := "", []any{}
	if estado != "" {
		filtro, args = " WHERE s.estado = ?", append(args, estado)
	}
	var total int
	if err := t.tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM solicitudes s`+filtro, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("contar solicitudes: %w", err)
	}
	filas, err := t.tx.QueryContext(ctx, `SELECT `+columnas+desdeSolicitudes+filtro+
		` ORDER BY s.actualizado_en DESC, s.id DESC LIMIT ? OFFSET ?`, append(args, limite, desde)...)
	if err != nil {
		return nil, 0, fmt.Errorf("listar solicitudes: %w", err)
	}
	defer filas.Close()
	var lista []*dominio.Solicitud
	for filas.Next() {
		s, err := leerSolicitud(filas)
		if err != nil {
			return nil, 0, err
		}
		lista = append(lista, s)
	}
	return lista, total, filas.Err()
}

func (t *transaccion) Existe(ctx context.Context, id string) (bool, error) {
	var existe bool
	if err := t.tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM solicitudes WHERE id = ?)`, id).Scan(&existe); err != nil {
		return false, fmt.Errorf("verificar solicitud: %w", err)
	}
	return existe, nil
}

// valores devuelve las columnas de la solicitud en el orden de las sentencias de abajo.
func valores(s *dominio.Solicitud) ([]any, error) {
	cosa, err1 := json.Marshal(cosaFila{Descripcion: s.Cosa.Descripcion, Cantidad: s.Cosa.Cantidad, PesoKg: s.Cosa.PesoKg, VolumenM3: s.Cosa.VolumenM3})
	desde, err2 := json.Marshal(ubicacionFila{Nombre: s.Desde.Nombre, Direccion: s.Desde.Direccion, Contacto: s.Desde.Contacto})
	hasta, err3 := json.Marshal(ubicacionFila{Nombre: s.Hasta.Nombre, Direccion: s.Hasta.Direccion, Contacto: s.Hasta.Contacto})
	if err := errors.Join(err1, err2, err3); err != nil {
		return nil, fmt.Errorf("serializar la solicitud %s: %w", s.ID, err)
	}
	var contraHora, contraVence, cancEn *time.Time
	var cancEstado, cancMotivo *string
	if c := s.Contrapropuesta; c != nil {
		contraHora, contraVence = &c.Hora, &c.VenceEn
	}
	if c := s.Cancelacion; c != nil {
		estado := string(c.Estado)
		cancEstado, cancMotivo, cancEn = &estado, &c.Motivo, &c.SolicitadaEn
	}
	return []any{s.OrganizacionID, string(s.Origen), s.ReferenciaExterna, string(cosa), string(desde), string(hasta),
		utc(s.HoraLlegadaSolicitada), s.ToleranciaMinutos, s.Observaciones, string(s.Estado), utc(contraHora),
		utc(contraVence), s.MotivoRechazo, cancEstado, cancMotivo, utc(cancEn), s.Version,
		s.CreadoEn.UTC(), s.ActualizadoEn.UTC(), s.ID}, nil
}

func (t *transaccion) Insertar(ctx context.Context, s *dominio.Solicitud) error {
	v, err := valores(s)
	if err != nil {
		return err
	}
	if _, err := t.tx.ExecContext(ctx, `INSERT INTO solicitudes (organizacion_id, origen_solicitud,
		referencia_externa, cosa, origen, destino, hora_llegada_solicitada, tolerancia_minutos, observaciones,
		estado, contrapropuesta_hora, contrapropuesta_vence, motivo_rechazo, cancelacion_estado,
		cancelacion_motivo, cancelacion_solicitada_en, version, creado_en, actualizado_en, id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, v...); err != nil {
		return fmt.Errorf("insertar solicitud: %w", err)
	}
	return t.guardarTraslado(ctx, s)
}

func (t *transaccion) Actualizar(ctx context.Context, s *dominio.Solicitud) error {
	v, err := valores(s)
	if err != nil {
		return err
	}
	if _, err := t.tx.ExecContext(ctx, `UPDATE solicitudes SET organizacion_id = ?, origen_solicitud = ?,
		referencia_externa = ?, cosa = ?, origen = ?, destino = ?, hora_llegada_solicitada = ?,
		tolerancia_minutos = ?, observaciones = ?, estado = ?, contrapropuesta_hora = ?,
		contrapropuesta_vence = ?, motivo_rechazo = ?, cancelacion_estado = ?, cancelacion_motivo = ?,
		cancelacion_solicitada_en = ?, version = ?, creado_en = ?, actualizado_en = ? WHERE id = ?`, v...); err != nil {
		return fmt.Errorf("actualizar solicitud: %w", err)
	}
	return t.guardarTraslado(ctx, s)
}

// guardarTraslado inserta o actualiza el traslado de la solicitud, si tiene.
func (t *transaccion) guardarTraslado(ctx context.Context, s *dominio.Solicitud) error {
	tr := s.Traslado
	if tr == nil {
		return nil
	}
	var extremo, alertaEstado *string
	var motivos *string
	var observado *time.Time
	if tr.AeropuertoExtremo != nil {
		e := string(*tr.AeropuertoExtremo)
		extremo = &e
	}
	if a := tr.Alerta; a != nil {
		e := string(a.Estado)
		alertaEstado, observado = &e, a.ObservadoEn
		lista := a.Motivos
		if lista == nil {
			lista = []string{}
		}
		crudo, err := json.Marshal(lista)
		if err != nil {
			return fmt.Errorf("serializar motivos: %w", err)
		}
		m := string(crudo)
		motivos = &m
	}
	_, err := t.tx.ExecContext(ctx, `INSERT INTO traslados (id, solicitud_id, estado, hora_llegada_comprometida,
		demora_minutos, finalizado_en, detalle_resultado, aeropuerto_extremo, alerta_estado, alerta_motivos,
		alerta_observado_en) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE estado = VALUES(estado), hora_llegada_comprometida = VALUES(hora_llegada_comprometida),
		demora_minutos = VALUES(demora_minutos), finalizado_en = VALUES(finalizado_en),
		detalle_resultado = VALUES(detalle_resultado), aeropuerto_extremo = VALUES(aeropuerto_extremo),
		alerta_estado = VALUES(alerta_estado), alerta_motivos = VALUES(alerta_motivos),
		alerta_observado_en = VALUES(alerta_observado_en)`,
		tr.ID, s.ID, string(tr.Estado), tr.HoraLlegadaComprometida.UTC(), tr.DemoraMinutos, utc(tr.FinalizadoEn),
		tr.DetalleResultado, extremo, alertaEstado, motivos, utc(observado))
	if err != nil {
		return fmt.Errorf("guardar traslado: %w", err)
	}
	return nil
}

func (t *transaccion) IDsContrapropuestasVencidas(ctx context.Context, ahora time.Time, limite int) ([]string, error) {
	// SKIP LOCKED: si otra transacción tiene tomada una solicitud (una respuesta del cliente en
	// curso u otra réplica), se saltea en vez de esperarla.
	filas, err := t.tx.QueryContext(ctx, `SELECT id FROM solicitudes
		WHERE estado = ? AND contrapropuesta_vence <= ?
		ORDER BY contrapropuesta_vence LIMIT ? FOR UPDATE SKIP LOCKED`,
		string(dominio.SolicitudContrapropuesta), ahora.UTC(), limite)
	if err != nil {
		return nil, fmt.Errorf("buscar contrapropuestas vencidas: %w", err)
	}
	defer filas.Close()
	var ids []string
	for filas.Next() {
		var id string
		if err := filas.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, filas.Err()
}

func (t *transaccion) AgregarAlOutbox(ctx context.Context, exchange, routingKey string, sobre mensajeria.Sobre) error {
	cuerpo, err := json.Marshal(sobre)
	if err != nil {
		return fmt.Errorf("serializar sobre: %w", err)
	}
	// El relay publica después y en otro contexto: se guarda la traza para continuarla
	// (docs/interfaces-internas.md §9).
	traza := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, traza)
	contexto, err := json.Marshal(traza)
	if err != nil {
		return fmt.Errorf("serializar contexto de traza: %w", err)
	}
	if _, err := t.tx.ExecContext(ctx, `INSERT INTO outbox (mensaje_id, exchange_nombre, routing_key, sobre,
		contexto_traza, creado_en) VALUES (?, ?, ?, ?, ?, ?)`,
		sobre.ID, exchange, routingKey, string(cuerpo), string(contexto), time.Now().UTC()); err != nil {
		return fmt.Errorf("agregar al outbox: %w", err)
	}
	return nil
}

func (t *transaccion) RegistrarMensaje(ctx context.Context, mensajeID string) (bool, error) {
	r, err := t.tx.ExecContext(ctx, `INSERT IGNORE INTO mensajes_procesados (mensaje_id, procesado_en) VALUES (?, ?)`,
		mensajeID, time.Now().UTC())
	if err != nil {
		return false, fmt.Errorf("registrar mensaje: %w", err)
	}
	n, err := r.RowsAffected()
	if err != nil {
		return false, err
	}
	return n == 0, nil
}

func (t *transaccion) BuscarIdempotencia(ctx context.Context, org, endpoint, clave string) (*puertos.RespuestaGuardada, error) {
	var r puertos.RespuestaGuardada
	err := t.tx.QueryRowContext(ctx, `SELECT hash_cuerpo, estado_http, respuesta FROM claves_idempotencia
		WHERE organizacion_id = ? AND endpoint = ? AND clave = ?`, org, endpoint, clave).Scan(&r.Hash, &r.EstadoHTTP, &r.Cuerpo)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("buscar clave de idempotencia: %w", err)
	}
	return &r, nil
}

func (t *transaccion) GuardarIdempotencia(ctx context.Context, org, endpoint, clave, hash string, r puertos.RespuestaGuardada) error {
	if _, err := t.tx.ExecContext(ctx, `INSERT INTO claves_idempotencia (organizacion_id, endpoint, clave,
		hash_cuerpo, estado_http, respuesta, creada_en) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		org, endpoint, clave, hash, r.EstadoHTTP, string(r.Cuerpo), time.Now().UTC()); err != nil {
		return fmt.Errorf("guardar clave de idempotencia: %w", err)
	}
	return nil
}

func texto(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	return &v.String
}

func hora(v sql.NullTime) *time.Time {
	if !v.Valid {
		return nil
	}
	t := v.Time.UTC()
	return &t
}

// utc pasa una hora opcional a UTC (todo se guarda en UTC).
func utc(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}
