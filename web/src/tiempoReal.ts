// Avisos en tiempo real (bonus, ADR-027): un único stream SSE por sesión, leído con fetch.
//
// - Se lee con fetch + getReader + TextDecoder porque EventSource no permite el header
//   Authorization, y el token no se pone en la URL (quedaría en los logs) (teórico 9).
// - El stream no es la fuente de verdad: ante cada aviso se invalidan las consultas de React
//   Query y la pantalla abierta vuelve a pedir los datos a la API (se reconcilia sola).
// - Si se corta, se reconecta con backoff (1, 2, 4… hasta 30 s) mandando Last-Event-ID, y
//   Notificaciones reenvía lo que se perdió.

import { useEffect } from 'react'
import { useQueryClient, type QueryClient } from '@tanstack/react-query'
import { notifications } from '@mantine/notifications'
import { ErrorApi, RUTA_STREAM_AVISOS, abrirStream } from './api'
import { LectorSSE } from './sse'
import { useSesion } from './sesion'
import type { Aviso, Pagina } from './tipos'

export const ESPERA_MAXIMA_MS = 30_000

/** Espera antes del intento n (desde 0): 1, 2, 4, 8, 16, 30, 30… segundos. */
export function esperaReconexion(intento: number): number {
  return Math.min(1000 * 2 ** intento, ESPERA_MAXIMA_MS)
}

/** Consultas que dependen de un aviso: siempre los avisos; si es de una solicitud, todo lo que la muestra. */
export function invalidarPorAviso(clienteConsultas: QueryClient, aviso: Aviso): void {
  void clienteConsultas.invalidateQueries({ queryKey: ['avisos'] })
  if (aviso.entidad?.tipo === 'solicitud') {
    void clienteConsultas.invalidateQueries({ queryKey: ['solicitudes'] })
    void clienteConsultas.invalidateQueries({ queryKey: ['franjas'] })
    void clienteConsultas.invalidateQueries({ queryKey: ['busqueda'] })
    void clienteConsultas.invalidateQueries({ queryKey: ['solicitud', aviso.entidad.id] })
  }
}

function esperar(ms: number, senal: AbortSignal): Promise<void> {
  return new Promise((resolver) => {
    const temporizador = setTimeout(resolver, ms)
    senal.addEventListener('abort', () => {
      clearTimeout(temporizador)
      resolver()
    })
  })
}

/** Lee el stream hasta que se corte o se aborte, avisando cada evento `aviso`. */
async function leerStream(
  ultimoId: { valor: string | undefined },
  alAviso: (aviso: Aviso) => void,
  alConectar: () => void,
  senal: AbortSignal,
): Promise<void> {
  const respuesta = await abrirStream(RUTA_STREAM_AVISOS, ultimoId.valor, senal)
  alConectar()
  const lector = respuesta.body!.getReader()
  const decodificador = new TextDecoder()
  const sse = new LectorSSE((evento) => {
    if (evento.id) ultimoId.valor = evento.id
    if (evento.evento !== 'aviso') return
    try {
      alAviso(JSON.parse(evento.datos) as Aviso)
    } catch {
      // Un aviso ilegible no corta el stream.
    }
  })
  try {
    for (;;) {
      const { value, done } = await lector.read()
      if (done) return
      sse.agregar(decodificador.decode(value, { stream: true }))
    }
  } finally {
    lector.releaseLock()
  }
}

/** Mantiene abierto el stream de avisos mientras haya sesión. Se monta una sola vez, en el layout. */
export function useAvisosEnTiempoReal(): void {
  const { token } = useSesion()
  const clienteConsultas = useQueryClient()

  useEffect(() => {
    if (!token) return
    const control = new AbortController()
    const ultimoId: { valor: string | undefined } = { valor: undefined }

    const alAviso = (aviso: Aviso) => {
      notifications.show({ title: aviso.titulo, message: aviso.mensaje })
      // La campana suma al instante; la invalidación trae después el total real de la API.
      clienteConsultas.setQueryData<Pagina<Aviso>>(['avisos', 'no-leidos'], (previo) =>
        previo ? { ...previo, total: previo.total + 1 } : previo,
      )
      invalidarPorAviso(clienteConsultas, aviso)
    }

    void (async () => {
      let intento = 0
      while (!control.signal.aborted) {
        try {
          await leerStream(ultimoId, alAviso, () => (intento = 0), control.signal)
        } catch (error) {
          if (control.signal.aborted) return
          // 401: la sesión venció; api.ts ya volvió al ingreso y el efecto se limpia solo.
          if (error instanceof ErrorApi && error.estado === 401) return
        }
        if (control.signal.aborted) return
        await esperar(esperaReconexion(intento), control.signal)
        intento++
      }
    })()

    // Al salir de la sesión (o cambiar de token), se cierra el stream.
    return () => control.abort()
  }, [token, clienteConsultas])
}
