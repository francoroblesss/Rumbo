package aplicacion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/francoroblesss/Rumbo/servicios/comun/mensajeria"
	"github.com/francoroblesss/Rumbo/servicios/comun/observabilidad"
	"github.com/francoroblesss/Rumbo/servicios/operaciones/dominio"
	"github.com/francoroblesss/Rumbo/servicios/operaciones/puertos"
)

// ExchangeEventos es el exchange de los eventos de Operaciones (docs/interfaces-internas.md §5.2).
const ExchangeEventos = "rumbo.operaciones"

// Solicitudes implementa los casos de uso de la solicitud (puertos.Solicitudes). Cada uno sigue
// el mismo patrón: una unidad de trabajo → cargar el agregado con bloqueo → aplicar la regla del
// dominio → guardar → agregar los eventos al outbox. Todo en una sola transacción.
type Solicitudes struct {
	uow        puertos.UnidadDeTrabajo
	clima      puertos.ConsultaClima
	icao       string
	palabras   []string // AEROPUERTO_PALABRAS (R-46)
	calendario dominio.Calendario
	reloj      func() time.Time
	instancia  string

	solicitudesTotal metric.Int64Counter
	trasladosEstado  metric.Int64Counter
	climaConsultas   metric.Int64Counter
}

var _ puertos.Solicitudes = (*Solicitudes)(nil)

// NuevasSolicitudes arma los casos de uso con sus dependencias (puertos de salida).
func NuevasSolicitudes(uow puertos.UnidadDeTrabajo, clima puertos.ConsultaClima, icao string, palabras []string,
	calendario dominio.Calendario, reloj func() time.Time, instancia string) *Solicitudes {
	medidor := observabilidad.Medidor()
	solicitudesTotal, _ := medidor.Int64Counter("rumbo_solicitudes_total",
		metric.WithDescription("Transiciones de solicitudes por origen y estado resultante"))
	trasladosEstado, _ := medidor.Int64Counter("rumbo_traslados_estado_total",
		metric.WithDescription("Transiciones de estado del traslado aplicadas desde la web, por estado resultante"))
	climaConsultas, _ := medidor.Int64Counter("rumbo_clima_consultas_total",
		metric.WithDescription("Consultas de clima por resultado y momento (reserva o inicio)"))
	return &Solicitudes{uow: uow, clima: clima, icao: icao, palabras: palabras, calendario: calendario, reloj: reloj,
		instancia: instancia, solicitudesTotal: solicitudesTotal, trasladosEstado: trasladosEstado, climaConsultas: climaConsultas}
}

// cambio es lo que un caso de uso le hace al agregado. emitir agrega al outbox el evento de la
// transición recién aplicada, con la vista y la versión de ese momento. tx sirve para lo que el
// cambio arrastra fuera del agregado en la misma transacción (devolver el lugar de una franja).
// Si devuelve errSinCambios, no había nada que aplicar: cambiar responde la vista actual sin
// escribir ni publicar (un reintento de algo que ya se hizo).
type cambio func(ctx context.Context, tx puertos.Transaccion, s *dominio.Solicitud, emitir func(dominio.Evento) error) error

// errSinCambios lo devuelve un cambio que no tiene nada que aplicar (idempotencia, teórico 4).
var errSinCambios = errors.New("sin cambios")

// cambiar ejecuta un cambio sobre una solicitud existente en una transacción. Si org no está
// vacío y no es el dueño, responde ErrNoEncontrada (no se revela que existe). Con idem, la
// clave de idempotencia se resuelve en la misma transacción.
func (a *Solicitudes) cambiar(ctx context.Context, org, id string, idem *puertos.Idempotencia, fn cambio) (puertos.Vista, error) {
	var vista puertos.Vista
	var final *dominio.Solicitud
	err := a.uow.Ejecutar(ctx, func(ctx context.Context, tx puertos.Transaccion) error {
		final = nil
		s, err := tx.ObtenerParaActualizar(ctx, id)
		if err != nil {
			return err
		}
		if org != "" && s.OrganizacionID != org {
			return puertos.ErrNoEncontrada
		}
		if idem != nil {
			// Reintento del mismo pedido: la misma respuesta, sin volver a aplicar el cambio.
			if repetido, err := buscarRespuesta(ctx, tx, org, idem, &vista); err != nil || repetido {
				return err
			}
		}
		err = fn(ctx, tx, s, func(ev dominio.Evento) error { return agregarEvento(ctx, tx, s, ev) })
		if errors.Is(err, errSinCambios) {
			vista = NuevaVista(s)
			return nil
		}
		if err != nil {
			return err
		}
		if err := tx.Actualizar(ctx, s); err != nil {
			return err
		}
		vista = NuevaVista(s)
		if idem != nil {
			if err := guardarRespuesta(ctx, tx, org, idem, 200, vista); err != nil {
				return err
			}
		}
		final = s
		return nil
	})
	if err != nil {
		return puertos.Vista{}, err
	}
	if final != nil {
		a.contar(ctx, final)
	}
	return vista, nil
}

