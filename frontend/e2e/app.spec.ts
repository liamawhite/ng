import { test, expect } from '@playwright/test'

test('app loads and renders', async ({ page }) => {
  // Mock the backend so the test doesn't need a running server
  await page.route('**/api/v1/projects', route =>
    route.fulfill({ json: { projects: [] } })
  )

  await page.goto('/')

  await expect(page.getByRole('heading', { name: 'ng' })).toBeVisible()
  await expect(page.getByText('No projects yet.')).toBeVisible()
})
