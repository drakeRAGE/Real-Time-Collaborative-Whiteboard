import { expect, test } from '@playwright/test';

test('login page renders', async ({ page }) => {
  await page.goto('/auth');
  await expect(page.getByRole('heading', { name: 'Welcome Back' })).toBeVisible();
  await expect(page.getByLabel('Email Address')).toBeVisible();
  await expect(page.getByLabel('Password')).toBeVisible();
  await expect(page.getByRole('button', { name: 'Sign In' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Sign Up' })).toBeVisible();
});

test('unauthenticated home redirects to login', async ({ page }) => {
  await page.goto('/');
  await expect(page).toHaveURL(/\/auth$/);
  await expect(page.getByRole('heading', { name: 'Welcome Back' })).toBeVisible();
});
