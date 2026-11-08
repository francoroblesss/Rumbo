import { Alert, List } from '@mantine/core'
import { ErrorApi } from '../api'

/** Muestra un error de la API: el `detail` de Problem Details y, si vinieron, los errores por campo. */
export function MensajeError({ error, titulo = 'No se pudo completar' }: { error: unknown; titulo?: string }) {
  if (!error) return null
  const mensaje = error instanceof Error ? error.message : 'Error inesperado.'
  const errores = error instanceof ErrorApi ? error.errores : []
  return (
    <Alert color="red" title={titulo} role="alert">
      {mensaje}
      {errores.length > 0 && (
        <List size="sm" mt="xs">
          {errores.map((e) => (
            <List.Item key={`${e.campo}-${e.mensaje}`}>
              {e.campo}: {e.mensaje}
            </List.Item>
          ))}
        </List>
      )}
    </Alert>
  )
}
