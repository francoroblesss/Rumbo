import react from '@vitejs/plugin-react'
import { defineConfig } from 'vitest/config'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  build: {
    rolldownOptions: {
      output: {
        // Las librerías van en archivos propios: cambian menos que el código de la web y el
        // navegador las reusa de su caché entre versiones.
        codeSplitting: {
          groups: [
            { name: 'mantine', test: /node_modules[\\/]@mantine/ },
            { name: 'react', test: /node_modules[\\/](react|react-dom|react-router|scheduler|@tanstack)[\\/]/ },
          ],
        },
      },
    },
  },
  // Los tests (vitest) corren en Node: prueban módulos sin DOM (sse.ts y api.ts).
  test: {
    environment: 'node',
  },
})
