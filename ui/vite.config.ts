import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

const service = 'http://127.0.0.1:8470'

export default defineConfig({
  plugins: [react()],
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    rolldownOptions: {
      output: {
        // three chunks, each under the 500 kB warning: react, the markdown
        // renderer with the rest of the dependencies, and mega's own code.
        // all three load with the page; the split is for size, not laziness.
        codeSplitting: {
          groups: [
            { name: 'react', test: /node_modules[\\/](react|react-dom|scheduler)[\\/]/, priority: 2 },
            { name: 'vendor', test: /node_modules[\\/]/, priority: 1 },
          ],
        },
      },
    },
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
