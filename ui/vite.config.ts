import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

const service = 'http://127.0.0.1:8470'

export default defineConfig({
  plugins: [react()],
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
  server: {
    port: 5173,
    proxy: {
      // the service rejects requests whose Origin differs from its Host, so the
      // dev proxy presents both as the service's own.
      '/api': { target: service, ws: true, changeOrigin: true, headers: { origin: service } },
    },
  },
})
