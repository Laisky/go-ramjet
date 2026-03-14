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
    rollupOptions: {
      output: {
        manualChunks: {
          // React core - cached long-term, rarely changes
          'vendor-react': ['react', 'react-dom', 'react-router-dom'],
          // Markdown rendering pipeline - heavy, only needed for chat messages
          'vendor-markdown': [
            'react-markdown',
            'remark-gfm',
            'remark-math',
            'rehype-katex',
            'rehype-raw',
          ],
          // Syntax highlighting
          'vendor-hljs': ['highlight.js'],
          // KaTeX CSS + fonts
          'vendor-katex': ['katex'],
          // Local database layer
          'vendor-pouchdb': ['pouchdb-browser'],
          // Radix UI primitives
          'vendor-radix': [
            '@radix-ui/react-dialog',
            '@radix-ui/react-dropdown-menu',
            '@radix-ui/react-scroll-area',
            '@radix-ui/react-select',
            '@radix-ui/react-separator',
            '@radix-ui/react-slider',
            '@radix-ui/react-switch',
            '@radix-ui/react-tabs',
            '@radix-ui/react-tooltip',
          ],
          // Payment - rarely used
          'vendor-stripe': ['@stripe/stripe-js', '@stripe/react-stripe-js'],
        },
      },
    },
  },
})
