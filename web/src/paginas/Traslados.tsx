import { Group, Loader, Pagination, Select, Stack, Table, Text, TextInput, Title } from '@mantine/core'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { useNavigate, useSearchParams } from 'react-router'
import { listarSolicitudes } from '../api'
import { Insignias } from '../componentes/Insignias'
import { MensajeError } from '../componentes/MensajeError'
import { ESTADOS_TRASLADO, franja } from '../formato'
import { esPersonal, useSesion } from '../sesion'

const TAMANIO = 20

/**
 * /traslados: operador y administrador ven todas las organizaciones; el cliente, solo la suya.
 * La web no manda organizacion_id: Operaciones filtra por la identidad del gateway (§11.1).
 * Los filtros viven en la URL, así el clic en una franja abre /traslados?fecha=….
 */
export function Traslados() {
  const { perfil } = useSesion()
  const navegar = useNavigate()
  const [parametros, setParametros] = useSearchParams()
  const fecha = parametros.get('fecha') ?? ''
  const estado = parametros.get('estado') ?? ''
  const pagina = Number(parametros.get('pagina') ?? '1') || 1

  function cambiar(cambios: Record<string, string>) {
    const nuevos = new URLSearchParams(parametros)
    for (const [clave, valor] of Object.entries(cambios)) {
      if (valor) nuevos.set(clave, valor)
      else nuevos.delete(clave)
    }
    if (!('pagina' in cambios)) nuevos.delete('pagina') // un filtro nuevo vuelve a la primera página
    setParametros(nuevos)
  }

  const filtros = { fecha: fecha || undefined, estado_traslado: estado || undefined, pagina, tamanio: TAMANIO }
  const solicitudes = useQuery({
    queryKey: ['solicitudes', filtros],
    queryFn: () => listarSolicitudes(filtros),
    placeholderData: keepPreviousData,
  })

  const titulo =
    !esPersonal(perfil) && perfil?.organizacion
      ? `Traslados de ${perfil.organizacion.nombre || perfil.organizacion.id}`
      : 'Traslados'
  const paginas = solicitudes.data ? Math.max(1, Math.ceil(solicitudes.data.total / TAMANIO)) : 1

  return (
    <Stack>
      <Title order={2}>{titulo}</Title>
      <Group align="flex-end">
        <TextInput type="date" label="Fecha" value={fecha} onChange={(e) => cambiar({ fecha: e.currentTarget.value })} />
        <Select
          label="Estado"
          placeholder="Todos"
          data={ESTADOS_TRASLADO}
          value={estado || null}
          onChange={(v) => cambiar({ estado: v ?? '' })}
          clearable
        />
      </Group>

      {solicitudes.isPending && <Loader />}
      <MensajeError error={solicitudes.error} titulo="No se pudieron cargar los traslados" />

      {solicitudes.data && solicitudes.data.resultados.length === 0 && <Text c="dimmed">No hay traslados con estos filtros.</Text>}
      {solicitudes.data && solicitudes.data.resultados.length > 0 && (
        <>
          <Table.ScrollContainer minWidth={900}>
            <Table highlightOnHover>
              <Table.Thead>
                <Table.Tr>
                  <Table.Th>Franja</Table.Th>
                  <Table.Th>Cosa</Table.Th>
                  <Table.Th>Origen → destino</Table.Th>
                  <Table.Th>Referencia</Table.Th>
                  <Table.Th>Estado</Table.Th>
                </Table.Tr>
              </Table.Thead>
              <Table.Tbody>
                {solicitudes.data.resultados.map((s) => {
                  const abrir = () => navegar(`/traslados/${s.id}`)
                  return (
                    <Table.Tr
                      key={s.id}
                      onClick={abrir}
                      onKeyDown={(e) => e.key === 'Enter' && abrir()}
                      tabIndex={0}
                      style={{ cursor: 'pointer' }}
                    >
                      <Table.Td>{franja(s.traslado?.franja?.inicio, s.traslado?.franja?.fin)}</Table.Td>
                      <Table.Td>
                        {s.cosa.descripcion}
                        <Text size="xs" c="dimmed">
                          {s.cosa.cantidad} u.{s.cosa.peso_kg != null && ` · ${s.cosa.peso_kg} kg`}
                        </Text>
                      </Table.Td>
                      <Table.Td>
                        {s.origen.nombre} → {s.destino.nombre}
                      </Table.Td>
                      <Table.Td>{s.referencia_externa ?? '—'}</Table.Td>
                      <Table.Td>
                        <Insignias
                          estado={s.traslado?.estado}
                          alerta={s.traslado?.alerta_climatica?.estado}
                          motivos={s.traslado?.alerta_climatica?.motivos}
                          tardia={s.cancelacion?.tardia}
                        />
                      </Table.Td>
                    </Table.Tr>
                  )
                })}
              </Table.Tbody>
            </Table>
          </Table.ScrollContainer>
          {paginas > 1 && (
            <Pagination total={paginas} value={pagina} onChange={(p) => cambiar({ pagina: String(p) })} />
          )}
        </>
      )}
    </Stack>
  )
}
