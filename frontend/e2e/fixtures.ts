import type { Page } from '@playwright/test'

// The demo-mode seed always creates this admin account (see
// backend/internal/bootstrap — DEMO_MODE defaults ADMIN_PASSWORD to "demo"
// specifically so E2E/manual testing doesn't need scraped credentials).
export const DEMO_ADMIN = { email: 'admin@kubepilot.local', password: 'demo' }

export async function login(page: Page, { email, password } = DEMO_ADMIN) {
  await page.goto('/login')
  await page.getByLabel('Email address').fill(email)
  await page.getByLabel('Password').fill(password)
  await page.getByRole('button', { name: /sign in/i }).click()
  await page.waitForURL('/')
}