// buscarRespuesta resuelve la clave de idempotencia: si ya se usó con el mismo cuerpo, deja en
// vista la respuesta original y devuelve true (no hay que volver a aplicar nada).
func buscarRespuesta(ctx context.Context, tx puertos.Transaccion, org string, idem *puertos.Idempotencia, vista *puertos.Vista) (bool, error) {
	guardada, err := tx.BuscarIdempotencia(ctx, org, idem.Endpoint, idem.Clave)
	if err != nil || guardada == nil {
		return false, err
	}
	if guardada.Hash != idem.Hash {
		return false, puertos.ErrClaveReutilizada
	}
	return true, json.Unmarshal(guardada.Cuerpo, vista)
}

// guardarRespuesta conserva la respuesta de la clave, en la misma transacción que el cambio.
func guardarRespuesta(ctx context.Context, tx puertos.Transaccion, org string, idem *puertos.Idempotencia, estado int, vista puertos.Vista) error {
	cuerpo, err := json.Marshal(vista)
	if err != nil {
		return fmt.Errorf("serializar la respuesta: %w", err)
	}
	return tx.GuardarIdempotencia(ctx, org, idem.Endpoint, idem.Clave, idem.Hash,
		puertos.RespuestaGuardada{Hash: idem.Hash, EstadoHTTP: estado, Cuerpo: cuerpo})
}

// agregarEvento guarda el evento en el outbox, en la misma transacción que el cambio (patrón
// outbox, teórico 2): o se guardan los dos o ninguno.
func agregarEvento(ctx context.Context, tx puertos.Transaccion, s *dominio.Solicitud, ev dominio.Evento) error {
	datos := map[string]any{
		"organizacion_id": s.OrganizacionID,
		"origen":          string(s.Origen),
		"solicitud":       NuevaVista(s),
	}
	sobre, err := mensajeria.NuevoSobre(ctx, string(ev), s.ID, s.Version, datos)
	if err != nil {
		return err
	}
	return tx.AgregarAlOutbox(ctx, ExchangeEventos, string(ev), sobre)
}

func (a *Solicitudes) contar(ctx context.Context, s *dominio.Solicitud) {
	a.solicitudesTotal.Add(ctx, 1, metric.WithAttributes(
		attribute.String("origen", string(s.Origen)), attribute.String("estado", string(s.Estado))))
}

func nuevoIDTraslado() string { return "tra-" + uuid.NewString() }

func nuevoIDSolicitud() string { return "sol-" + uuid.NewString() }

// Reservar es la operación crítica de la capacidad publicada (D4, ADR-022). En UNA transacción:
// resuelve la clave de idempotencia, bloquea la franja (SELECT … FOR UPDATE), verifica que haya
// lugar, lo descuenta, crea la solicitud aceptada con su traslado y deja el evento en el outbox.
// Dos reservas simultáneas sobre la misma franja se ordenan en el bloqueo: la segunda ve el cupo
// que dejó la primera, así nunca se reserva más de lo que hay.
func (a *Solicitudes) Reservar(ctx context.Context, org string, d dominio.DatosSolicitud, franjaInicio time.Time, idem *puertos.Idempotencia) (puertos.Vista, error) {
	ahora := a.reloj()
	var vista puertos.Vista
	var creada *dominio.Solicitud
	err := a.uow.Ejecutar(ctx, func(ctx context.Context, tx puertos.Transaccion) error {
		creada = nil
		// Primero la clave: un reintento devuelve la reserva original aunque, mientras tanto, la
		// franja se haya llenado o ya no tenga la anticipación mínima.
		if repetido, err := buscarRespuesta(ctx, tx, org, idem, &vista); err != nil || repetido {
			return err
		}
		plantilla, err := a.calendario.Validar(franjaInicio, ahora)
		if err != nil {
			return err
		}
		franja, err := tx.FranjaParaActualizar(ctx, plantilla)
		if err != nil {
			return err
		}
		s, ev, err := dominio.NuevaReservaExterna(nuevoIDSolicitud(), nuevoIDTraslado(), org, d, franja, a.palabras, ahora)
		if err != nil {
			return err
		}
		if err := tx.GuardarFranja(ctx, franja); err != nil {
			return err
		}
		if err := tx.Insertar(ctx, s); err != nil {
			return err
		}
		if err := agregarEvento(ctx, tx, s, ev); err != nil {
			return err
		}
		vista = NuevaVista(s)
		creada = s
		return guardarRespuesta(ctx, tx, org, idem, 201, vista)
	})
	if err != nil {
		return puertos.Vista{}, err
	}
	if creada != nil {
		a.contar(ctx, creada)
		slog.InfoContext(ctx, "franja reservada", slog.String("solicitud_id", creada.ID),
			slog.String("franja_inicio", hora(*creada.Traslado.FranjaInicio)))
	}
	return vista, nil
}

