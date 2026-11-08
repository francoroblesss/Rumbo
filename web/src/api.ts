// Cliente HTTP único de la web (ADR-029). Toda llamada al gateway pasa por acá:
// - agrega Authorization: Bearer con el token de la sesión (la web nunca manda headers de
//   identidad: los pone el gateway a partir del token, teórico 6);
// - genera la Idempotency-Key de la cancelación una vez por intención (teórico 9);
// - traduce Problem Details (RFC 9457) a un ErrorApi con el `detail` y los errores por campo;
// - ante un 401 vuelve al ingreso; ante un 502, 503 o 504 muestra que Rumbo no está disponible;
// - no reintenta escrituras por su cuenta.

import type {
  Aviso,
  DiaDeFranjas,
  Pagina,
  Perfil,
  ResultadoBusqueda,
  Token,
  VistaSolicitud,
} from './tipos'

/** URL del gateway. Se fija al compilar (VITE_API_URL); por defecto, el gateway local. */
export const API_URL: string = (import.meta.env?.VITE_API_URL as string | undefined) || 'http://localhost:8080'

export const MENSAJE_NO_DISPONIBLE = 'Rumbo no está disponible en este momento. Probá de nuevo en unos segundos.'

export interface ErrorCampo {
  campo: string
  mensaje: string
}

/** Error de una llamada a la API, ya listo para mostrar. */
export class ErrorApi extends Error {
  /** Estado HTTP; 0 si no hubo respuesta (error de red). */
  readonly estado: number
  /** `codigo` de Problem Details (por ejemplo, ESTADO_NO_PERMITE_OPERACION). */
  readonly codigo: string | undefined
  readonly errores: ErrorCampo[]

  constructor(estado: number, mensaje: string, codigo?: string, errores: ErrorCampo[] = []) {
    super(mensaje)
    this.name = 'ErrorApi'
    this.estado = estado
    this.codigo = codigo
    this.errores = errores
  }

  /** El servicio (o el gateway) no está disponible: 502, 503, 504 o sin respuesta. */
  get noDisponible(): boolean {
    return this.estado === 0 || this.estado === 502 || this.estado === 503 || this.estado === 504
  }
}

interface Configuracion {
  obtenerToken: () => string | null
  alNoAutenticado: () => void
}

let configuracion: Configuracion = { obtenerToken: () => null, alNoAutenticado: () => {} }

/** La sesión le dice al cliente de dónde sacar el token y qué hacer ante un 401. */
export function configurarApi(nueva: Configuracion): void {
  configuracion = nueva
}

interface Opciones {
  metodo?: 'GET' | 'POST'
  cuerpo?: unknown
  claveIdempotencia?: string
  /** Para el login: un 401 es "credenciales inválidas", no "la sesión venció". */
  sinSesion?: boolean
  senal?: AbortSignal
}

function encabezados(opciones: Opciones): Headers {
  const headers = new Headers({ Accept: 'application/json' })
  const token = opciones.sinSesion ? null : configuracion.obtenerToken()
  if (token) headers.set('Authorization', `Bearer ${token}`)
  if (opciones.cuerpo !== undefined) headers.set('Content-Type', 'application/json')
  if (opciones.claveIdempotencia) headers.set('Idempotency-Key', opciones.claveIdempotencia)
  return headers
}

/** Traduce una respuesta de error a ErrorApi (Problem Details si vino; si no, un mensaje genérico). */
async function errorDe(respuesta: Response, opciones: Opciones): Promise<ErrorApi> {
  if (respuesta.status === 401 && !opciones.sinSesion) {
    configuracion.alNoAutenticado()
    return new ErrorApi(401, 'La sesión venció. Ingresá de nuevo.', 'NO_AUTENTICADO')
  }

  let problema: { detail?: string; title?: string; codigo?: string; errores?: ErrorCampo[] } = {}
  try {
    problema = await respuesta.json()
  } catch {
    // Sin cuerpo JSON (por ejemplo, un 502 del gateway sin Problem Details).
  }

  if ([502, 503, 504].includes(respuesta.status)) {
    return new ErrorApi(respuesta.status, MENSAJE_NO_DISPONIBLE, problema.codigo)
  }
  const mensaje = problema.detail || problema.title || `Error ${respuesta.status}`
  return new ErrorApi(respuesta.status, mensaje, problema.codigo, problema.errores ?? [])
}

async function enviar(ruta: string, opciones: Opciones): Promise<Response> {
  try {
    return await fetch(API_URL + ruta, {
      method: opciones.metodo ?? 'GET',
      headers: encabezados(opciones),
      body: opciones.cuerpo === undefined ? undefined : JSON.stringify(opciones.cuerpo),
      signal: opciones.senal,
    })
  } catch (error) {
    if (error instanceof DOMException && error.name === 'AbortError') throw error
    throw new ErrorApi(0, MENSAJE_NO_DISPONIBLE)
  }
}

/** Hace una llamada al gateway y devuelve el JSON de la respuesta (o undefined si es 204). */
export async function pedir<T>(ruta: string, opciones: Opciones = {}): Promise<T> {
  const respuesta = await enviar(ruta, opciones)
  if (!respuesta.ok) throw await errorDe(respuesta, opciones)
  if (respuesta.status === 204) return undefined as T
  return (await respuesta.json()) as T
}

