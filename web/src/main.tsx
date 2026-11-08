import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { MantineProvider } from '@mantine/core'
import { Notifications } from '@mantine/notifications'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { BrowserRouter } from 'react-router'
import '@mantine/core/styles.css'
import '@mantine/notifications/styles.css'
import { ErrorApi } from './api'
import { App } from './App'
import { ProveedorSesion } from './componentes/ProveedorSesion'

// React Query guarda los datos del servidor y los vuelve a pedir cuando se invalidan (por ejemplo,
// al llegar un aviso por el stream). Las lecturas se reintentan solo ante fallas transitorias;
// las escrituras (mutations) nunca se reintentan solas.
const clienteConsultas = new QueryClient({
  defaultOptions: {
    queries: {
      retry: (intentos, error) => intentos < 2 && error instanceof ErrorApi && error.noDisponible,
      refetchOnWindowFocus: false,
    },
    mutations: { retry: false },
  },
})

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <MantineProvider>
      <Notifications position="top-right" />
      <QueryClientProvider client={clienteConsultas}>
        <BrowserRouter>
          <ProveedorSesion>
            <App />
          </ProveedorSesion>
        </BrowserRouter>
      </QueryClientProvider>
    </MantineProvider>
  </StrictMode>,
)
