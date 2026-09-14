import { test, expect } from '@playwright/test';

test('register, reload, logout, reject a wrong password, and log in again', async ({ page }) => {
  const email = `browser-${Date.now()}@example.com`;
  const password = 'a long browser test password';
  await page.goto('/');
  await page.getByRole('button', { name: 'Нет аккаунта? Зарегистрироваться' }).click();
  await page.getByRole('textbox', { name: 'Email', exact: true }).fill(email);
  await page.locator('input[name="password"]').fill(password);
  await page.getByRole('button', { name: 'Создать аккаунт', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Добро пожаловать' })).toBeVisible();
  await expect(page.getByText(email, { exact: true }).last()).toBeVisible();
  await page.reload();
  await expect(page.getByRole('heading', { name: 'Добро пожаловать' })).toBeVisible();
  await page.screenshot({ path: 'test-results/cabinet-desktop.png', fullPage: true });
  await page.getByRole('button', { name: /профиль|profile|user menu/i }).click();
  await page.getByRole('menuitem', { name: /выход|выйти|logout/i }).click();
  await expect(page.getByRole('heading', { name: 'С возвращением' })).toBeVisible();
  await page.getByRole('textbox', { name: 'Email', exact: true }).fill(email);
  await page.locator('input[name="password"]').fill('a wrong browser test password');
  await page.getByRole('button', { name: 'Войти', exact: true }).click();
  await expect(page.getByRole('alert')).toContainText('Неверный email или пароль');
  await page.locator('input[name="password"]').fill(password);
  await page.getByRole('button', { name: 'Войти', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Добро пожаловать' })).toBeVisible();
});

test('mobile login has no horizontal overflow', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto('/');
  await expect(page.getByRole('heading', { name: 'С возвращением' })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  await page.screenshot({ path: 'test-results/login-mobile.png', fullPage: true });
});
