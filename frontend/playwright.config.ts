import { defineConfig, devices } from '@playwright/test'
import { BACKEND_PORT } from './e2e/globalSetup'

export default defineConfig({
  testDir: './e2e',
  globalSetup: './e2e/globalSetup.ts',
  workers: 1,
  use: {
    baseURL: 'http://localhost:5174',
  },
  projects: [
    { name: 'chromium', use: { ...devices['Desktop Chrome'] } },
  ],
  webServer: {
    command: 'vite --port 5174',
    port: 5174,
    reuseExistingServer: false,
    env: {
      VITE_BACKEND_URL: `http://localhost:${BACKEND_PORT}`,
    },
  },
})
