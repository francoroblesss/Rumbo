import { useState } from 'react'
import { Group, Loader, Progress, Stack, Table, Text, TextInput, Title } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from 'react-router'
import { obtenerFranjas } from '../api'
import { MensajeError } from '../componentes/MensajeError'
import { hora, hoy } from '../formato'
import type { Franja } from '../tipos'

/**
 * Ocupación de la franja, en %: la del cupo que esté más lleno (traslados o kg), porque la franja
 * deja de aceptar reservas cuando se llena cualquiera de los dos.
 */
function ocupacion(f: Franja): number {
  const traslados = f.cupo_traslados > 0 ? f.traslados_reservados / f.cupo_traslados : 0
  const kg = f.cupo_kg > 0 ? f.kg_reservados / f.cupo_kg : 0
  return Math.min(100, Math.round(Math.max(traslados, kg) * 100))
}

function colorOcupacion(porcentaje: number): string {
  if (porcentaje >= 100) return 'red'
  if (porcentaje >= 75) return 'orange'
  return 'teal'
}

/** /franjas (operador y administrador): las franjas de un día con su ocupación (§12). */
export function Franjas() {
  const navegar = useNavigate()
  const [fecha, setFecha] = useState(hoy)
  const franjas = useQuery({
    queryKey: ['franjas', fecha],
    queryFn: () => obtenerFranjas(fecha),
    enabled: fecha !== '',
  })

  return (
    <Stack>
      <Group justify="space-between" align="flex-end">
        <Title order={2}>Franjas</Title>
        <TextInput type="date" label="Fecha" value={fecha} onChange={(e) => setFecha(e.currentTarget.value)} />
      </Group>

      {franjas.isPending && fecha !== '' && <Loader />}
      <MensajeError error={franjas.error} titulo="No se pudieron cargar las franjas" />

      {franjas.data && (
        <Table.ScrollContainer minWidth={560}>
          <Table highlightOnHover>
            <Table.Thead>
              <Table.Tr>
                <Table.Th>Horario</Table.Th>
                <Table.Th>Traslados</Table.Th>
                <Table.Th>Kg</Table.Th>
                <Table.Th w="30%">Ocupación</Table.Th>
              </Table.Tr>
            </Table.Thead>
            <Table.Tbody>
              {franjas.data.franjas.map((f) => {
                const porcentaje = ocupacion(f)
                const abrir = () => navegar(`/traslados?fecha=${franjas.data.fecha}`)
                return (
                  <Table.Tr
                    key={f.inicio}
                    onClick={abrir}
                    onKeyDown={(e) => e.key === 'Enter' && abrir()}
                    tabIndex={0}
                    style={{ cursor: 'pointer' }}
                    aria-label={`Ver los traslados del ${franjas.data.fecha}`}
                  >
                    <Table.Td>
                      {hora(f.inicio)} a {hora(f.fin)}
                    </Table.Td>
                    <Table.Td>
                      {f.traslados_reservados} / {f.cupo_traslados}
                    </Table.Td>
                    <Table.Td>
                      {f.kg_reservados} / {f.cupo_kg}
                    </Table.Td>
                    <Table.Td>
                      <Group gap="xs" wrap="nowrap">
                        <Progress
                          value={porcentaje}
                          color={colorOcupacion(porcentaje)}
                          style={{ flex: 1 }}
                          aria-label={`Ocupación ${porcentaje} %`}
                        />
                        <Text size="xs" w={40} ta="right">
                          {porcentaje} %
                        </Text>
                      </Group>
                    </Table.Td>
                  </Table.Tr>
                )
              })}
            </Table.Tbody>
          </Table>
        </Table.ScrollContainer>
      )}
    </Stack>
  )
}
