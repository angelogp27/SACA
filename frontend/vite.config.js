import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

// En desarrollo la página habla con el lanzador local (bin/saca, puerto
// 8090): /local son sus botones y /api lo reenvía él al maestro del
// clúster. Con el lanzador, la página compilada la sirve él mismo.
const lanzador = { target: 'http://127.0.0.1:8090', changeOrigin: true }
const api = { '/api': lanzador, '/local': lanzador }

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  server: { proxy: api },
  preview: { proxy: api },
})
