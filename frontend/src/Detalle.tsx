import { useEffect, useRef, useState, type FormEvent, type ReactNode } from 'react'
import { accionSolicitud, ErrorApi, obtenerSolicitud } from './api.ts'
import { etiqueta, fecha, type Solicitud } from './tipos.ts'

interface Props {
  id: string
  refresco: number
  alCambiar: () => void
}

type Accion = 'aceptacion' | 'contrapropuesta' | 'rechazo' | 'cancelacion-tardia'

export default function Detalle({ id, refresco, alCambiar }: Props) {
  const [s, setS] = useState<Solicitud | null>(null)
  const [error, setError] = useState('')
  const [enviando, setEnviando] = useState(false)
  const [extremo, setExtremo] = useState('')
  const [horaPropuesta, setHoraPropuesta] = useState('')
  const [motivo, setMotivo] = useState('')
  // Una Idempotency-Key por operación lógica (acción + cuerpo). Si el pedido falla y el operador
  // reintenta lo mismo, viaja la misma clave y Operaciones no repite el efecto (teórico 9).
  const claves = useRef(new Map<string, string>())

  useEffect(() => {
    let vigente = true
    obtenerSolicitud(id)
      .then((v) => {
        if (vigente) {
          setS(v)
          setError('')
        }
      })
      .catch((err: Error) => vigente && setError(err.message))
    return () => {
      vigente = false
    }
  }, [id, refresco])

  async function ejecutar(accion: Accion, cuerpo: unknown) {
    const operacion = `${id}|${accion}|${JSON.stringify(cuerpo)}`
    let clave = claves.current.get(operacion)
    if (!clave) {
      clave = crypto.randomUUID()
      claves.current.set(operacion, clave)
    }
    setEnviando(true)
    setError('')
    try {
      setS(await accionSolicitud(id, accion, cuerpo, clave))
      claves.current.delete(operacion)
      alCambiar()
    } catch (err) {
      setError(mensaje(err))
      // Un 4xx es definitivo para ese cuerpo: el próximo intento es otra operación.
      if (err instanceof ErrorApi && err.problema.status >= 400 && err.problema.status < 500) {
        claves.current.delete(operacion)
      }
    } finally {
      setEnviando(false)
    }
  }

  if (!s) return error ? <p className="alerta error">{error}</p> : <p className="vacio">Cargando…</p>

  const recibida = s.estado === 'RECIBIDA'
  const cancelacionPendiente = s.cancelacion?.estado === 'PENDIENTE_APROBACION'
  const alerta = s.traslado?.alerta_climatica

  return (
    <div className="detalle-contenido">
      <div className="detalle-cabecera">
        <div>
          <h2>{s.cosa.descripcion}</h2>
          <p className="tenue chico">
            {s.id}
            {s.referencia_externa ? ` · ref. ${s.referencia_externa}` : ''}
          </p>
        </div>
        <span className={`estado e-${s.estado.toLowerCase()}`}>{etiqueta(s.estado)}</span>
      </div>

      {alerta && (
        <p className={`alerta ${alerta.estado === 'NO_OPERABLE' ? 'error' : 'info'}`}>
          <strong>{etiqueta(alerta.estado)}</strong>
          {alerta.motivos && alerta.motivos.length > 0 ? `: ${alerta.motivos.join(', ')}` : ''}
          {alerta.observado_en ? ` (observado ${fecha(alerta.observado_en)})` : ''}. Decide el operador.
        </p>
      )}

      <dl className="datos">
        <dt>Qué</dt>
        <dd>
          {s.cosa.cantidad} × {s.cosa.descripcion}
          {s.cosa.peso_kg ? ` · ${s.cosa.peso_kg} kg` : ''}
          {s.cosa.volumen_m3 ? ` · ${s.cosa.volumen_m3} m³` : ''}
        </dd>
        <dt>Origen</dt>
        <dd>
          {s.origen.nombre} <span className="tenue">· {s.origen.direccion}</span>
        </dd>
        <dt>Destino</dt>
        <dd>
          {s.destino.nombre} <span className="tenue">· {s.destino.direccion}</span>
        </dd>
        <dt>Llegada pedida</dt>
        <dd>
          {s.hora_llegada_solicitada ? fecha(s.hora_llegada_solicitada) : 'Sin hora: hay que proponer una'}
          {s.tolerancia_minutos ? <span className="tenue"> · tolerancia {s.tolerancia_minutos} min</span> : null}
        </dd>
        {s.observaciones && (
          <>
            <dt>Observaciones</dt>
            <dd>{s.observaciones}</dd>
          </>
        )}
        {s.contrapropuesta && (
          <>
            <dt>Contrapropuesta</dt>
            <dd>
              {fecha(s.contrapropuesta.hora_llegada_propuesta)}
              {s.contrapropuesta.vence_en && <span className="tenue"> · vence {fecha(s.contrapropuesta.vence_en)}</span>}
            </dd>
          </>
        )}
        {s.traslado && (
          <>
            <dt>Traslado</dt>
            <dd>
              {etiqueta(s.traslado.estado)}
              {s.traslado.hora_llegada_comprometida && (
                <span className="tenue"> · llegada comprometida {fecha(s.traslado.hora_llegada_comprometida)}</span>
              )}
            </dd>
          </>
        )}
        {s.motivo_rechazo && (
          <>
            <dt>Motivo de rechazo</dt>
            <dd>{s.motivo_rechazo}</dd>
          </>
        )}
        {s.cancelacion && (
          <>
            <dt>Cancelación</dt>
            <dd>
              {etiqueta(s.cancelacion.estado)}
              {s.cancelacion.motivo && <span className="tenue"> · {s.cancelacion.motivo}</span>}
            </dd>
          </>
        )}
        <dt>Actualizada</dt>
        <dd className="tenue">{fecha(s.actualizado_en)}</dd>
      </dl>

      {error && <p className="alerta error">{error}</p>}

      {recibida && (
        <div className="acciones">
          <AccionForm
            titulo="Aceptar con la hora pedida"
            deshabilitada={!s.hora_llegada_solicitada}
            ayuda={!s.hora_llegada_solicitada ? 'No tiene hora pedida: proponé una.' : undefined}
            boton="Aceptar"
            enviando={enviando}
            alEnviar={() => ejecutar('aceptacion', { aeropuerto_extremo: extremo || null })}
          >
            <label>
              ¿Algún extremo es el aeropuerto? (consulta el clima)
              <select value={extremo} onChange={(e) => setExtremo(e.target.value)}>
                <option value="">No</option>
                <option value="ORIGEN">Sí, el origen</option>
                <option value="DESTINO">Sí, el destino</option>
              </select>
            </label>
          </AccionForm>

          <AccionForm
            titulo="Proponer otro horario"
            boton="Contraproponer"
            enviando={enviando}
            deshabilitada={!horaPropuesta}
            alEnviar={() => ejecutar('contrapropuesta', { hora_llegada_propuesta: `${horaPropuesta}:00-03:00` })}
          >
            <label>
              Llegada propuesta (hora de Córdoba)
              <input type="datetime-local" value={horaPropuesta} onChange={(e) => setHoraPropuesta(e.target.value)} />
            </label>
          </AccionForm>

          <AccionForm
            titulo="Rechazar"
            boton="Rechazar"
            peligro
            enviando={enviando}
            deshabilitada={!motivo.trim()}
            alEnviar={() => ejecutar('rechazo', { motivo: motivo.trim() })}
          >
            <label>
              Motivo
              <input value={motivo} onChange={(e) => setMotivo(e.target.value)} placeholder="Ej.: sin vehículo con esa capacidad" />
            </label>
          </AccionForm>
        </div>
      )}

      {cancelacionPendiente && (
        <div className="acciones fila-botones">
          <p>El cliente pidió cancelar fuera de término. ¿Se aprueba?</p>
          <button className="boton" disabled={enviando} onClick={() => ejecutar('cancelacion-tardia', { aprobar: true })}>
            Aprobar cancelación
          </button>
          <button className="boton secundario" disabled={enviando} onClick={() => ejecutar('cancelacion-tardia', { aprobar: false })}>
            No aprobar
          </button>
        </div>
      )}
    </div>
  )
}