// Disponibilidad lista las franjas de una fecha que todavía se pueden reservar, con el lugar que
// les queda. Es una lectura sin bloqueo: orienta al consumidor, pero la que decide es Reservar
// (otro puede tomar el último lugar entre la consulta y la reserva).
func (a *Solicitudes) Disponibilidad(ctx context.Context, fecha string, pesoKg float64) (puertos.Disponibilidad, error) {
	dia, err := time.ParseInLocation(time.DateOnly, fecha, zonaRumbo)
	if err != nil || pesoKg < 0 {
		return puertos.Disponibilidad{}, dominio.ErrDatosInvalidos
	}
	franjas, err := a.calendario.DelDia(dia.Year(), dia.Month(), dia.Day(), a.reloj())
	if err != nil {
		return puertos.Disponibilidad{}, err
	}
	franjas, err = a.conOcupacion(ctx, franjas)
	if err != nil {
		return puertos.Disponibilidad{}, err
	}
	resultado := puertos.Disponibilidad{Fecha: fecha, Franjas: []puertos.FranjaDisponible{}}
	for _, f := range franjas {
		resultado.Franjas = append(resultado.Franjas, puertos.FranjaDisponible{
			Inicio: hora(f.Inicio), Fin: hora(f.Fin), Disponible: f.Admite(pesoKg),
			TrasladosDisponibles: f.TrasladosDisponibles(), KgDisponibles: f.KgDisponibles(),
		})
	}
	return resultado, nil
}

// conOcupacion reemplaza cada franja del calendario por la guardada en MySQL, si alguien ya
// reservó en ella. Una franja sin fila queda con el cupo completo y nada reservado.
func (a *Solicitudes) conOcupacion(ctx context.Context, franjas []dominio.Franja) ([]dominio.Franja, error) {
	if len(franjas) == 0 {
		return franjas, nil
	}
	var guardadas []dominio.Franja
	err := a.uow.Ejecutar(ctx, func(ctx context.Context, tx puertos.Transaccion) error {
		var err error
		guardadas, err = tx.Franjas(ctx, franjas[0].Inicio, franjas[len(franjas)-1].Inicio)
		return err
	})
	if err != nil {
		return nil, err
	}
	for i, f := range franjas {
		for _, g := range guardadas {
			if g.Inicio.Equal(f.Inicio) {
				franjas[i] = g
			}
		}
	}
	return franjas, nil
}

// Cancelar aplica la cancelación y, si el traslado tenía una franja reservada, le devuelve el
// lugar en la misma transacción (se bloquea primero la solicitud y después la franja).
func (a *Solicitudes) Cancelar(ctx context.Context, org, id, motivo string, idem *puertos.Idempotencia) (puertos.Vista, error) {
	ahora := a.reloj()
	return a.cambiar(ctx, org, id, idem, func(ctx context.Context, tx puertos.Transaccion, s *dominio.Solicitud, emitir func(dominio.Evento) error) error {
		ev, err := s.Cancelar(motivo, ahora)
		if err != nil {
			return err
		}
		if err := emitir(ev); err != nil {
			return err
		}
		inicio, peso, libera := s.LiberaFranja()
		if !libera {
			return nil
		}
		franja, err := tx.FranjaParaActualizar(ctx, a.calendario.Nueva(inicio))
		if err != nil {
			return err
		}
		franja.Liberar(peso)
		return tx.GuardarFranja(ctx, franja)
	})
}

func (a *Solicitudes) Obtener(ctx context.Context, org, id string) (puertos.Vista, error) {
	s, err := a.leer(ctx, org, id)
	if err != nil {
		return puertos.Vista{}, err
	}
	return NuevaVista(s), nil
}

// leer carga una solicitud sin bloquearla, verificando la organización.
func (a *Solicitudes) leer(ctx context.Context, org, id string) (*dominio.Solicitud, error) {
	var s *dominio.Solicitud
	err := a.uow.Ejecutar(ctx, func(ctx context.Context, tx puertos.Transaccion) error {
		var err error
		s, err = tx.Obtener(ctx, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	if org != "" && s.OrganizacionID != org {
		return nil, puertos.ErrNoEncontrada
	}
	return s, nil
}
