import react from '@vitejs/plugin-react'
import { defineConfig, loadEnv } from 'vite'

// En desarrollo, Vite reenvía al API Gateway las rutas de la API: la web habla solo con el
// gateway (punto de entrada único) y no hace falta CORS. GATEWAY_URL permite apuntar a otro
// puerto u otra máquina.
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '')
  const gateway = env.GATEWAY_URL || 'http://localhost:8080'
  return {
    plugins: [react()],
    server: {
      proxy: {
        '/api': gateway,
      },
    },
  }
})
