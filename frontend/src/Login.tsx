import { useState, type FormEvent } from 'react'
import { ErrorApi, iniciarSesion } from './api.ts'

interface Props {
  aviso: string
  alIngresar: () => void
}

export default function Login({ aviso, alIngresar }: Props) {
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [enviando, setEnviando] = useState(false)

  async function enviar(e: FormEvent) {
    e.preventDefault()
    setError('')
    setEnviando(true)
    try {
      await iniciarSesion(email.trim(), password)
      alIngresar()
    } catch (err) {
      // Un error de credenciales no dice si el usuario existe (lo decide Identidad).
      setError(err instanceof ErrorApi && err.problema.status === 401 ? 'Email o contraseña incorrectos.' : String((err as Error).message))
    } finally {
      setEnviando(false)
    }
  }

  return (
    <main className="ingreso">
      <form className="tarjeta formulario-ingreso" onSubmit={enviar}>
        <div className="marca grande">
          <span className="logo" aria-hidden="true">R</span>
          <span>Rumbo</span>
        </div>
        <p className="tenue">Traslados programados · ingreso de operadores</p>
        {aviso && <p className="alerta info">{aviso}</p>}
        <label>
          Email
          <input type="email" autoComplete="username" required value={email} onChange={(e) => setEmail(e.target.value)} />
        </label>
        <label>
          Contraseña
          <input
            type="password"
            autoComplete="current-password"
            required
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
        </label>
        {error && <p className="alerta error">{error}</p>}
        <button className="boton" disabled={enviando}>
          {enviando ? 'Ingresando…' : 'Ingresar'}
        </button>
      </form>
    </main>
  )
}
