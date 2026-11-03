import { fileURLToPath } from 'node:url'
import { defineConfig } from 'vite'
import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react-swc'

export default defineConfig({
  base: '/',
  plugins: [
    react(),
    tailwindcss(),
  ],
  resolve: {
    alias: {
      events: fileURLToPath(new URL('./node_modules/events/events.js', import.meta.url)),
    },
  },
  define: {
    global: 'globalThis',
  },
})
