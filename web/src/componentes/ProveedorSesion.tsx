import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { configurarApi, ingresar, obtenerPerfil } from '../api'
import { ContextoSesion, guardarToken, leerToken, type Sesion } from '../sesion'
import type { Perfil } from '../tipos'

/** Mantiene el token y el perfil, y conecta el cliente HTTP con la sesión. */
export function ProveedorSesion({ children }: { children: ReactNode }) {
  const clienteConsultas = useQueryClient()
  const [token, setToken] = useState<string | null>(leerToken)
  const [perfil, setPerfil] = useState<Perfil | null>(null)
  const [cargando, setCargando] = useState<boolean>(() => leerToken() !== null)

  // "Salir" borra la sesión; los datos de un usuario no quedan en caché para el siguiente.
  const salir = useCallback(() => {
    guardarToken(null)
    setToken(null)
    setPerfil(null)
    setCargando(false)
    clienteConsultas.clear()
  }, [clienteConsultas])

  // El cliente HTTP lee el token de acá; ante un 401 se sale de la sesión y el ruteo lleva al ingreso.
  useEffect(() => {
    configurarApi({ obtenerToken: leerToken, alNoAutenticado: salir })
  }, [salir])

  // Al recargar la página con una sesión guardada, se vuelve a pedir el perfil.
  useEffect(() => {
    if (!token || perfil) return
    let vigente = true
    obtenerPerfil()
      .then((p) => {
        if (vigente) setPerfil(p)
      })
      .catch(() => {
        if (vigente) salir()
      })
      .finally(() => {
        if (vigente) setCargando(false)
      })
    return () => {
      vigente = false
    }
  }, [token, perfil, salir])

  // Ingreso: login → token en sessionStorage → perfil (/yo).
  const entrar = useCallback(async (email: string, password: string) => {
    const respuesta = await ingresar(email, password)
    guardarToken(respuesta.access_token)
    try {
      const nuevoPerfil = await obtenerPerfil()
      setPerfil(nuevoPerfil)
      setToken(respuesta.access_token)
      setCargando(false)
      return nuevoPerfil
    } catch (error) {
      guardarToken(null)
      throw error
    }
  }, [])

  const valor = useMemo<Sesion>(
    () => ({ token, perfil, cargando, ingresar: entrar, salir }),
    [token, perfil, cargando, entrar, salir],
  )
  return <ContextoSesion.Provider value={valor}>{children}</ContextoSesion.Provider>
}
