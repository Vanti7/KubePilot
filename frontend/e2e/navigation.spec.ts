import { test, expect } from '@playwright/test'
import { login } from './fixtures'

// Every route reachable from the sidebar for an admin, paired with a heading
// to check where the page has one — pages without a plain <h1> just get a
// "did the app actually render something, not a blank/crashed page" check.
const ROUTES: { path: string; heading?: string }[] = [
  { path: '/', heading: 'Overview' },
  { path: '/updates' },
  { path: '/risks' },
  { path: '/exception-rules', heading: 'Exception rules' },
  { path: '/inventory' },
  { path: '/nodes' },
  { path: '/helm' },
  { path: '/secrets' },
  { path: '/deploy' },
  { path: '/clusters', heading: 'Clusters' },
  { path: '/registries', heading: 'Image registries' },
  { path: '/integrations', heading: 'Integrations' },
  { path: '/history', heading: 'History' },
  { path: '/settings', heading: 'Settings' },
]

test.describe('sidebar navigation', () => {
  test.beforeEach(async ({ page }) => {
    await login(page)
  })

  for (const route of ROUTES) {
    test(`${route.path} renders without crashing`, async ({ page }) => {
      await page.goto(route.path)
      await expect(page).toHaveURL(new RegExp(route.path.replace('/', '\\/') + '$'))

      // The sidebar brand mark surviving proves Layout (and therefore the
      // route's page component) didn't throw and unmount the whole tree.
      await expect(page.getByText('KubePilot', { exact: true })).toBeVisible()

      if (route.heading) {
        await expect(page.getByRole('heading', { name: route.heading })).toBeVisible()
      }
    })
  }

  test('unknown routes redirect to Overview', async ({ page }) => {
    await page.goto('/this-route-does-not-exist')
    await page.waitForURL('/')
    await expect(page.getByRole('heading', { name: 'Overview' })).toBeVisible()
  })
})
