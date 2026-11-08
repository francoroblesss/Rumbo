import { describe, expect, it } from 'vitest'
import { LectorSSE, type EventoSSE } from './sse'

function lector() {
  const eventos: EventoSSE[] = []
  return { eventos, sse: new LectorSSE((e) => eventos.push(e)) }
}

const aviso = '{"id":"m-1","titulo":"Reserva nueva"}'

describe('LectorSSE', () => {
  it('emite un evento completo con su nombre, id y datos', () => {
    const { eventos, sse } = lector()
    sse.agregar(`event: aviso\nid: m-1\ndata: ${aviso}\n\n`)
    expect(eventos).toEqual([{ evento: 'aviso', id: 'm-1', datos: aviso }])
  })

  it('arma un evento partido entre dos lecturas', () => {
    const { eventos, sse } = lector()
    sse.agregar('event: aviso\nid: m-1\nda')
    expect(eventos).toHaveLength(0)
    sse.agregar(`ta: ${aviso.slice(0, 10)}`)
    expect(eventos).toHaveLength(0)
    sse.agregar(`${aviso.slice(10)}\n`)
    expect(eventos).toHaveLength(0) // falta la línea vacía que cierra el evento
    sse.agregar('\n')
    expect(eventos).toEqual([{ evento: 'aviso', id: 'm-1', datos: aviso }])
  })

  it('ignora los comentarios ": ping"', () => {
    const { eventos, sse } = lector()
    sse.agregar(': ping\n\n')
    sse.agregar(`: ping\nevent: aviso\nid: m-2\ndata: {}\n\n: ping\n\n`)
    expect(eventos).toEqual([{ evento: 'aviso', id: 'm-2', datos: '{}' }])
  })

  it('separa varios eventos que llegan en la misma lectura', () => {
    const { eventos, sse } = lector()
    sse.agregar('event: aviso\nid: a\ndata: 1\n\nevent: aviso\nid: b\ndata: 2\n\nevent: aviso\nid: c\ndata: 3\n\n')
    expect(eventos.map((e) => [e.id, e.datos])).toEqual([
      ['a', '1'],
      ['b', '2'],
      ['c', '3'],
    ])
  })

  it('tolera \\r\\n y une varias líneas data con \\n', () => {
    const { eventos, sse } = lector()
    sse.agregar('data: linea 1\r\ndata: linea 2\r\n\r\n')
    expect(eventos).toEqual([{ evento: 'message', id: undefined, datos: 'linea 1\nlinea 2' }])
  })

  it('conserva el último id para los eventos siguientes y no emite eventos sin data', () => {
    const { eventos, sse } = lector()
    sse.agregar('id: m-9\n\n') // sin data: no se emite, pero actualiza el último id
    sse.agregar('event: aviso\ndata: x\n\n')
    expect(eventos).toEqual([{ evento: 'aviso', id: 'm-9', datos: 'x' }])
  })
})
