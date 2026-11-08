// Tipos de las respuestas de la API, tal como las define docs/interfaces-internas.md (§6 y §11 a §16).
// La web programa contra el contrato: si un campo no está acá, no se usa.

export type Rol = 'cliente' | 'operador' | 'administrador' | 'chofer' | 'sistema_externo'

/** GET /api/identidad/yo (§15). */
export interface Perfil {
  id: string
  email: string
  nombre: string
  roles: Rol[]
  organizacion: { id: string; nombre: string } | null
}

/** Respuesta de POST /api/identidad/login (§4.3). */
export interface Token {
  access_token: string
  token_type: 'Bearer'
  expira_en_segundos: number
}

/** Página de un listado (§11.1). */
export interface Pagina<T> {
  resultados: T[]
  pagina: number
  tamanio: number
  total: number
}

/** GET /api/operaciones/franjas (§12). */
export interface Franja {
  inicio: string
  fin: string
  cupo_traslados: number
  traslados_reservados: number
  cupo_kg: number
  kg_reservados: number
}

export interface DiaDeFranjas {
  fecha: string
  franjas: Franja[]
}

/** Estados del traslado que se producen en esta etapa (ADR-024); el resto existe en el contrato público. */
export type EstadoTraslado = 'PROGRAMADO' | 'EN_CURSO' | 'COMPLETADO' | 'CANCELADO' | string

export interface AlertaClimatica {
  estado: 'NO_OPERABLE' | 'NO_VERIFICADO'
  motivos?: string[] | null
  observado_en?: string | null
}

export interface Lugar {
  nombre: string
  direccion: string
}

/** Vista de solicitud (§6). */
export interface VistaSolicitud {
  id: string
  referencia_externa?: string | null
  estado: string
  cosa: { descripcion: string; cantidad: number; peso_kg?: number | null }
  origen: Lugar
  destino: Lugar
  tolerancia_minutos?: number | null
  observaciones?: string | null
  traslado?: {
    id: string
    estado: EstadoTraslado
    hora_llegada_comprometida?: string | null
    demora_minutos?: number | null
    finalizado_en?: string | null
    detalle_resultado?: string | null
    franja?: { inicio: string; fin: string } | null
    alerta_climatica?: AlertaClimatica | null
  } | null
  cancelacion?: {
    estado: string
    motivo: string
    solicitada_en: string
    tardia: boolean
  } | null
  version: number
  creado_en: string
  actualizado_en: string
}

/** Resultado de GET /api/operaciones/traslados/busqueda (§16). */
export interface ResultadoBusqueda {
  solicitud_id: string
  traslado_id: string
  organizacion_id: string
  referencia_externa: string | null
  descripcion: string
  origen_nombre: string
  destino_nombre: string
  estado_traslado: EstadoTraslado
  franja_inicio: string
  franja_fin: string
  alerta_climatica: 'NO_OPERABLE' | 'NO_VERIFICADO' | null
  cancelacion_tardia: boolean
}

/** Aviso de Notificaciones (§14). */
export interface Aviso {
  id: string
  destinatario: { tipo: 'ORGANIZACION' | 'ROL'; valor: string }
  titulo: string
  mensaje: string
  entidad: { tipo: string; id: string }
  creado_en: string
  leido: boolean
}
