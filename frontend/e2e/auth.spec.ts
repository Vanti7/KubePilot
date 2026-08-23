import { test, expect } from '@playwright/test'
import { login, DEMO_ADMIN } from './fixtures'

test('unauthenticated visitors are redirected to /login', async ({ page }) => {
  await page.goto('/')
  await page.waitForURL('/login')
  await expect(page.getByRole('heading', { name: 'KubePilot' })).toBeVisible()
})

test('wrong credentials show an error and do not navigate away', async ({ page }) => {
  await page.goto('/login')
  await page.getByLabel('Email address').fill(DEMO_ADMIN.email)
  await page.getByLabel('Password').fill('definitely-not-the-password')
  await page.getByRole('button', { name: /sign in/i }).click()

  // Login.tsx surfaces err.message as-is (no friendly-message mapping for a
  // 401 today), so this is deliberately a loose match rather than pinning
  // axios's exact wording.
  await expect(page.getByText(/status code 401|invalid credentials|login failed/i)).toBeVisible()
  await expect(page).toHaveURL(/\/login$/)
})

test('correct credentials land on Overview with the sidebar visible', async ({ page }) => {
  await login(page)
  await expect(page.getByRole('heading', { name: 'Overview' })).toBeVisible()
  await expect(page.getByText('KubePilot', { exact: true })).toBeVisible() // sidebar brand
})

test('signing out via the user menu returns to /login and blocks the previous page', async ({ page }) => {
  await login(page)
  await page.getByRole('button', { name: DEMO_ADMIN.email }).click()
  await page.getByRole('menuitem', { name: /sign out/i }).click()
  await page.waitForURL('/login')

  await page.goto('/')
  await page.waitForURL('/login')
})
