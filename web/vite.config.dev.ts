import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'
import path from 'node:path'

/**
 * Development-only Vite configuration.
 *
 * This configuration is used when running `make dev` for local development.
 * It works with the NGINX proxy configuration that forwards
 * `https://chat2.laisky.com/` directly to `http://host:25173/`.
 *
 * Key features:
 * - No base path: NGINX forwards requests directly to root
 * - Proxy: API requests are forwarded to the Go backend on port 24456
 * - HMR: Hot module replacement works for instant frontend updates
 *
 * Flow:
 * 1. User visits https://chat2.laisky.com/
 * 2. NGINX forwards to http://host:25173/
 * 3. Vite serves index.html with React app
 * 4. React Router handles /gptchat and other routes
 * 5. API calls (/gptchat/api/*) are proxied to Go backend
 *
 * For production builds, use the default vite.config.ts which builds
 * the SPA for serving from the Go binary.
 */
export default defineConfig({
  plugins: [react()],
  // No base path needed - NGINX forwards directly to root
  server: {
    host: true,
    allowedHosts: true,
    port: 25173,
    // Configure HMR for the NGINX proxy setup
    // Since NGINX may not support WebSocket properly, we use a direct connection
    hmr: {
      // Connect directly to the Vite server for HMR
      host: '100.75.198.70',
      port: 25173,
      protocol: 'ws',
    },
    proxy: {
      // Proxy API requests to the Go backend
      // These patterns match API endpoints that should go to the backend
      '^/gptchat/(api|user|audit|audio|ramjet|oneapi|version|favicon\\.ico|create-payment-intent)': {
        target: 'http://127.0.0.1:24456',
        changeOrigin: true,
      },
      // Also proxy other task endpoints
      '^/(jav|auditlog|arweave|crawler|heartbeat|es|gitlab|health|version)': {
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
