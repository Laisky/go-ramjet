import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// https://vitejs.dev/config/
export default defineConfig({
    plugins: [react()],
    server: {
        // If you need a proxy
        // proxy: {
        //   '/api': {
        //     target: 'http://localhost:8000',
        //     changeOrigin: true,
        //     secure: false,
        //   }
        // }
    },
    build: {
        sourcemap: true, // Recommended for debugging in production
    }
})
