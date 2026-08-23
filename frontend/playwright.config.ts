import { defineConfig, devices } from '@playwright/test'

// Dedicated ports, distinct from the normal dev-server defaults (frontend
// 3000, backend 8080) — a developer's regular `npm run dev` / backend can
// stay running alongside an E2E run without either one reusing the other's
// (differently-seeded) server by accident.
const FRONTEND_PORT = 3001
const BACKEND_PORT = 8090

export default defineConfig({
  testDir: './e2e',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: process.env.CI ? 1 : undefined,
  reporter: [['html', { open: 'never' }]],

  use: {
    baseURL: `http://localhost:${FRONTEND_PORT}`,
    trace: 'on-first-retry',
  },

  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],

  // Both servers are started fresh for a CI run; locally, reuse whatever's
  // already listening on these exact E2E ports (not the normal dev ports)
  // so repeated `npm run test:e2e` runs don't pay the go-run/vite startup
  // cost every time.
  webServer: [
    {
      command: 'node e2e/run-backend.mjs',
      url: `http://localhost:${BACKEND_PORT}/health`,
      reuseExistingServer: !process.env.CI,
      timeout: 90_000,
      env: { KUBEPILOT_E2E_PORT: String(BACKEND_PORT) },
    },
    {
      command: `npm run dev -- --port ${FRONTEND_PORT} --strictPort`,
      url: `http://localhost:${FRONTEND_PORT}`,
      reuseExistingServer: !process.env.CI,
      timeout: 60_000,
      env: { VITE_PROXY_TARGET: `http://localhost:${BACKEND_PORT}` },
    },
  ],
})
