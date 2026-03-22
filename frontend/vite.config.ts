import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import path from 'path'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: { '@': path.resolve(__dirname, './src') },
  },
  build: { outDir: '../desktop/frontend/dist', emptyOutDir: true },
  test: { environment: 'jsdom', include: ['src/**/*.{test,spec}.{ts,tsx}'] },
})
