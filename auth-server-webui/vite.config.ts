import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

const backend = process.env.BACKEND_URL || 'http://localhost:8080'

export default defineConfig({
  base: '/admin-page/',
  plugins: [tailwindcss(), react()],
  server: {
    host: true,
    port: 5173,
    strictPort: true,
    allowedHosts: ['aki.internal', 'localhost', '127.0.0.1'],
    proxy: {
      '/oauth2': backend,
      '/login': backend,
      '/auth': backend,
      '/api': backend,
      '/registration': backend,
      '/userinfo': backend,
      '/introspect': backend,
      '/.well-known': backend,
    },
  },
})
