import react from '@vitejs/plugin-react'
import { defineConfig, type PluginOption, type UserConfig } from 'vite'
import path from 'node:path'
import {
  isPrivateSourceMapBuild,
  privateSentryArtifactPlugin,
  privateSentryOptions,
} from './private-sentry-maps.mjs'

export default defineConfig(async ({ command }): Promise<UserConfig> => {
  const privateMaps = isPrivateSourceMapBuild(
    command,
    process.env.CHAT_SENTRY_PRIVATE_SOURCE_MAPS,
  )
  const plugins: PluginOption[] = [react()]
  if (privateMaps) {
    // Import and instantiate the standard plugin only for the approved private build.
    const { sentryVitePlugin } = await import('@sentry/vite-plugin')
    plugins.push(
      sentryVitePlugin(privateSentryOptions()),
      privateSentryArtifactPlugin(),
    )
  }
  return {
    plugins,
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
        '@': path.resolve(import.meta.dirname, './src'),
        events: 'events',
      },
    },
    define: {
      global: 'window',
    },
    build: {
      sourcemap: privateMaps ? 'hidden' : false,
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
  }
})