interface AccionProps {
  titulo: string
  boton: string
  enviando: boolean
  deshabilitada?: boolean
  peligro?: boolean
  ayuda?: string
  alEnviar: () => void
  children: ReactNode
}

function AccionForm({ titulo, boton, enviando, deshabilitada, peligro, ayuda, alEnviar, children }: AccionProps) {
  function enviar(e: FormEvent) {
    e.preventDefault()
    alEnviar()
  }
  return (
    <form className="accion" onSubmit={enviar}>
      <h3>{titulo}</h3>
      {children}
      {ayuda && <p className="tenue chico">{ayuda}</p>}
      <button className={`boton${peligro ? ' peligro' : ''}`} disabled={enviando || deshabilitada}>
        {boton}
      </button>
    </form>
  )
}

// mensaje arma un texto para el operador a partir del Problem Details.
function mensaje(err: unknown): string {
  if (!(err instanceof ErrorApi)) return 'Ocurrió un error inesperado.'
  const p = err.problema
  if (p.status === 429) return 'Demasiados pedidos seguidos. Esperá unos segundos y reintentá.'
  if (p.status >= 500 || p.status === 0) return `${err.message} Podés reintentar: no se duplica.`
  const campos = p.errores?.map((e) => `${e.campo}: ${e.mensaje}`).join('; ')
  return campos ? `${err.message} (${campos})` : err.message
}
