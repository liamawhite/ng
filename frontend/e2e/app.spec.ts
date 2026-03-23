import { test, expect } from '@playwright/test'
import { BACKEND_PORT } from './globalSetup'

const BACKEND = `http://localhost:${BACKEND_PORT}`

test('redirects / to /projects', async ({ page }) => {
  await page.goto('/')
  await expect(page).toHaveURL('/projects')
})

test('nav renders home and projects links', async ({ page }) => {
  await page.goto('/projects')
  await expect(page.getByRole('link', { name: /projects/i })).toBeVisible()
})

test('projects page shows empty state when there are no projects', async ({ page }) => {
  await page.goto('/projects')
  await expect(page.getByText('No projects yet.')).toBeVisible()
})

test('projects page lists projects from the backend', async ({ page }) => {
  const res = await fetch(`${BACKEND}/api/v1/projects`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ title: 'Test Project' }),
  })
  const project = await res.json()

  try {
    await page.goto('/projects')
    await expect(page.getByText('Test Project')).toBeVisible()
  } finally {
    await fetch(`${BACKEND}/api/v1/projects/${project.id}`, { method: 'DELETE' })
  }
})
