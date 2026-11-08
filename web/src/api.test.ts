import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  ErrorApi,
  MENSAJE_NO_DISPONIBLE,
  configurarApi,
  intencionDeCancelar,
  obtenerSolicitud,
  pedir,
} from './api'

// fetch simulado: guarda los pedidos y responde lo que se le indique, en orden.
function simularFetch(...respuestas: Array<Response | Error>) {
  const pedidos: Array<{ url: string; init: RequestInit }> = []
  const fetchFalso = vi.fn(async (url: string, init: RequestInit) => {
    pedidos.push({ url, init })
    const siguiente = respuestas.shift()
    if (!siguiente) throw new Error('no hay más respuestas simuladas')
    if (siguiente instanceof Error) throw siguiente
    return siguiente
  })
  vi.stubGlobal('fetch', fetchFalso)
  return pedidos
}

function problema(estado: number, cuerpo: object): Response {
  return new Response(JSON.stringify(cuerpo), {
    status: estado,
    headers: { 'Content-Type': 'application/problem+json' },
  })
}

function json(cuerpo: object, estado = 200): Response {
  return new Response(JSON.stringify(cuerpo), { status: estado, headers: { 'Content-Type': 'application/json' } })
}

function encabezado(init: RequestInit, nombre: string): string | null {
  return new Headers(init.headers).get(nombre)
}

let alNoAutenticado: ReturnType<typeof vi.fn<() => void>>

beforeEach(() => {
  alNoAutenticado = vi.fn<() => void>()
  configurarApi({ obtenerToken: () => 'token-de-prueba', alNoAutenticado })
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('pedir', () => {
  it('agrega Authorization: Bearer con el token de la sesión', async () => {
    const pedidos = simularFetch(json({ id: 'sol-1' }))
    await obtenerSolicitud('sol-1')
    expect(encabezado(pedidos[0].init, 'Authorization')).toBe('Bearer token-de-prueba')
    expect(pedidos[0].url).toMatch(/\/api\/operaciones\/solicitudes\/sol-1$/)
  })

  it('traduce Problem Details al detail y a los errores por campo', async () => {
    simularFetch(
      problema(422, {
        status: 422,
        codigo: 'DATOS_INVALIDOS',
        title: 'Datos inválidos',
        detail: 'Falta un dato obligatorio.',
        errores: [{ campo: 'motivo', mensaje: 'es obligatorio' }],
      }),
    )
    const error = await pedir('/x').catch((e: unknown) => e)
    expect(error).toBeInstanceOf(ErrorApi)
    const e = error as ErrorApi
    expect(e.estado).toBe(422)
    expect(e.codigo).toBe('DATOS_INVALIDOS')
    expect(e.message).toBe('Falta un dato obligatorio.')
    expect(e.errores).toEqual([{ campo: 'motivo', mensaje: 'es obligatorio' }])
  })

  it('muestra el detail de un 409', async () => {
    simularFetch(
      problema(409, { codigo: 'ESTADO_NO_PERMITE_OPERACION', title: 'x', detail: 'El traslado ya está en curso.' }),
    )
    await expect(pedir('/x', { metodo: 'POST', cuerpo: {} })).rejects.toThrow('El traslado ya está en curso.')
  })

  it('ante un 401 vuelve al ingreso', async () => {
    simularFetch(problema(401, { codigo: 'NO_AUTENTICADO', detail: 'No autenticado' }))
    const error = (await pedir('/x').catch((e: unknown) => e)) as ErrorApi
    expect(error.estado).toBe(401)
    expect(alNoAutenticado).toHaveBeenCalledOnce()
  })

  it('un 401 del login son credenciales inválidas, no una sesión vencida', async () => {
    simularFetch(problema(401, { codigo: 'NO_AUTENTICADO', detail: 'Las credenciales no son válidas.' }))
    await expect(pedir('/api/identidad/login', { metodo: 'POST', cuerpo: {}, sinSesion: true })).rejects.toThrow(
      'Las credenciales no son válidas.',
    )
    expect(alNoAutenticado).not.toHaveBeenCalled()
  })

  it.each([502, 503, 504])('ante un %i muestra que Rumbo no está disponible', async (estado) => {
    simularFetch(problema(estado, { codigo: 'SERVICIO_NO_DISPONIBLE', detail: 'detalle interno' }))
    const error = (await pedir('/x').catch((e: unknown) => e)) as ErrorApi
    expect(error.message).toBe(MENSAJE_NO_DISPONIBLE)
    expect(error.noDisponible).toBe(true)
  })

  it('un 503 sin cuerpo JSON también es "no disponible"', async () => {
    simularFetch(new Response('Service Unavailable', { status: 503 }))
    await expect(pedir('/x')).rejects.toThrow(MENSAJE_NO_DISPONIBLE)
  })

  it('un error de red es "no disponible" y no se reintenta solo', async () => {
    const pedidos = simularFetch(new TypeError('Failed to fetch'))
    const error = (await pedir('/x', { metodo: 'POST', cuerpo: {} }).catch((e: unknown) => e)) as ErrorApi
    expect(error.estado).toBe(0)
    expect(error.message).toBe(MENSAJE_NO_DISPONIBLE)
    expect(pedidos).toHaveLength(1)
  })
})

describe('intencionDeCancelar', () => {
  const vista = { id: 'sol-1', traslado: { estado: 'CANCELADO' } }

  it('reintentar la misma cancelación reusa la misma Idempotency-Key', async () => {
    const pedidos = simularFetch(new TypeError('Failed to fetch'), json(vista))
    const intencion = intencionDeCancelar('sol-1')

    await expect(intencion.ejecutar('Se suspendió el envío')).rejects.toBeInstanceOf(ErrorApi)
    await intencion.ejecutar('Se suspendió el envío')

    const claves = pedidos.map((p) => encabezado(p.init, 'Idempotency-Key'))
    expect(claves[0]).toBeTruthy()
    expect(claves[1]).toBe(claves[0])
    expect(pedidos[0].url).toMatch(/\/api\/operaciones\/solicitudes\/sol-1\/cancelacion$/)
    expect(pedidos[0].init.method).toBe('POST')
    expect(JSON.parse(pedidos[0].init.body as string)).toEqual({ motivo: 'Se suspendió el envío' })
  })

  it('una cancelación nueva desde cero genera otra clave', async () => {
    const pedidos = simularFetch(json(vista), json(vista))
    await intencionDeCancelar('sol-1').ejecutar('motivo')
    await intencionDeCancelar('sol-1').ejecutar('motivo')
    const [primera, segunda] = pedidos.map((p) => encabezado(p.init, 'Idempotency-Key'))
    expect(primera).not.toBe(segunda)
  })

  it('si cambia el motivo ya no es el mismo pedido: otra clave', async () => {
    const pedidos = simularFetch(new TypeError('Failed to fetch'), json(vista))
    const intencion = intencionDeCancelar('sol-1')
    await intencion.ejecutar('primero').catch(() => {})
    await intencion.ejecutar('otro motivo')
    const [primera, segunda] = pedidos.map((p) => encabezado(p.init, 'Idempotency-Key'))
    expect(primera).not.toBe(segunda)
  })
})
