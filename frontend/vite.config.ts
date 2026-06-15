import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// Backend target for the dev proxy. Override with VITE_PROXY_TARGET to point a
// dev frontend at an alternate backend (e.g. a parallel demo instance).
const proxyTarget = process.env.VITE_PROXY_TARGET || 'http://localhost:8080'

export default defineConfig({
  plugins: [react()],
  server: {
    port: 3000,
    proxy: {
      '/api': {
        target: proxyTarget,
        changeOrigin: true,
      },
      '/events': {
        target: proxyTarget,
        changeOrigin: true,
      }
    }
  }
})
