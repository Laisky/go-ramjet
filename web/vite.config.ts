import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'
import path from 'node:path'

export default defineConfig({
  plugins: [react()],
  server: {
    host: true,
    allowedHosts: true,
    proxy: {
      '^/(gptchat|jav|auditlog|arweave|crawler|heartbeat|es|gitlab|health|version)': {
        target: 'http://127.0.0.1:24456',
        changeOrigin: true,
      },
    },
  },
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
      events: 'events',
    },
  },
  define: {
    global: 'window',
  },
})
