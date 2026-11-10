import { describe, expect, it } from 'vitest'
import { QueryClient } from '@tanstack/react-query'
import { esperaReconexion, reconciliarAlReconectar } from './tiempoReal'

describe('reconciliarAlReconectar', () => {
  it('invalida los avisos y todo lo que muestra traslados, sin tocar el resto', () => {
    const cliente = new QueryClient()
    const claves = [
      ['avisos', 'no-leidos'],
      ['solicitudes', { fecha: '2026-11-10' }],
      ['franjas', '2026-11-10'],
      ['busqueda', 'caja'],
      ['solicitud', 'sol-1'],
      ['yo'],
    ]
    for (const clave of claves) cliente.setQueryData(clave, { cualquier: 'dato' })

    reconciliarAlReconectar(cliente)

    const invalidada = (clave: unknown[]) => cliente.getQueryState(clave)?.isInvalidated
    expect(claves.slice(0, 5).map(invalidada)).toEqual([true, true, true, true, true])
    expect(invalidada(['yo'])).toBe(false) // el perfil no depende de los avisos
  })
})

describe('esperaReconexion', () => {
  it('crece 1, 2, 4… segundos hasta un tope de 30', () => {
    expect([0, 1, 2, 3, 4, 5, 9].map(esperaReconexion)).toEqual([1000, 2000, 4000, 8000, 16000, 30000, 30000])
  })
})
