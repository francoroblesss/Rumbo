import { useRef, useState, type ReactNode } from 'react'
import {
  Alert,
  Anchor,
  Button,
  Card,
  Group,
  Loader,
  Modal,
  SimpleGrid,
  Stack,
  Text,
  Textarea,
  Title,
} from '@mantine/core'
import { notifications } from '@mantine/notifications'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useParams } from 'react-router'
import { avanzarEstado, intencionDeCancelar, obtenerSolicitud, type IntencionCancelacion } from '../api'
import { AlertaClima, CancelacionTardia, EstadoTraslado } from '../componentes/Insignias'
import { MensajeError } from '../componentes/MensajeError'
import { fechaHora, franja } from '../formato'
import { esPersonal, useSesion } from '../sesion'
import type { AlertaClimatica, VistaSolicitud } from '../tipos'

function Dato({ etiqueta, children }: { etiqueta: string; children: ReactNode }) {
  return (
    <div>
      <Text size="xs" c="dimmed">
        {etiqueta}
      </Text>
      <Text component="div">{children}</Text>
    </div>
  )
}

function mismaAlerta(a: AlertaClimatica | null | undefined, b: AlertaClimatica | null | undefined): boolean {
  if (!a || !b) return !a && !b
  return a.estado === b.estado && (a.motivos ?? []).join(',') === (b.motivos ?? []).join(',')
}

