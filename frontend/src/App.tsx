import { useEffect, useState } from 'react'
import { alExpirar, cerrarSesion, tokenGuardado, usuarioConectado } from './api.ts'
import Login from './Login.tsx'
import Solicitudes from './Solicitudes.tsx'

// App decide entre el ingreso y la consola del operador según haya un token en la sesión.
export default function App() {
  const [conectado, setConectado] = useState(() => tokenGuardado() !== null)
  const [aviso, setAviso] = useState('')

  useEffect(() => {
    alExpirar(() => {
      setAviso('La sesión venció. Volvé a ingresar.')
      setConectado(false)
    })
  }, [])

  if (!conectado) {
    return (
      <Login
        aviso={aviso}
        alIngresar={() => {
          setAviso('')
          setConectado(true)
        }}
      />
    )
  }

  const usuario = usuarioConectado()
  return (
    <div className="app">
      <header className="barra">
        <div className="marca">
          <span className="logo" aria-hidden="true">R</span>
          <span>Rumbo</span>
          <span className="seccion">Solicitudes</span>
        </div>
        <div className="usuario">
          <span title={usuario.roles.join(', ')}>{usuario.email || 'Sesión iniciada'}</span>
          <button
            className="boton secundario"
            onClick={() => {
              cerrarSesion()
              setConectado(false)
            }}
          >
            Salir
          </button>
        </div>
      </header>
      <Solicitudes />
    </div>
  )
}
