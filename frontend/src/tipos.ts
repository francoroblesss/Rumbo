// Vista de solicitud de Operaciones (docs/interfaces-internas.md §6).

export type EstadoSolicitud =
  | 'RECIBIDA'
  | 'CONTRAPROPUESTA'
  | 'ACEPTADA'
  | 'RECHAZADA'
  | 'RECHAZADA_POR_CLIENTE'
  | 'CANCELADA'

export interface Ubicacion {
  nombre: string
  direccion: string
  contacto?: string | null
}

export interface Solicitud {
  id: string
  referencia_externa?: string | null
  estado: EstadoSolicitud | string
  cosa: { descripcion: string; cantidad: number; peso_kg?: number | null; volumen_m3?: number | null }
  origen: Ubicacion
  destino: Ubicacion
  hora_llegada_solicitada?: string | null
  tolerancia_minutos?: number | null
  observaciones?: string | null
  contrapropuesta?: { hora_llegada_propuesta: string; vence_en?: string | null } | null
  motivo_rechazo?: string | null
  traslado?: {
    id: string
    estado: string
    hora_llegada_comprometida?: string | null
    alerta_climatica?: { estado: string; motivos?: string[] | null; observado_en?: string | null } | null
  } | null
  cancelacion?: { estado: string; motivo?: string | null; solicitada_en?: string | null } | null
  version?: number
  creado_en: string
  actualizado_en: string
}

export interface PaginaSolicitudes {
  resultados: Solicitud[]
  pagina: number
  tamanio: number
  total: number
}

export const ESTADOS: { valor: string; etiqueta: string }[] = [
  { valor: '', etiqueta: 'Todas' },
  { valor: 'RECIBIDA', etiqueta: 'Recibidas' },
  { valor: 'CONTRAPROPUESTA', etiqueta: 'Contrapropuestas' },
  { valor: 'ACEPTADA', etiqueta: 'Aceptadas' },
  { valor: 'RECHAZADA', etiqueta: 'Rechazadas' },
  { valor: 'RECHAZADA_POR_CLIENTE', etiqueta: 'Rechazadas por el cliente' },
  { valor: 'CANCELADA', etiqueta: 'Canceladas' },
]

const ETIQUETAS: Record<string, string> = {
  RECIBIDA: 'Recibida',
  CONTRAPROPUESTA: 'Contrapropuesta',
  ACEPTADA: 'Aceptada',
  RECHAZADA: 'Rechazada',
  RECHAZADA_POR_CLIENTE: 'Rechazada por el cliente',
  CANCELADA: 'Cancelada',
  PROGRAMADO: 'Programado',
  ASIGNADO: 'Asignado',
  EN_CURSO: 'En curso',
  COMPLETADO: 'Completado',
  CANCELADO: 'Cancelado',
  NO_REALIZADO: 'No realizado',
  INTERRUMPIDO: 'Interrumpido',
  PENDIENTE_APROBACION: 'Pendiente de aprobación',
  APROBADA: 'Aprobada',
  NO_OPERABLE: 'Aeropuerto no operable',
  NO_VERIFICADO: 'Clima no verificado',
}

// etiqueta traduce un estado del contrato; si llega uno desconocido se muestra tal cual.
export function etiqueta(estado: string): string {
  return ETIQUETAS[estado] ?? estado
}

const FORMATO = new Intl.DateTimeFormat('es-AR', {
  dateStyle: 'short',
  timeStyle: 'short',
  timeZone: 'America/Argentina/Cordoba',
})

export function fecha(iso?: string | null): string {
  if (!iso) return '—'
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? iso : FORMATO.format(d)
}
