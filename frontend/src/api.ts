// Cliente de la API interna de Rumbo. Todo pasa por el API Gateway (/api/...): la web nunca
// habla directo con los microservicios.

import type { PaginaSolicitudes, Solicitud } from './tipos.ts'

const BASE = import.meta.env.VITE_API_URL ?? ''
const CLAVE_TOKEN = 'rumbo.token'
const CLAVE_EMAIL = 'rumbo.email'

// Problema es el cuerpo Problem Details (RFC 9457) que devuelven todos los servicios.
export interface Problema {
  status: number
  codigo?: string
  title?: string
  detail?: string
  errores?: { campo: string; mensaje: string }[]
}

export class ErrorApi extends Error {
  readonly problema: Problema

  constructor(problema: Problema) {
    super(problema.detail ?? problema.title ?? `Error ${problema.status}`)
    this.problema = problema
  }
}

// El token vive en sessionStorage: dura lo que la pestaña y no se comparte entre pestañas.
export function tokenGuardado(): string | null {
  try {
    return sessionStorage.getItem(CLAVE_TOKEN)
  } catch {
    return null
  }
}

function guardarToken(token: string | null, email?: string) {
  try {
    if (token) {
      sessionStorage.setItem(CLAVE_TOKEN, token)
      if (email) sessionStorage.setItem(CLAVE_EMAIL, email)
    } else {
      sessionStorage.removeItem(CLAVE_TOKEN)
      sessionStorage.removeItem(CLAVE_EMAIL)
    }
  } catch {
    // Sin almacenamiento la sesión dura hasta recargar la página.
  }
}

let alVencerSesion: () => void = () => {}

// alExpirar registra qué hacer cuando el gateway responde 401 (token vencido o inválido).
export function alExpirar(accion: () => void) {
  alVencerSesion = accion
}

async function pedir<T>(metodo: string, ruta: string, cuerpo?: unknown, claveIdempotencia?: string): Promise<T> {
  const headers: Record<string, string> = { Accept: 'application/json' }
  const token = tokenGuardado()
  if (token) headers.Authorization = `Bearer ${token}`
  if (cuerpo !== undefined) headers['Content-Type'] = 'application/json'
  if (claveIdempotencia) headers['Idempotency-Key'] = claveIdempotencia

  let respuesta: Response
  try {
    respuesta = await fetch(BASE + ruta, {
      method: metodo,
      headers,
      body: cuerpo === undefined ? undefined : JSON.stringify(cuerpo),
    })
  } catch {
    throw new ErrorApi({ status: 0, detail: 'No se pudo conectar con Rumbo. Revisá la conexión y reintentá.' })
  }

  if (respuesta.ok) return (await respuesta.json()) as T

  let problema: Problema = { status: respuesta.status }
  try {
    problema = { ...problema, ...((await respuesta.json()) as Problema) }
  } catch {
    // Respuesta sin cuerpo: alcanza con el status.
  }
  if (respuesta.status === 401 && token) {
    guardarToken(null)
    alVencerSesion()
  }
  throw new ErrorApi(problema)
}

export async function iniciarSesion(email: string, password: string): Promise<void> {
  const r = await pedir<{ access_token: string }>('POST', '/api/identidad/login', { email, password })
  guardarToken(r.access_token, email)
}

export function cerrarSesion() {
  guardarToken(null)
}

// usuarioConectado devuelve el email con el que se inició sesión y los roles del token, solo para
// mostrarlos. La validación la hace el gateway y la autorización cada servicio: la web no decide
// nada con estos datos.
export function usuarioConectado(): { email: string; roles: string[] } {
  let email = ''
  try {
    email = sessionStorage.getItem(CLAVE_EMAIL) ?? ''
  } catch {
    // sin almacenamiento
  }
  return { email, roles: rolesDelToken() }
}

function rolesDelToken(): string[] {
  const token = tokenGuardado()
  if (!token) return []
  try {
    const payload = token.split('.')[1].replace(/-/g, '+').replace(/_/g, '/')
    const json = decodeURIComponent(
      atob(payload)
        .split('')
        .map((c) => '%' + c.charCodeAt(0).toString(16).padStart(2, '0'))
        .join(''),
    )
    return (JSON.parse(json) as { roles?: string[] }).roles ?? []
  } catch {
    return []
  }
}

export function listarSolicitudes(estado: string, pagina: number, tamanio: number) {
  const q = new URLSearchParams({ pagina: String(pagina), tamanio: String(tamanio) })
  if (estado) q.set('estado', estado)
  return pedir<PaginaSolicitudes>('GET', `/api/operaciones/solicitudes?${q}`)
}

export function obtenerSolicitud(id: string) {
  return pedir<Solicitud>('GET', `/api/operaciones/solicitudes/${encodeURIComponent(id)}`)
}

// Las acciones son POST: cada una lleva la Idempotency-Key de su operación lógica (teórico 9).
// Quien llama conserva la clave mientras reintenta la misma acción, así un reintento después de
// un timeout no repite el efecto.
export function accionSolicitud(id: string, accion: string, cuerpo: unknown, clave: string) {
  return pedir<Solicitud>('POST', `/api/operaciones/solicitudes/${encodeURIComponent(id)}/${accion}`, cuerpo, clave)
}
