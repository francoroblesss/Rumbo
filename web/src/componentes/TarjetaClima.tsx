import { Alert, Badge, Card, Group, Loader, Progress, SimpleGrid, Stack, Text } from '@mantine/core'
import {
  IconArrowUp,
  IconCloudRain,
  IconCloudStorm,
  IconDroplet,
  IconEye,
  IconMist,
  IconSun,
  IconTemperature,
  IconWind,
  type Icon,
} from '@tabler/icons-react'
import { useQuery } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { obtenerClima } from '../api'
import {
  condicionClima,
  hora,
  LIMITE_RAFAGAS_KMH,
  LIMITE_VIENTO_KMH,
  LIMITE_VISIBILIDAD_M,
  MOTIVOS_CLIMA,
  type CondicionClima,
} from '../formato'

// Los 5 min de la caché del backend (ADR-012): pedir más seguido no traería un dato más nuevo.
const CINCO_MINUTOS = 5 * 60_000

const ASPECTO: Record<CondicionClima, { icono: Icon; color: string; texto: string }> = {
  TORMENTA: { icono: IconCloudStorm, color: 'red', texto: 'Tormenta eléctrica' },
  VIENTO: { icono: IconWind, color: 'orange', texto: 'Viento fuerte' },
  BAJA_VISIBILIDAD: { icono: IconMist, color: 'orange', texto: 'Visibilidad reducida' },
  LLUVIA: { icono: IconCloudRain, color: 'blue', texto: 'Lluvia' },
  BUENO: { icono: IconSun, color: 'yellow', texto: 'Sin fenómenos adversos' },
}

/** Número con coma decimal y hasta un decimal: 22,4. */
function numero(n: number): string {
  return n.toLocaleString('es-AR', { maximumFractionDigits: 1 })
}

function visibilidad(metros: number): string {
  return metros >= 1000 ? `${numero(metros / 1000)} km` : `${numero(metros)} m`
}

/** Verde hasta el 70 % del límite, naranja hasta el límite y rojo al pasarlo: el proveedor deja de
 * operar con "más de" el límite, así que justo en el límite sigue operable. */
function colorBarra(porcentaje: number): string {
  if (porcentaje > 100) return 'red'
  if (porcentaje >= 70) return 'orange'
  return 'green'
}

function Dato({ icono, children }: { icono: ReactNode; children: ReactNode }) {
  return (
    <Group gap={6} wrap="nowrap">
      {icono}
      <Text size="sm">{children}</Text>
    </Group>
  )
}

function Barra({ etiqueta, valor, limite }: { etiqueta: string; valor: number; limite: number }) {
  const porcentaje = (valor / limite) * 100
  return (
    <div>
      <Text size="sm" mb={4}>
        {etiqueta}: {numero(valor)} de {limite} km/h
      </Text>
      <Progress value={Math.min(100, porcentaje)} color={colorBarra(porcentaje)} aria-label={etiqueta} />
    </div>
  )
}

/**
 * Condiciones actuales del aeropuerto (§13.5, ADR-034). Si el clima no responde se dice que no se
 * sabe: nunca un clima de ejemplo ni uno viejo como si fuera actual (teórico 7). La falla queda en
 * la tarjeta: el resto de la página sigue funcionando.
 */
export function TarjetaClima() {
  const clima = useQuery({
    queryKey: ['clima'],
    queryFn: obtenerClima,
    staleTime: CINCO_MINUTOS,
    refetchInterval: CINCO_MINUTOS,
    // Sin reintentos: un 503 dice que el clima no responde, y reintentar enseguida solo le suma
    // pedidos al proveedor. Se vuelve a pedir en el próximo refetch (5 min).
    retry: false,
  })

  let contenido: ReactNode
  if (clima.isPending) {
    contenido = <Loader size="sm" />
  } else if (clima.isError) {
    contenido = (
      <Alert color="gray" title="Clima no disponible">
        No se pudo consultar el clima del aeropuerto. Se vuelve a intentar en unos minutos.
      </Alert>
    )
  } else {
    const c = clima.data
    const aspecto = ASPECTO[condicionClima(c)]
    const IconoCondicion = aspecto.icono
    const color = (nombre: string) => `var(--mantine-color-${nombre}-6)`
    contenido = (
      <Stack gap="md">
        <Group gap="md" wrap="nowrap">
          <IconoCondicion size={56} color={color(aspecto.color)} aria-hidden />
          <Stack gap={4}>
            <Text fw={600} size="lg">
              {aspecto.texto}
            </Text>
            <Group gap="xs">
              <Badge color={c.operable ? 'green' : 'red'}>{c.operable ? 'Aeropuerto operable' : 'Aeropuerto no operable'}</Badge>
              {!c.operable && <Text size="sm">{c.motivos.map((m) => MOTIVOS_CLIMA[m] ?? m).join(', ')}</Text>}
            </Group>
          </Stack>
        </Group>

        <SimpleGrid cols={{ base: 2, sm: 4 }}>
          <Dato icono={<IconTemperature size={18} aria-hidden />}>{numero(c.condiciones.temperatura_c)} °C</Dato>
          <Dato
            icono={
              // El proveedor informa de dónde viene el viento; la flecha apunta hacia donde va.
              <IconArrowUp
                size={18}
                aria-hidden
                style={{ transform: `rotate(${(c.condiciones.direccion_viento_grados + 180) % 360}deg)` }}
              />
            }
          >
            {numero(c.condiciones.viento_kmh)} km/h, ráfagas {numero(c.condiciones.rafagas_kmh)} km/h
          </Dato>
          <Dato icono={<IconEye size={18} aria-hidden />}>
            <Text span size="sm" c={c.condiciones.visibilidad_m < LIMITE_VISIBILIDAD_M ? 'red' : undefined}>
              {visibilidad(c.condiciones.visibilidad_m)}
            </Text>
          </Dato>
          <Dato icono={<IconDroplet size={18} aria-hidden />}>{numero(c.condiciones.precipitacion_mm)} mm</Dato>
        </SimpleGrid>

        <SimpleGrid cols={{ base: 1, sm: 2 }}>
          <Barra etiqueta="Viento" valor={c.condiciones.viento_kmh} limite={LIMITE_VIENTO_KMH} />
          <Barra etiqueta="Ráfagas" valor={c.condiciones.rafagas_kmh} limite={LIMITE_RAFAGAS_KMH} />
        </SimpleGrid>
      </Stack>
    )
  }

  return (
    <Card withBorder>
      <Stack gap="sm">
        <div>
          <Text fw={600}>Clima en el aeropuerto</Text>
          {clima.data && !clima.isError && (
            <Text size="xs" c="dimmed">
              {clima.data.aeropuerto.nombre} ({clima.data.aeropuerto.icao}) · observado a las {hora(clima.data.observado_en)}
            </Text>
          )}
        </div>
        {contenido}
      </Stack>
    </Card>
  )
}