/** /traslados/:id: la vista completa (§6), avance de estado (personal) y cancelación (todos). */
export function DetalleTraslado() {
  const { id = '' } = useParams()
  const { perfil } = useSesion()
  const clienteConsultas = useQueryClient()
  const solicitud = useQuery({ queryKey: ['solicitud', id], queryFn: () => obtenerSolicitud(id) })

  // Tras una escritura: se guarda la vista que devolvió la API y se refrescan los listados.
  function actualizar(vista: VistaSolicitud) {
    clienteConsultas.setQueryData(['solicitud', id], vista)
    void clienteConsultas.invalidateQueries({ queryKey: ['solicitudes'] })
    void clienteConsultas.invalidateQueries({ queryKey: ['franjas'] })
    void clienteConsultas.invalidateQueries({ queryKey: ['busqueda'] })
  }

  const avance = useMutation({
    mutationFn: (estado: 'EN_CURSO' | 'COMPLETADO') => avanzarEstado(id, estado),
    onSuccess: (vista) => {
      const anterior = solicitud.data?.traslado?.alerta_climatica
      const nueva = vista.traslado?.alerta_climatica
      if (nueva && !mismaAlerta(anterior, nueva)) {
        notifications.show({
          color: nueva.estado === 'NO_OPERABLE' ? 'red' : 'yellow',
          title: 'Alerta climática',
          message:
            nueva.estado === 'NO_OPERABLE'
              ? `El aeropuerto no está operable (${(nueva.motivos ?? []).join(', ')}).`
              : 'No se pudo verificar el clima del aeropuerto.',
        })
      }
      actualizar(vista)
    },
  })

  // Cancelación: la intención (y su Idempotency-Key) se crea al abrir el modal; reintentar desde
  // el mismo modal reusa la clave. Cerrarlo y volver a abrirlo es una cancelación nueva.
  const intencion = useRef<IntencionCancelacion | null>(null)
  const [modalAbierto, setModalAbierto] = useState(false)
  const [motivo, setMotivo] = useState('')
  const cancelacion = useMutation({
    mutationFn: (m: string) => intencion.current!.ejecutar(m),
    onSuccess: (vista) => {
      setModalAbierto(false)
      notifications.show({
        color: vista.cancelacion?.tardia ? 'orange' : 'green',
        title: 'Traslado cancelado',
        message: vista.cancelacion?.tardia
          ? 'La cancelación quedó registrada como tardía: faltaban menos de 24 h.'
          : 'Se canceló el traslado y se liberó su lugar en la franja.',
      })
      actualizar(vista)
    },
  })

  function abrirCancelacion() {
    intencion.current = intencionDeCancelar(id)
    setMotivo('')
    cancelacion.reset()
    setModalAbierto(true)
  }

  if (solicitud.isPending) return <Loader />
  if (solicitud.error) return <MensajeError error={solicitud.error} titulo="No se pudo cargar el traslado" />
  const s = solicitud.data
  const t = s.traslado
  const personal = esPersonal(perfil)

  return (
    <Stack>
      <Anchor component={Link} to="/traslados" size="sm">
        ← Volver a los traslados
      </Anchor>
      <Group justify="space-between" align="flex-start">
        <div>
          <Title order={2}>{s.cosa.descripcion}</Title>
          <Text c="dimmed" size="sm">
            {s.id}
            {t && ` · ${t.id}`}
          </Text>
        </div>
        <Group gap={4}>
          <EstadoTraslado estado={t?.estado} />
          <AlertaClima estado={t?.alerta_climatica?.estado} motivos={t?.alerta_climatica?.motivos} />
          <CancelacionTardia tardia={s.cancelacion?.tardia} />
        </Group>
      </Group>

      {t?.alerta_climatica?.estado === 'NO_OPERABLE' && (
        <Alert color="red" title="Aeropuerto no operable">
          Motivos: {(t.alerta_climatica.motivos ?? []).join(', ') || 'sin motivos informados'}.
          {t.alerta_climatica.observado_en && ` Observado el ${fechaHora(t.alerta_climatica.observado_en)}.`}
        </Alert>
      )}
      {t?.alerta_climatica?.estado === 'NO_VERIFICADO' && (
        <Alert color="yellow" title="Clima no verificado">
          No se pudo verificar el clima del aeropuerto.
        </Alert>
      )}

      <Group>
        {personal && t?.estado === 'PROGRAMADO' && (
          <Button onClick={() => avance.mutate('EN_CURSO')} loading={avance.isPending}>
            Marcar en curso
          </Button>
        )}
        {personal && t?.estado === 'EN_CURSO' && (
          <Button color="green" onClick={() => avance.mutate('COMPLETADO')} loading={avance.isPending}>
            Marcar completado
          </Button>
        )}
        {t?.estado === 'PROGRAMADO' && (
          <Button color="red" variant="light" onClick={abrirCancelacion}>
            Cancelar traslado
          </Button>
        )}
      </Group>
      <MensajeError error={avance.error} titulo="No se pudo cambiar el estado" />

      <SimpleGrid cols={{ base: 1, sm: 2 }}>
        <Card withBorder>
          <Stack gap="sm">
            <Title order={4}>Traslado</Title>
            <Dato etiqueta="Franja">{franja(t?.franja?.inicio, t?.franja?.fin)}</Dato>
            <Dato etiqueta="Llegada comprometida">{fechaHora(t?.hora_llegada_comprometida)}</Dato>
            {t?.finalizado_en && <Dato etiqueta="Finalizado">{fechaHora(t.finalizado_en)}</Dato>}
            {t?.demora_minutos != null && <Dato etiqueta="Demora">{t.demora_minutos} min</Dato>}
            {t?.detalle_resultado && <Dato etiqueta="Resultado">{t.detalle_resultado}</Dato>}
            <Dato etiqueta="Referencia externa">{s.referencia_externa ?? '—'}</Dato>
          </Stack>
        </Card>
        <Card withBorder>
          <Stack gap="sm">
            <Title order={4}>Cosa</Title>
            <Dato etiqueta="Descripción">{s.cosa.descripcion}</Dato>
            <Dato etiqueta="Cantidad">{s.cosa.cantidad}</Dato>
            <Dato etiqueta="Peso">{s.cosa.peso_kg != null ? `${s.cosa.peso_kg} kg` : '—'}</Dato>
            {s.observaciones && <Dato etiqueta="Observaciones">{s.observaciones}</Dato>}
          </Stack>
        </Card>
        <Card withBorder>
          <Stack gap="sm">
            <Title order={4}>Origen</Title>
            <Dato etiqueta="Nombre">{s.origen.nombre}</Dato>
            <Dato etiqueta="Dirección">{s.origen.direccion}</Dato>
          </Stack>
        </Card>
        <Card withBorder>
          <Stack gap="sm">
            <Title order={4}>Destino</Title>
            <Dato etiqueta="Nombre">{s.destino.nombre}</Dato>
            <Dato etiqueta="Dirección">{s.destino.direccion}</Dato>
          </Stack>
        </Card>
        {s.cancelacion && (
          <Card withBorder>
            <Stack gap="sm">
              <Title order={4}>Cancelación</Title>
              <Dato etiqueta="Motivo">{s.cancelacion.motivo}</Dato>
              <Dato etiqueta="Pedida el">{fechaHora(s.cancelacion.solicitada_en)}</Dato>
              <Dato etiqueta="Tardía">{s.cancelacion.tardia ? 'Sí (menos de 24 h antes)' : 'No'}</Dato>
            </Stack>
          </Card>
        )}
        <Card withBorder>
          <Stack gap="sm">
            <Title order={4}>Registro</Title>
            <Dato etiqueta="Creada">{fechaHora(s.creado_en)}</Dato>
            <Dato etiqueta="Actualizada">{fechaHora(s.actualizado_en)}</Dato>
            <Dato etiqueta="Versión">{s.version}</Dato>
          </Stack>
        </Card>
      </SimpleGrid>

      <Modal opened={modalAbierto} onClose={() => setModalAbierto(false)} title="Cancelar traslado">
        <form
          onSubmit={(e) => {
            e.preventDefault()
            cancelacion.mutate(motivo.trim())
          }}
        >
          <Stack>
            <Textarea
              label="Motivo"
              required
              autosize
              minRows={3}
              value={motivo}
              onChange={(e) => setMotivo(e.currentTarget.value)}
            />
            <MensajeError error={cancelacion.error} titulo="No se pudo cancelar" />
            <Group justify="flex-end">
              <Button variant="default" onClick={() => setModalAbierto(false)}>
                Volver
              </Button>
              <Button type="submit" color="red" loading={cancelacion.isPending} disabled={motivo.trim() === ''}>
                Confirmar cancelación
              </Button>
            </Group>
          </Stack>
        </form>
      </Modal>
    </Stack>
  )
}
