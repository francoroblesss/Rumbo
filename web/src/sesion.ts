// Sesión de la web (ADR-029): el token va en sessionStorage (se borra al cerrar la pestaña y vence
// en 1 h) y el perfil sale de GET /api/identidad/yo. La web nunca manda la identidad: el gateway
// valida el token y pone los headers X-Rumbo-* (teórico 6). El proveedor está en
// componentes/ProveedorSesion.tsx.

import { createContext, useContext } from 'react'
import type { Perfil, Rol } from './tipos'

const CLAVE_TOKEN = 'rumbo.token'

export function leerToken(): string | null {
  try {
    return sessionStorage.getItem(CLAVE_TOKEN)
  } catch {
    return null
  }
}

export function guardarToken(token: string | null): void {
  try {
    if (token) sessionStorage.setItem(CLAVE_TOKEN, token)
    else sessionStorage.removeItem(CLAVE_TOKEN)
  } catch {
    // Sin sessionStorage disponible: la sesión no sobrevive a una recarga.
  }
}

export interface Sesion {
  token: string | null
  perfil: Perfil | null
  /** true mientras se recupera el perfil de una sesión guardada. */
  cargando: boolean
  ingresar: (email: string, password: string) => Promise<Perfil>
  salir: () => void
}

export const ContextoSesion = createContext<Sesion | null>(null)

export function useSesion(): Sesion {
  const sesion = useContext(ContextoSesion)
  if (!sesion) throw new Error('useSesion fuera de ProveedorSesion')
  return sesion
}

// ---------------- Roles ----------------
// Solo deciden qué se muestra. La autorización real la hace cada servicio con la identidad del
// gateway (autorización distribuida): el cliente ve solo lo suyo aunque cambie la URL.

export function tieneRol(perfil: Perfil | null, ...roles: Rol[]): boolean {
  return !!perfil && perfil.roles.some((r) => roles.includes(r))
}

/** Operador o administrador: ven todas las organizaciones (§11.1). */
export function esPersonal(perfil: Perfil | null): boolean {
  return tieneRol(perfil, 'operador', 'administrador')
}

/** Pantalla inicial según el rol: operador y administrador → /franjas; cliente → /traslados. */
export function pantallaInicial(perfil: Perfil | null): string {
  return esPersonal(perfil) ? '/franjas' : '/traslados'
}
