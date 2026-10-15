import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

// La API del maestro en modo servidor (entrenamiento/cmd/maestro -web)
// escucha en 127.0.0.1:8080; la página la usa como /api a través de este
// proxy, tanto con `npm run dev` como con `npm run preview`.
const api = { '/api': { target: 'http://127.0.0.1:8080', changeOrigin: true } }

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  server: { proxy: api },
  preview: { proxy: api },
})
