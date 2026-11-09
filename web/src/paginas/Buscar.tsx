import { useState, type FormEvent } from 'react'
import {
  Alert,
  Anchor,
  Button,
  Checkbox,
  Group,
  Loader,
  Pagination,
  SegmentedControl,
  Select,
  Stack,
  Table,
  Text,
  TextInput,
  Title,
} from '@mantine/core'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { Link } from 'react-router'
import { ErrorApi, buscarTraslados, type FiltrosBusqueda } from '../api'
import { Insignias } from '../componentes/Insignias'
import { MensajeError } from '../componentes/MensajeError'
import { ESTADOS_TRASLADO, franja } from '../formato'

const TAMANIO = 20

type Formulario = Omit<FiltrosBusqueda, 'pagina' | 'tamanio'>

const VACIO: Formulario = { q: '', estado: '', desde: '', hasta: '', alerta: false, orden: 'fecha' }

/** /buscar: búsqueda de traslados en Solr (§16). Al cliente, Operaciones le fuerza su organización. */
export function Buscar() {
  const [formulario, setFormulario] = useState<Formulario>(VACIO)
  const [buscado, setBuscado] = useState<Formulario | null>(null)
  const [pagina, setPagina] = useState(1)

  const filtros: FiltrosBusqueda | null = buscado ? { ...buscado, pagina, tamanio: TAMANIO } : null
  const resultados = useQuery({
    queryKey: ['busqueda', filtros],
    queryFn: () => buscarTraslados(filtros!),
    enabled: filtros !== null,
    placeholderData: keepPreviousData,
  })

  function buscar(evento: FormEvent) {
    evento.preventDefault()
    // Con los mismos filtros la consulta no cambia y React Query devolvería lo guardado (por
    // ejemplo, el 503 de cuando Solr estaba caído): se vuelve a pedir.
    const mismaConsulta = pagina === 1 && JSON.stringify(buscado) === JSON.stringify(formulario)
    setBuscado({ ...formulario })
    setPagina(1)
    if (mismaConsulta) void resultados.refetch()
  }

  const cambiar = <K extends keyof Formulario>(campo: K, valor: Formulario[K]) =>
    setFormulario((f) => ({ ...f, [campo]: valor }))

  // Sin Solr, la búsqueda responde 503 (o 404 si la ruta todavía no existe): el resto de la web sigue.
  const sinBusqueda =
    resultados.error instanceof ErrorApi && (resultados.error.estado === 503 || resultados.error.estado === 404)
  const paginas = resultados.data ? Math.max(1, Math.ceil(resultados.data.total / TAMANIO)) : 1

  return (
    <Stack>
      <Title order={2}>Buscar traslados</Title>
      <form onSubmit={buscar}>
        <Stack>
          <TextInput
            label="Texto"
            description="Descripción, origen, destino o referencia externa"
            value={formulario.q}
            onChange={(e) => cambiar('q', e.currentTarget.value)}
          />
          <Group align="flex-end">
            <Select
              label="Estado"
              placeholder="Todos"
              data={ESTADOS_TRASLADO}
              value={formulario.estado || null}
              onChange={(v) => cambiar('estado', v ?? '')}
              clearable
            />
            <TextInput type="date" label="Desde" value={formulario.desde} onChange={(e) => cambiar('desde', e.currentTarget.value)} />
            <TextInput type="date" label="Hasta" value={formulario.hasta} onChange={(e) => cambiar('hasta', e.currentTarget.value)} />
            <Stack gap={4}>
              <Text size="sm" fw={500} id="etiqueta-orden">
                Orden
              </Text>
              <SegmentedControl
                aria-labelledby="etiqueta-orden"
                value={formulario.orden}
                onChange={(v) => cambiar('orden', v as Formulario['orden'])}
                data={[
                  { value: 'fecha', label: 'Fecha' },
                  { value: 'relevancia', label: 'Relevancia' },
                ]}
              />
            </Stack>
          </Group>
          <Checkbox
            label="Solo con alerta climática"
            checked={formulario.alerta}
            onChange={(e) => cambiar('alerta', e.currentTarget.checked)}
          />
          <Group>
            <Button type="submit">Buscar</Button>
          </Group>
        </Stack>
      </form>

      {resultados.isFetching && !resultados.data && <Loader />}
      {sinBusqueda ? (
        <Alert color="yellow" title="Búsqueda no disponible">
          La búsqueda no está disponible en este momento
        </Alert>
      ) : (
        <MensajeError error={resultados.error} titulo="No se pudo buscar" />
      )}

      {resultados.data && resultados.data.resultados.length === 0 && <Text c="dimmed">No se encontraron traslados.</Text>}
      {resultados.data && resultados.data.resultados.length > 0 && (
        <>
          <Text size="sm" c="dimmed">
            {resultados.data.total} resultado{resultados.data.total === 1 ? '' : 's'}
          </Text>
          <Table.ScrollContainer minWidth={900}>
            <Table highlightOnHover>
              <Table.Thead>
                <Table.Tr>
                  <Table.Th>Franja</Table.Th>
                  <Table.Th>Descripción</Table.Th>
                  <Table.Th>Origen → destino</Table.Th>
                  <Table.Th>Referencia</Table.Th>
                  <Table.Th>Estado</Table.Th>
                </Table.Tr>
              </Table.Thead>
              <Table.Tbody>
                {resultados.data.resultados.map((r) => (
                  <Table.Tr key={r.traslado_id || r.solicitud_id}>
                    <Table.Td>{franja(r.franja_inicio, r.franja_fin)}</Table.Td>
                    <Table.Td>
                      <Anchor component={Link} to={`/traslados/${r.solicitud_id}`}>
                        {r.descripcion}
                      </Anchor>
                    </Table.Td>
                    <Table.Td>
                      {r.origen_nombre} → {r.destino_nombre}
                    </Table.Td>
                    <Table.Td>{r.referencia_externa ?? '—'}</Table.Td>
                    <Table.Td>
                      <Insignias estado={r.estado_traslado} alerta={r.alerta_climatica} tardia={r.cancelacion_tardia} />
                    </Table.Td>
                  </Table.Tr>
                ))}
              </Table.Tbody>
            </Table>
          </Table.ScrollContainer>
          {paginas > 1 && <Pagination total={paginas} value={pagina} onChange={setPagina} />}
        </>
      )}
    </Stack>
  )
}
