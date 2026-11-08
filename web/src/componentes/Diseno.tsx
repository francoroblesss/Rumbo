// Layout de la web: encabezado (Rumbo, usuario, organización, campana de avisos y "Salir") y menú
// lateral según el rol, colapsable en pantallas angostas. Acá se abre el stream de avisos: uno
// solo por sesión.

import { AppShell, Burger, Button, Group, Indicator, NavLink, Stack, Text, Title } from '@mantine/core'
import { useDisclosure } from '@mantine/hooks'
import { useQuery } from '@tanstack/react-query'
import { Link, Outlet, useLocation } from 'react-router'
import { listarAvisos } from '../api'
import { esPersonal, tieneRol, useSesion } from '../sesion'
import { useAvisosEnTiempoReal } from '../tiempoReal'

function Campana() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden="true">
      <path d="M6 8a6 6 0 1 1 12 0c0 7 3 9 3 9H3s3-2 3-9" />
      <path d="M10.3 21a1.94 1.94 0 0 0 3.4 0" />
    </svg>
  )
}

interface Seccion {
  ruta: string
  texto: string
}

export function Diseno() {
  const { perfil, salir } = useSesion()
  const [abierto, { toggle, close }] = useDisclosure()
  const ubicacion = useLocation()
  useAvisosEnTiempoReal()

  // Contador de la campana: total de avisos no leídos (un solo resultado, solo interesa el total).
  const noLeidos = useQuery({
    queryKey: ['avisos', 'no-leidos'],
    queryFn: () => listarAvisos({ no_leidos: true, pagina: 1, tamanio: 1 }),
  })
  const cantidad = noLeidos.data?.total ?? 0

  // Un usuario con varios roles ve todas sus secciones.
  const secciones: Seccion[] = []
  if (esPersonal(perfil)) secciones.push({ ruta: '/franjas', texto: 'Franjas' })
  secciones.push({ ruta: '/traslados', texto: 'Traslados' }, { ruta: '/buscar', texto: 'Buscar' }, { ruta: '/avisos', texto: 'Avisos' })
  if (tieneRol(perfil, 'administrador')) secciones.push({ ruta: '/admin/busqueda', texto: 'Índice de búsqueda' })

  return (
    <AppShell header={{ height: 60 }} navbar={{ width: 220, breakpoint: 'sm', collapsed: { mobile: !abierto } }} padding="md">
      <AppShell.Header>
        <Group h="100%" px="md" justify="space-between" wrap="nowrap">
          <Group gap="sm" wrap="nowrap">
            <Burger opened={abierto} onClick={toggle} hiddenFrom="sm" size="sm" aria-label="Abrir el menú" />
            <Title order={3}>Rumbo</Title>
          </Group>
          <Group gap="sm" wrap="nowrap">
            <Stack gap={0} visibleFrom="xs" align="flex-end">
              <Text size="sm" fw={600}>
                {perfil?.nombre || perfil?.email}
              </Text>
              {perfil?.organizacion && (
                <Text size="xs" c="dimmed">
                  {perfil.organizacion.nombre || perfil.organizacion.id}
                </Text>
              )}
            </Stack>
            <Indicator label={cantidad} size={18} disabled={cantidad === 0} offset={4}>
              <Button
                component={Link}
                to="/avisos"
                variant="subtle"
                leftSection={<Campana />}
                aria-label={`Avisos: ${cantidad} sin leer`}
              >
                Avisos
              </Button>
            </Indicator>
            <Button variant="default" onClick={salir}>
              Salir
            </Button>
          </Group>
        </Group>
      </AppShell.Header>

      <AppShell.Navbar p="sm" aria-label="Secciones">
        {secciones.map((s) => (
          <NavLink
            key={s.ruta}
            component={Link}
            to={s.ruta}
            label={s.texto}
            active={ubicacion.pathname === s.ruta || ubicacion.pathname.startsWith(`${s.ruta}/`)}
            onClick={close}
          />
        ))}
      </AppShell.Navbar>

      <AppShell.Main>
        <Outlet />
      </AppShell.Main>
    </AppShell>
  )
}
