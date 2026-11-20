import { describe, expect, it } from 'vitest'
import { condicionClima } from './formato'
import type { ClimaAeropuerto } from './tipos'

// El ejemplo "operable" del contrato de clima; cada caso cambia lo que necesita.
function clima(cambios: { motivos?: string[]; tormenta?: boolean; precipitacion_mm?: number } = {}): ClimaAeropuerto {
  return {
    aeropuerto: { icao: 'SACO', nombre: 'Aeropuerto Internacional Ingeniero Aeronáutico Ambrosio Taravella' },
    observado_en: '2026-10-09T11:00:00-03:00',
    operable: (cambios.motivos ?? []).length === 0,
    motivos: cambios.motivos ?? [],
    condiciones: {
      temperatura_c: 22.4,
      viento_kmh: 18,
      rafagas_kmh: 27.5,
      direccion_viento_grados: 340,
      visibilidad_m: 10000,
      precipitacion_mm: cambios.precipitacion_mm ?? 0,
      tormenta: cambios.tormenta ?? false,
    },
  }
}

describe('condicionClima', () => {
  it('tormenta por el dato o por el motivo', () => {
    expect(condicionClima(clima({ tormenta: true }))).toBe('TORMENTA')
    expect(condicionClima(clima({ motivos: ['THUNDERSTORM'] }))).toBe('TORMENTA')
  })
  it('viento por viento o por ráfagas', () => {
    expect(condicionClima(clima({ motivos: ['HIGH_WIND'] }))).toBe('VIENTO')
    expect(condicionClima(clima({ motivos: ['HIGH_GUSTS'] }))).toBe('VIENTO')
  })
  it('baja visibilidad', () => {
    expect(condicionClima(clima({ motivos: ['LOW_VISIBILITY'] }))).toBe('BAJA_VISIBILIDAD')
  })
  it('lluvia si hay precipitación', () => {
    expect(condicionClima(clima({ precipitacion_mm: 1.2 }))).toBe('LLUVIA')
  })
  it('sin fenómenos adversos', () => {
    expect(condicionClima(clima())).toBe('BUENO')
  })
  it('la tormenta gana al viento', () => {
    expect(condicionClima(clima({ tormenta: true, motivos: ['HIGH_WIND', 'HIGH_GUSTS'] }))).toBe('TORMENTA')
  })
  it('el viento gana a la lluvia', () => {
    expect(condicionClima(clima({ motivos: ['HIGH_WIND'], precipitacion_mm: 1.2 }))).toBe('VIENTO')
  })
})
