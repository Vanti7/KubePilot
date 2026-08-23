import { test, expect } from '@playwright/test'
import { login } from './fixtures'

// Exercises the create -> appears in table -> delete round trip end to end
// through the real UI (Dialog form, DataTable render, window.confirm on
// delete) against the real POST/DELETE /api/v1/exception-rules endpoints —
// not just that the page renders.
test('create and delete a global exception rule', async ({ page }) => {
  await login(page)
  await page.goto('/exception-rules')

  const ruleName = `e2e-rule-${Date.now()}`

  await page.getByRole('button', { name: 'New rule' }).click()
  await page.getByLabel('Name').fill(ruleName)
  await page.getByPlaceholder(/why this exception exists/i).fill('Created by the Playwright E2E suite')
  await page.getByRole('button', { name: 'Add rule' }).click()

  // The dialog closes and the new row shows up once the create mutation
  // settles and the list query is invalidated.
  const row = page.locator('tr', { hasText: ruleName })
  await expect(row).toBeVisible()
  await expect(row.getByText('suppress')).toBeVisible() // default effect
  await expect(row.getByText('Global')).toBeVisible() // default scope

  page.once('dialog', (dialog) => dialog.accept())
  await row.getByTitle('Delete', { exact: true }).click()
  await expect(row).not.toBeVisible()
})

test('pausing a rule via the Power button updates its status', async ({ page }) => {
  // The create form always sends is_active:true (there's no "create as
  // paused" control in the UI) — pausing goes through the same PUT as a
  // full edit, a separate, already-correct code path from the
  // is_active:false-at-creation bug fixed in CreateExceptionRule. This just
  // checks the toggle round-trips through the real UI.
  await login(page)
  await page.goto('/exception-rules')

  const ruleName = `e2e-paused-${Date.now()}`
  await page.getByRole('button', { name: 'New rule' }).click()
  await page.getByLabel('Name').fill(ruleName)
  // Deliberately avoids the words "pause"/"paused" here — they'd collide as
  // a substring match against the Power button's title="Pause" within this
  // same row (getByTitle does a case-insensitive substring match by default).
  await page.getByPlaceholder(/why this exception exists/i).fill('Created for an E2E status-toggle check')
  await page.getByRole('button', { name: 'Add rule' }).click()

  const row = page.locator('tr', { hasText: ruleName })
  await expect(row).toBeVisible()
  await expect(row.getByText('active', { exact: true })).toBeVisible()

  await row.getByTitle('Pause', { exact: true }).click()
  await expect(row.getByText('paused', { exact: true })).toBeVisible()

  page.once('dialog', (dialog) => dialog.accept())
  await row.getByTitle('Delete', { exact: true }).click()
  await expect(row).not.toBeVisible()
})
