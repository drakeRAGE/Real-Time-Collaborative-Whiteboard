import { defineConfig } from 'vite'
import tailwindcss from '@tailwindcss/vite'

import react from '@vitejs/plugin-react-swc'

// https://vite.dev/config/
export default defineConfig({
  plugins: [
    react(),
    tailwindcss(),
  ],
  define: {
    global: 'globalThis', // fixes "global is not defined"
  },
  optimizeDeps: {
    include: ['@mui/material', '@mui/material/styles']
  }
})
