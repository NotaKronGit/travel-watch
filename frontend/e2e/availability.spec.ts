import { test, expect } from '@playwright/test';

const api = '**/travelwatch.cabinet.v1.AuthService/*';
const user = { id: 'availability-test', email: 'availability@example.com' };

for (const failure of ['502', 'network'] as const) {
  test(`session check recovers from ${failure} without logging out`, async ({ page }) => {
    let unavailable = true;
    let logoutCalls = 0;
    const errors: string[] = [];
    page.on('pageerror', error => errors.push(error.message));
    await page.route(api, async route => {
      if (route.request().url().endsWith('/Logout')) logoutCalls++;
      if (unavailable) {
        if (failure === 'network') await route.abort('failed');
        else await route.fulfill({ status: 502, contentType: 'text/plain', body: 'Bad Gateway' });
      } else await route.fulfill({ json: { user } });
    });
    await page.goto('/#/account');
    await expect(page.getByRole('heading', { name: 'Кабинет временно недоступен' })).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Добро пожаловать' })).toHaveCount(0);
    await page.getByRole('button', { name: 'Повторить проверку' }).click();
    await expect(page.getByRole('heading', { name: 'Кабинет временно недоступен' })).toBeVisible();
    unavailable = false;
    await page.getByRole('button', { name: 'Повторить проверку' }).click();
    await expect(page.getByRole('heading', { name: 'Добро пожаловать' })).toBeVisible();
    await expect(page.getByText(user.email, { exact: true }).last()).toBeVisible();
    expect(logoutCalls).toBe(0);
    expect(errors).toEqual([]);
  });
}

test('missing session redirects to login without requesting logout', async ({ page }) => {
  let logoutCalls = 0;
  const errors: string[] = [];
  page.on('pageerror', error => errors.push(error.message));
  await page.route(api, async route => {
    if (route.request().url().endsWith('/Logout')) logoutCalls++;
    await route.fulfill({ status: 401, json: { code: 'unauthenticated', message: 'Session expired' } });
  });
  await page.goto('/#/account');
  await expect(page.getByRole('heading', { name: 'С возвращением' })).toBeVisible();
  expect(logoutCalls).toBe(0);
  expect(errors).toEqual([]);
});

test('failed explicit logout is visible and can be retried', async ({ page }) => {
  let failLogout = true;
  let logoutCalls = 0;
  const errors: string[] = [];
  page.on('pageerror', error => errors.push(error.message));
  await page.route(api, async route => {
    if (route.request().url().endsWith('/Logout')) {
      logoutCalls++;
      await route.fulfill(failLogout
        ? { status: 502, contentType: 'text/plain', body: 'Bad Gateway' }
        : { json: {} });
    } else await route.fulfill({ json: { user } });
  });
  await page.goto('/#/account');
  await expect(page.getByRole('heading', { name: 'Добро пожаловать' })).toBeVisible();
  await page.getByRole('button', { name: /профиль|profile|user menu/i }).click();
  await page.getByRole('menuitem', { name: 'Выйти', exact: true }).click();
  await expect(page.getByRole('alert')).toContainText('Не удалось подтвердить выход');
  await expect(page.getByRole('heading', { name: 'С возвращением' })).toHaveCount(0);
  failLogout = false;
  await page.getByRole('button', { name: /профиль|profile|user menu/i }).click();
  await page.getByRole('menuitem', { name: 'Выйти', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'С возвращением' })).toBeVisible();
  expect(logoutCalls).toBe(2);
  expect(errors).toEqual([]);
});