/** Abre el stream SSE de avisos (§14). Devuelve la respuesta para leer su cuerpo. */
export async function abrirStream(ruta: string, ultimoId: string | undefined, senal: AbortSignal): Promise<Response> {
  const opciones: Opciones = { senal }
  const headers = encabezados(opciones)
  headers.set('Accept', 'text/event-stream')
  if (ultimoId) headers.set('Last-Event-ID', ultimoId)
  let respuesta: Response
  try {
    respuesta = await fetch(API_URL + ruta, { headers, signal: senal, cache: 'no-store' })
  } catch (error) {
    if (error instanceof DOMException && error.name === 'AbortError') throw error
    throw new ErrorApi(0, MENSAJE_NO_DISPONIBLE)
  }
  if (!respuesta.ok || !respuesta.body) throw await errorDe(respuesta, opciones)
  return respuesta
}

// ---------------- Parámetros ----------------

/** Arma "?a=1&b=2" con los valores definidos y no vacíos. */
export function consulta(parametros: Record<string, string | number | boolean | undefined | null>): string {
  const busqueda = new URLSearchParams()
  for (const [clave, valor] of Object.entries(parametros)) {
    if (valor !== undefined && valor !== null && valor !== '' && valor !== false) busqueda.set(clave, String(valor))
  }
  const texto = busqueda.toString()
  return texto ? `?${texto}` : ''
}

// ---------------- Identidad (§4.3 y §15) ----------------

export function ingresar(email: string, password: string): Promise<Token> {
  return pedir<Token>('/api/identidad/login', { metodo: 'POST', cuerpo: { email, password }, sinSesion: true })
}

export function obtenerPerfil(): Promise<Perfil> {
  return pedir<Perfil>('/api/identidad/yo')
}

// ---------------- Operaciones (§12 y §16) ----------------

export function obtenerFranjas(fecha: string): Promise<DiaDeFranjas> {
  return pedir<DiaDeFranjas>(`/api/operaciones/franjas${consulta({ fecha })}`)
}

export interface FiltrosSolicitudes {
  fecha?: string
  estado_traslado?: string
  pagina: number
  tamanio: number
}

export function listarSolicitudes(filtros: FiltrosSolicitudes): Promise<Pagina<VistaSolicitud>> {
  return pedir<Pagina<VistaSolicitud>>(`/api/operaciones/solicitudes${consulta({ ...filtros })}`)
}

export function obtenerSolicitud(id: string): Promise<VistaSolicitud> {
  return pedir<VistaSolicitud>(`/api/operaciones/solicitudes/${encodeURIComponent(id)}`)
}

export function avanzarEstado(id: string, estado: 'EN_CURSO' | 'COMPLETADO'): Promise<VistaSolicitud> {
  return pedir<VistaSolicitud>(`/api/operaciones/solicitudes/${encodeURIComponent(id)}/estado`, {
    metodo: 'POST',
    cuerpo: { estado },
  })
}

/**
 * Intención de cancelar una solicitud. La Idempotency-Key se genera una sola vez, al crear la
 * intención: si el usuario reintenta la misma cancelación (doble clic o error de red), se reusa
 * la clave y Operaciones devuelve la respuesta original en lugar de cancelar dos veces (teórico 9).
 * Un intento nuevo de cancelar desde cero crea otra intención y, con ella, otra clave.
 * Si cambia el motivo, ya no es el mismo pedido: se genera otra clave (reusar la anterior con
 * otro cuerpo daría 409 CLAVE_IDEMPOTENCIA_REUTILIZADA).
 */
export interface IntencionCancelacion {
  readonly clave: string
  ejecutar: (motivo: string) => Promise<VistaSolicitud>
}

export function intencionDeCancelar(solicitudId: string): IntencionCancelacion {
  let clave = crypto.randomUUID()
  let motivoEnviado: string | undefined
  return {
    get clave() {
      return clave
    },
    ejecutar(motivo: string) {
      if (motivoEnviado !== undefined && motivoEnviado !== motivo) clave = crypto.randomUUID()
      motivoEnviado = motivo
      return pedir<VistaSolicitud>(`/api/operaciones/solicitudes/${encodeURIComponent(solicitudId)}/cancelacion`, {
        metodo: 'POST',
        cuerpo: { motivo },
        claveIdempotencia: clave,
      })
    },
  }
}

export interface FiltrosBusqueda {
  q?: string
  estado?: string
  desde?: string
  hasta?: string
  alerta?: boolean
  orden?: 'fecha' | 'relevancia'
  pagina: number
  tamanio: number
}

export function buscarTraslados(filtros: FiltrosBusqueda): Promise<Pagina<ResultadoBusqueda>> {
  return pedir<Pagina<ResultadoBusqueda>>(`/api/operaciones/traslados/busqueda${consulta({ ...filtros })}`)
}

export function reconstruirIndice(): Promise<{ estado: string }> {
  return pedir<{ estado: string }>('/api/operaciones/admin/reindexacion', { metodo: 'POST' })
}

// ---------------- Notificaciones (§14) ----------------

export const RUTA_STREAM_AVISOS = '/api/notificaciones/avisos/stream'

export function listarAvisos(filtros: { no_leidos?: boolean; pagina: number; tamanio: number }): Promise<Pagina<Aviso>> {
  return pedir<Pagina<Aviso>>(`/api/notificaciones/avisos${consulta({ ...filtros })}`)
}

export function marcarLeido(id: string): Promise<void> {
  return pedir<void>(`/api/notificaciones/avisos/${encodeURIComponent(id)}/lectura`, { metodo: 'POST' })
}
