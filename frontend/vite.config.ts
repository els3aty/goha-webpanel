import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/api': {
        target: 'http://142.132.213.38:2087',
        changeOrigin: true,
      }
    }
  },
  build: {
    outDir: '../control-plane/ui/dist',
    emptyOutDir: true
  }
})
