import type { ReactNode } from 'react'
import { Center, Loader } from '@mantine/core'
import { Navigate, Route, Routes } from 'react-router'
import { Diseno } from './componentes/Diseno'
import { AdminBusqueda } from './paginas/AdminBusqueda'
import { Avisos } from './paginas/Avisos'
import { Buscar } from './paginas/Buscar'
import { DetalleTraslado } from './paginas/DetalleTraslado'
import { Franjas } from './paginas/Franjas'
import { Ingreso } from './paginas/Ingreso'
import { Traslados } from './paginas/Traslados'
import { pantallaInicial, tieneRol, useSesion } from './sesion'
import type { Rol } from './tipos'

/** Solo deja pasar a los roles indicados; si no, lleva a la pantalla inicial del usuario. */
function ConRol({ roles, children }: { roles: Rol[]; children: ReactNode }) {
  const { perfil } = useSesion()
  return tieneRol(perfil, ...roles) ? children : <Navigate to={pantallaInicial(perfil)} replace />
}

const TODOS: Rol[] = ['operador', 'administrador', 'cliente']
const PERSONAL: Rol[] = ['operador', 'administrador']

export function App() {
  const { perfil, cargando } = useSesion()

  if (cargando) {
    return (
      <Center mih="100vh">
        <Loader />
      </Center>
    )
  }

  if (!perfil) {
    return (
      <Routes>
        <Route path="/ingreso" element={<Ingreso />} />
        <Route path="*" element={<Navigate to="/ingreso" replace />} />
      </Routes>
    )
  }

  // La web no reserva franjas (ADR-021): no hay pantalla de reserva, solo de gestión.
  return (
    <Routes>
      <Route element={<Diseno />}>
        <Route path="/franjas" element={<ConRol roles={PERSONAL}><Franjas /></ConRol>} />
        <Route path="/traslados" element={<ConRol roles={TODOS}><Traslados /></ConRol>} />
        <Route path="/traslados/:id" element={<ConRol roles={TODOS}><DetalleTraslado /></ConRol>} />
        <Route path="/buscar" element={<ConRol roles={TODOS}><Buscar /></ConRol>} />
        <Route path="/avisos" element={<ConRol roles={TODOS}><Avisos /></ConRol>} />
        <Route path="/admin/busqueda" element={<ConRol roles={['administrador']}><AdminBusqueda /></ConRol>} />
      </Route>
      <Route path="*" element={<Navigate to={pantallaInicial(perfil)} replace />} />
    </Routes>
  )
}
