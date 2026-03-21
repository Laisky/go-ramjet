import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'
import path from 'node:path'

export default defineConfig({
  plugins: [react()],
  server: {
    host: true,
    allowedHosts: true,
    proxy: {
      '^/(gptchat|jav|auditlog|arweave|crawler|heartbeat|es|gitlab|health|version)':
        {
          target: 'http://127.0.0.1:24456',
          changeOrigin: true,
        },
      '^/cv/(content|pdf)': {
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
  build: {
    rolldownOptions: {
      output: {
        manualChunks(id: string) {
          // React core - cached long-term, rarely changes
          if (
            /node_modules\/(react|react-dom|react-router|react-router-dom)\//.test(
              id,
            )
          ) {
            return 'vendor-react'
          }
          // Markdown rendering pipeline - heavy, only needed for chat messages
          if (
            /node_modules\/(react-markdown|remark-gfm|remark-math|rehype-katex|rehype-raw)\//.test(
              id,
            )
          ) {
            return 'vendor-markdown'
          }
          // Syntax highlighting
          if (id.includes('node_modules/highlight.js/')) {
            return 'vendor-hljs'
          }
          // KaTeX CSS + fonts
          if (id.includes('node_modules/katex/')) {
            return 'vendor-katex'
          }
          // Local database layer
          if (id.includes('node_modules/pouchdb-browser/')) {
            return 'vendor-pouchdb'
          }
          // Radix UI primitives
          if (id.includes('node_modules/@radix-ui/')) {
            return 'vendor-radix'
          }
          // Payment - rarely used
          if (id.includes('node_modules/@stripe/')) {
            return 'vendor-stripe'
          }
        },
      },
    },
  },
})
