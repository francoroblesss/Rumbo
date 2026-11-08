import { Alert, Button, Stack, Text, Title } from '@mantine/core'
import { useMutation } from '@tanstack/react-query'
import { reconstruirIndice } from '../api'
import { MensajeError } from '../componentes/MensajeError'

/** /admin/busqueda (solo administrador): reconstruye el índice de Solr desde MySQL (§16). */
export function AdminBusqueda() {
  const reindexacion = useMutation({ mutationFn: reconstruirIndice })

  return (
    <Stack maw={600}>
      <Title order={2}>Índice de búsqueda</Title>
      <Text>
        Vuelve a indexar todos los traslados a partir de los datos de Operaciones. Corre en segundo plano: la búsqueda
        sigue respondiendo mientras tanto.
      </Text>
      <div>
        <Button onClick={() => reindexacion.mutate()} loading={reindexacion.isPending}>
          Reconstruir índice de búsqueda
        </Button>
      </div>
      {reindexacion.isSuccess && (
        <Alert color="green" title="Listo">
          Reindexación en curso
        </Alert>
      )}
      <MensajeError error={reindexacion.error} titulo="No se pudo reconstruir el índice" />
    </Stack>
  )
}
