// Formato de fechas y textos en la hora de Argentina (docs/interfaces-internas.md §11.1).

import type { ClimaAeropuerto } from './tipos'

const ZONA = 'America/Argentina/Cordoba'

const formatoFechaHora = new Intl.DateTimeFormat('es-AR', {
  timeZone: ZONA,
  day: '2-digit',
  month: '2-digit',
  year: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
})

const formatoDia = new Intl.DateTimeFormat('en-CA', {
  timeZone: ZONA,
  year: 'numeric',
  month: '2-digit',
  day: '2-digit',
})

function partes(formato: Intl.DateTimeFormat, fecha: Date): Record<string, string> {
  const resultado: Record<string, string> = {}
  for (const parte of formato.formatToParts(fecha)) resultado[parte.type] = parte.value
  return resultado
}

/** "dd/mm/aaaa HH:MM" en hora de Argentina. */
export function fechaHora(iso: string | null | undefined): string {
  if (!iso) return '—'
  const p = partes(formatoFechaHora, new Date(iso))
  return `${p.day}/${p.month}/${p.year} ${p.hour}:${p.minute}`
}

/** "HH:MM" en hora de Argentina. */
export function hora(iso: string): string {
  const p = partes(formatoFechaHora, new Date(iso))
  return `${p.hour}:${p.minute}`
}

/** Franja como "dd/mm/aaaa HH:MM a HH:MM". */
export function franja(inicio: string | undefined, fin: string | undefined): string {
  if (!inicio || !fin) return '—'
  return `${fechaHora(inicio)} a ${hora(fin)}`
}

/** Fecha de hoy en Argentina, "AAAA-MM-DD" (formato de los filtros). */
export function hoy(): string {
  const p = partes(formatoDia, new Date())
  return `${p.year}-${p.month}-${p.day}`
}

const TEXTO_ESTADO: Record<string, string> = {
  PROGRAMADO: 'Programado',
  EN_CURSO: 'En curso',
  COMPLETADO: 'Completado',
  CANCELADO: 'Cancelado',
}

/** Estados del traslado que se producen en esta etapa (ADR-024), para los filtros. */
export const ESTADOS_TRASLADO = Object.entries(TEXTO_ESTADO).map(([value, label]) => ({ value, label }))

/** Texto en español del estado del traslado. */
export function textoEstado(estado: string): string {
  return TEXTO_ESTADO[estado] ?? estado
}

// ---------------- Clima (§13.5, ADR-034) ----------------

// Límites operativos del proveedor (contrato de clima 2.1.0): más de 50 km/h de viento, más de
// 65 km/h de ráfagas o menos de 800 m de visibilidad hacen que el aeropuerto no sea operable.
export const LIMITE_VIENTO_KMH = 50
export const LIMITE_RAFAGAS_KMH = 65
export const LIMITE_VISIBILIDAD_M = 800

export const MOTIVOS_CLIMA: Record<string, string> = {
  HIGH_WIND: 'Viento fuerte',
  HIGH_GUSTS: 'Ráfagas fuertes',
  LOW_VISIBILITY: 'Baja visibilidad',
  THUNDERSTORM: 'Tormenta eléctrica',
}

export type CondicionClima = 'TORMENTA' | 'VIENTO' | 'BAJA_VISIBILIDAD' | 'LLUVIA' | 'BUENO'

/**
 * Condición para el ícono de la tarjeta; gana la primera que se cumple. Viento y visibilidad salen
 * de los motivos del proveedor, no de umbrales propios: así la web nunca contradice si el
 * aeropuerto es operable. Sin dato de nubosidad no se puede decir "soleado": BUENO es "sin
 * fenómenos adversos".
 */
export function condicionClima(c: ClimaAeropuerto): CondicionClima {
  if (c.condiciones.tormenta || c.motivos.includes('THUNDERSTORM')) return 'TORMENTA'
  if (c.motivos.includes('HIGH_WIND') || c.motivos.includes('HIGH_GUSTS')) return 'VIENTO'
  if (c.motivos.includes('LOW_VISIBILITY')) return 'BAJA_VISIBILIDAD'
  if (c.condiciones.precipitacion_mm > 0) return 'LLUVIA'
  return 'BUENO'
}
