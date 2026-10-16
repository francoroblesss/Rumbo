import { useEffect, useState } from 'react'
import { ErrorApi, listarSolicitudes } from './api.ts'
import Detalle from './Detalle.tsx'
import { ESTADOS, etiqueta, fecha, type PaginaSolicitudes } from './tipos.ts'

const TAMANIO = 20
// Los cambios llegan con poca frecuencia: alcanza con consultar cada 15 s (teórico 9: polling).
// Se pausa con la pestaña oculta para no gastar pedidos del límite del gateway (60 por minuto).
const INTERVALO_MS = 15_000

export default function Solicitudes() {
  const [estado, setEstado] = useState('')
  const [pagina, setPagina] = useState(1)
  const [datos, setDatos] = useState<PaginaSolicitudes | null>(null)
  const [error, setError] = useState('')
  const [seleccionada, setSeleccionada] = useState<string | null>(null)
  const [refresco, setRefresco] = useState(0)

  useEffect(() => {
    let vigente = true
    listarSolicitudes(estado, pagina, TAMANIO)
      .then((d) => {
        if (!vigente) return
        setDatos(d)
        setError('')
      })
      .catch((err: unknown) => {
        if (vigente) setError(err instanceof ErrorApi ? err.message : 'No se pudo cargar la lista.')
      })
    return () => {
      vigente = false
    }
  }, [estado, pagina, refresco])

  useEffect(() => {
    const t = setInterval(() => {
      if (!document.hidden) setRefresco((n) => n + 1)
    }, INTERVALO_MS)
    return () => clearInterval(t)
  }, [])

  const paginas = datos ? Math.max(1, Math.ceil(datos.total / TAMANIO)) : 1

  return (
    <main className="consola">
      <section className="tarjeta lista">
        <div className="lista-cabecera">
          <h2>Solicitudes</h2>
          <div className="controles">
            <select
              aria-label="Filtrar por estado"
              value={estado}
              onChange={(e) => {
                setEstado(e.target.value)
                setPagina(1)
              }}
            >
              {ESTADOS.map((e) => (
                <option key={e.valor} value={e.valor}>
                  {e.etiqueta}
                </option>
              ))}
            </select>
            <button className="boton secundario" onClick={() => setRefresco((n) => n + 1)}>
              Actualizar
            </button>
          </div>
        </div>

        {error && <p className="alerta error">{error}</p>}

        {datos && datos.resultados.length === 0 && <p className="vacio">No hay solicitudes con este filtro.</p>}

        <ul className="filas">
          {datos?.resultados.map((s) => (
            <li key={s.id}>
              <button className={`fila${s.id === seleccionada ? ' activa' : ''}`} onClick={() => setSeleccionada(s.id)}>
                <span className="fila-principal">
                  <strong>{s.cosa.descripcion}</strong>
                  <span className="tenue">
                    {s.origen.nombre} → {s.destino.nombre}
                  </span>
                </span>
                <span className="fila-lateral">
                  <span className={`estado e-${s.estado.toLowerCase()}`}>{etiqueta(s.estado)}</span>
                  <span className="tenue chico">{fecha(s.creado_en)}</span>
                </span>
              </button>
            </li>
          ))}
        </ul>

        {datos && datos.total > TAMANIO && (
          <div className="paginacion">
            <button className="boton secundario" disabled={pagina <= 1} onClick={() => setPagina((p) => p - 1)}>
              Anterior
            </button>
            <span className="tenue">
              Página {pagina} de {paginas} · {datos.total} solicitudes
            </span>
            <button className="boton secundario" disabled={pagina >= paginas} onClick={() => setPagina((p) => p + 1)}>
              Siguiente
            </button>
          </div>
        )}
      </section>

      <section className="tarjeta detalle">
        {seleccionada ? (
          // key: al cambiar de solicitud, el detalle arranca con formularios y claves nuevos.
          <Detalle key={seleccionada} id={seleccionada} refresco={refresco} alCambiar={() => setRefresco((n) => n + 1)} />
        ) : (
          <p className="vacio">Elegí una solicitud para ver el detalle y resolverla.</p>
        )}
      </section>
    </main>
  )
}
