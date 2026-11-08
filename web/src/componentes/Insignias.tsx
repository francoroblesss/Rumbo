import { Badge, Group, Tooltip } from '@mantine/core'
import { textoEstado } from '../formato'

const COLOR_ESTADO: Record<string, string> = {
  PROGRAMADO: 'blue',
  EN_CURSO: 'violet',
  COMPLETADO: 'green',
  CANCELADO: 'gray',
}

/** Estado del traslado con su texto en español. */
export function EstadoTraslado({ estado }: { estado: string | undefined }) {
  if (!estado) return null
  return <Badge color={COLOR_ESTADO[estado] ?? 'gray'}>{textoEstado(estado)}</Badge>
}

/** Alerta climática: roja si el aeropuerto no está operable (motivos en el tooltip), amarilla si no se verificó. */
export function AlertaClima({ estado, motivos }: { estado: string | null | undefined; motivos?: string[] | null }) {
  if (estado === 'NO_OPERABLE') {
    const texto = motivos && motivos.length > 0 ? motivos.join(', ') : 'Sin motivos informados'
    return (
      <Tooltip label={texto} withArrow>
        <Badge color="red" tabIndex={0}>
          Aeropuerto no operable
        </Badge>
      </Tooltip>
    )
  }
  if (estado === 'NO_VERIFICADO') return <Badge color="yellow">Clima no verificado</Badge>
  return null
}

export function CancelacionTardia({ tardia }: { tardia: boolean | undefined }) {
  return tardia ? <Badge color="orange">Cancelación tardía</Badge> : null
}

/** Las tres insignias juntas (estado, alerta y cancelación tardía), para las tablas. */
export function Insignias(props: {
  estado: string | undefined
  alerta: string | null | undefined
  motivos?: string[] | null
  tardia: boolean | undefined
}) {
  return (
    <Group gap={4}>
      <EstadoTraslado estado={props.estado} />
      <AlertaClima estado={props.alerta} motivos={props.motivos} />
      <CancelacionTardia tardia={props.tardia} />
    </Group>
  )
}
