# Clima del aeropuerto en la web (ADR-034)

Capturas de la página de traslados como operador, con el sistema levantado con `docker compose up --build -d --wait` (2026-11-20).

| Captura | Qué muestra |
| --- | --- |
| `operable.png` | Mock del clima con el ejemplo `operable` del contrato: ícono de sol, "Sin fenómenos adversos", "Aeropuerto operable", 22,4 °C, viento 18 km/h con ráfagas de 27,5 km/h (flecha hacia el SSE: el viento viene de 340°), 10 km de visibilidad, 0 mm y las dos barras en verde. |
| `no-operable.png` | `CLIMA_PREFER=example=not_operable docker compose up -d operaciones`: ícono de viento, "Aeropuerto no operable" con "Viento fuerte, Ráfagas fuertes", 6,1 °C, 58,3 km/h con ráfagas de 81 km/h (flecha hacia el este: viene de 270°), 1,2 mm y las barras en rojo (58,3 de 50 y 81 de 65). |
| `no-disponible.png` | `docker compose stop clima-mock` y Operaciones reiniciado (caché vacía): la tarjeta dice "Clima no disponible" y la tabla de traslados sigue funcionando. `GET /api/operaciones/clima` → `503 SERVICIO_NO_DISPONIBLE` con `Retry-After: 60`, y Operaciones loguea `WARN "reintento al clima"` y `WARN "clima no disponible para la web"`. |

Como cliente se ve la misma tarjeta (verificado el mismo día, sin captura).
