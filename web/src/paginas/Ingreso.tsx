import { useState, type FormEvent } from 'react'
import { Button, Center, Paper, PasswordInput, Stack, Text, TextInput, Title } from '@mantine/core'
import { useNavigate } from 'react-router'
import { MensajeError } from '../componentes/MensajeError'
import { pantallaInicial, useSesion } from '../sesion'

/** Ingreso: login → token en sessionStorage → perfil (/yo) → pantalla inicial del rol. */
export function Ingreso() {
  const { ingresar } = useSesion()
  const navegar = useNavigate()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [enviando, setEnviando] = useState(false)
  const [error, setError] = useState<unknown>(null)

  async function enviar(evento: FormEvent) {
    evento.preventDefault()
    setEnviando(true)
    setError(null)
    try {
      const perfil = await ingresar(email.trim(), password)
      navegar(pantallaInicial(perfil), { replace: true })
    } catch (e) {
      setError(e)
    } finally {
      setEnviando(false)
    }
  }

  return (
    <Center mih="100vh" p="md">
      <Paper withBorder shadow="sm" p="xl" radius="md" w="100%" maw={400}>
        <form onSubmit={enviar}>
          <Stack>
            <div>
              <Title order={2}>Rumbo</Title>
              <Text c="dimmed" size="sm">
                Ingresá con tu usuario
              </Text>
            </div>
            <TextInput
              label="Email"
              type="email"
              autoComplete="username"
              required
              value={email}
              onChange={(e) => setEmail(e.currentTarget.value)}
            />
            <PasswordInput
              label="Contraseña"
              autoComplete="current-password"
              required
              value={password}
              onChange={(e) => setPassword(e.currentTarget.value)}
            />
            <MensajeError error={error} titulo="No se pudo ingresar" />
            <Button type="submit" loading={enviando}>
              Ingresar
            </Button>
          </Stack>
        </form>
      </Paper>
    </Center>
  )
}
