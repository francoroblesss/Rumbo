// Formato de fechas y textos en la hora de Argentina (docs/interfaces-internas.md §11.1).

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
