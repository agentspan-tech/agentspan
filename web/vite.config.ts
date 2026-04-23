import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import path from 'path'

export default defineConfig({
  plugins: [
    react(),
    tailwindcss(),
  ],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  server: {
    proxy: {
      '/api': 'http://localhost:8081',
      '/auth': 'http://localhost:8081',
      '/ws': { target: 'http://localhost:8081', ws: true },
    },
  },
  build: {
    outDir: 'dist',   // locked — processing embed.go depends on this path
  },
})
