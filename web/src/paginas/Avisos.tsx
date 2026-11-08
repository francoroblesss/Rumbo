import { useState } from 'react'
import { Anchor, Badge, Button, Card, Group, Loader, Pagination, Stack, Switch, Text, Title } from '@mantine/core'
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router'
import { listarAvisos, marcarLeido } from '../api'
import { MensajeError } from '../componentes/MensajeError'
import { fechaHora } from '../formato'

const TAMANIO = 20

/** /avisos: la bandeja de Notificaciones (§14), más nuevos primero. `leido` es compartido por el destinatario. */
export function Avisos() {
  const clienteConsultas = useQueryClient()
  const [soloNoLeidos, setSoloNoLeidos] = useState(false)
  const [pagina, setPagina] = useState(1)

  const filtros = { no_leidos: soloNoLeidos || undefined, pagina, tamanio: TAMANIO }
  const avisos = useQuery({
    queryKey: ['avisos', 'bandeja', filtros],
    queryFn: () => listarAvisos(filtros),
    placeholderData: keepPreviousData,
  })

  const lectura = useMutation({
    mutationFn: marcarLeido,
    onSuccess: () => clienteConsultas.invalidateQueries({ queryKey: ['avisos'] }),
  })

  const paginas = avisos.data ? Math.max(1, Math.ceil(avisos.data.total / TAMANIO)) : 1

  return (
    <Stack>
      <Group justify="space-between">
        <Title order={2}>Avisos</Title>
        <Switch
          label="Solo no leídos"
          checked={soloNoLeidos}
          onChange={(e) => {
            setSoloNoLeidos(e.currentTarget.checked)
            setPagina(1)
          }}
        />
      </Group>

      {avisos.isPending && <Loader />}
      <MensajeError error={avisos.error} titulo="No se pudieron cargar los avisos" />
      <MensajeError error={lectura.error} titulo="No se pudo marcar como leído" />

      {avisos.data && avisos.data.resultados.length === 0 && <Text c="dimmed">No hay avisos.</Text>}
      {avisos.data?.resultados.map((a) => (
        <Card key={a.id} withBorder>
          <Group justify="space-between" align="flex-start" wrap="nowrap">
            <Stack gap={4}>
              <Group gap="xs">
                <Text fw={600}>{a.titulo}</Text>
                {!a.leido && <Badge size="sm">Nuevo</Badge>}
              </Group>
              <Text size="sm">{a.mensaje}</Text>
              <Group gap="sm">
                <Text size="xs" c="dimmed">
                  {fechaHora(a.creado_en)}
                </Text>
                {a.entidad?.tipo === 'solicitud' && (
                  <Anchor component={Link} to={`/traslados/${a.entidad.id}`} size="xs">
                    Ver el traslado
                  </Anchor>
                )}
              </Group>
            </Stack>
            {!a.leido && (
              <Button
                size="xs"
                variant="light"
                onClick={() => lectura.mutate(a.id)}
                loading={lectura.isPending && lectura.variables === a.id}
              >
                Marcar como leído
              </Button>
            )}
          </Group>
        </Card>
      ))}
      {paginas > 1 && <Pagination total={paginas} value={pagina} onChange={setPagina} />}
    </Stack>
  )
}
