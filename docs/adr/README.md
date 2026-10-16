# Registro de decisiones arquitectónicas (ADR)

Cada decisión se conserva en un archivo independiente `ADR-NNN-titulo-corto.md`. Mientras un ADR no fue commiteado, se puede corregir en el mismo archivo dejando una línea "Revisión <fecha>: motivo"; una vez commiteado, cualquier cambio de decisión se registra con un ADR nuevo. **Si una decisión cambia, no se borra su historia**: el ADR anterior se marca como reemplazado y el nuevo indica qué decisión reemplaza y por qué fue necesario revisarla (consigna §5).

## Decisiones obligatorias
| # | Decisión | Entrega | ADR |
| --- | --- | --- | --- |
| D1 | Límites de los servicios | E1 (validación en E2) | [ADR-001](ADR-001-limites-de-los-servicios.md), validado en [ADR-013](ADR-013-validacion-d1-d3.md) |
| D2 | Arquitectura interna de los servicios | E2 | [ADR-010](ADR-010-estilos-internos.md) |
| D3 | Persistencia | E1 inicial (validación en E2) | [ADR-003](ADR-003-persistencia.md), motor reemplazado por [ADR-005](ADR-005-mysql-como-motor-relacional.md), validado en [ADR-013](ADR-013-validacion-d1-d3.md) |
| D4 | Consistencia y concurrencia | — | — |
| D5 | Comunicación entre servicios | E1 inicial (validación en E2) | [ADR-004](ADR-004-comunicacion-entre-servicios.md), validado en [ADR-016](ADR-016-validacion-d5.md) |
| D6 | Búsqueda | E2 | [ADR-011](ADR-011-busqueda.md) |
| D7 | Caché | E2 | [ADR-007](ADR-007-cache.md) |
| D8 | Contrato propio | E1 (actualizar en E2 si cambió) | [ADR-002](ADR-002-contrato-propio.md), contrato reemplazado por [ADR-006](ADR-006-contrato-generico-v2.md) (v2) |
| D9 | Consumo del proveedor | E2 | [ADR-012](ADR-012-consumo-clima.md) |
| D10 | Resiliencia | E2 | [ADR-008](ADR-008-resiliencia.md), reconexión reemplazada por [ADR-017](ADR-017-reconexion-broker-y-balanceador.md) |
| D11 | Observabilidad | E2 primera versión | [ADR-014](ADR-014-observabilidad.md) |
| D12 | Balanceo de carga | E2 | [ADR-009](ADR-009-balanceo.md) |
| D13 | Capacidad y costos | E2 | [ADR-015](ADR-015-capacidad-y-costos.md) |

## Otras decisiones
| ADR | Decisión |
| --- | --- |
| [ADR-005](ADR-005-mysql-como-motor-relacional.md) | MySQL como motor relacional (reemplaza parcialmente a ADR-003) |
| [ADR-006](ADR-006-contrato-generico-v2.md) | Contrato genérico "Solicitar traslado" v2 (reemplaza parcialmente a ADR-002) |
| [ADR-017](ADR-017-reconexion-broker-y-balanceador.md) | Reconexión al broker sin terminar el proceso y resolución dinámica de réplicas en NGINX (reemplaza parcialmente a ADR-008) |

## Formato
```
# ADR-NNN — Título  (Dn si es obligatoria)
- Fecha:
- Estado: propuesta | aceptada | reemplazada por ADR-XXX (reemplaza a ADR-YYY: motivo)
- Contexto: qué problema concreto hay.
- Decisión: qué hacemos.
- Alternativas consideradas: y por qué no.
- Consecuencias / trade-offs: qué ganamos y qué pagamos.
- Cómo se verifica: prueba, métrica o demo.
```
