import { test, expect } from '@playwright/test';
const origin = {id: '11111111-1111-1111-1111-111111111111', name:'Курск', country:'Россия', region:'49', iataCode:'URS', timezone:'Europe/Moscow'};
const destination = {id: '22222222-2222-2222-2222-222222222222', name:'Бангкок', country:'Таиланд', region:'', timezone:'Asia/Bangkok'};

test('create trip uses selected IDs, preserves retry key and reports saved state', async ({page}, testInfo) => {
  await page.route('**/travelwatch.cabinet.v1.AuthService/*', route => route.fulfill({json:{user:{id:'test-user',email:'trip@example.com'}}}));
  const submissions: Record<string, unknown>[] = [];
  await page.route('**/travelwatch.cabinet.v1.TripService/*', async route => {
    const body = route.request().postDataJSON();
    if (route.request().url().endsWith('/SearchCities')) await route.fulfill({json:{cities: body.query.startsWith('Ку') ? [origin] : [destination]}});
    else { submissions.push(body); await route.fulfill(submissions.length === 1 ? {status:503,json:{code:'unavailable',message:'Не удалось подтвердить сохранение'}} : {json:{id:'saved-trip'}}); }
  });
  await page.goto('/#/trips/create');
  await expect(page.getByRole('heading',{name:'Создать заявку',exact:true,level:1})).toBeVisible();
  await page.screenshot({path:testInfo.outputPath('form.png'),fullPage:true});
  await expect(page.getByRole('heading',{name:'Создать заявку',exact:true,level:1})).toBeVisible();
  await page.getByRole('combobox',{name:'Откуда'}).fill('Кур');
  await expect(page.getByRole('option',{name:'Курск, Россия (URS)'})).toBeVisible();
  await page.screenshot({path:testInfo.outputPath('city-options.png'),fullPage:true});
  await page.getByRole('option',{name:'Курск, Россия (URS)'}).click();
  await page.getByRole('combobox',{name:'Куда',exact:true}).fill('Бан');
  await page.getByRole('option',{name:'Бангкок, Таиланд'}).click();
  await page.getByRole('textbox',{name:'Самая ранняя дата выезда',exact:true}).fill('2027-01-10');
  await page.getByRole('textbox',{name:'Самая поздняя дата выезда',exact:true}).fill('2027-01-12');
  await page.getByRole('button',{name:'Сохранить заявку',exact:true}).click();
  await expect(page.getByRole('alert').first()).toContainText('Не удалось подтвердить сохранение');
  await expect(page.getByRole('textbox',{name:'Самая ранняя дата выезда',exact:true})).toHaveValue('2027-01-10');
  await page.getByRole('button',{name:'Сохранить заявку',exact:true}).click();
  await expect(page.getByRole('heading',{name:'Заявка сохранена'})).toBeVisible();
  expect(submissions).toHaveLength(2);
  expect(submissions[0]).toEqual(submissions[1]);
  expect(submissions[1].originId).toBe(origin.id);
  await expect(page.getByText(/Поиск ещё не запущен/)).toBeVisible();
});

test('city errors can be retried and editing a selection invalidates its ID on mobile', async ({page}, testInfo) => {
  await page.setViewportSize({width:390,height:844});
  await page.route('**/travelwatch.cabinet.v1.AuthService/*', route => route.fulfill({json:{user:{id:'test-user',email:'trip@example.com'}}}));
  let fail = true; let saves = 0;
  await page.route('**/travelwatch.cabinet.v1.TripService/*', async route => {
    if (route.request().url().endsWith('/CreateTrip')) { saves++; await route.fulfill({json:{id:'unexpected'}}); return; }
    await route.fulfill(fail ? {status:502,body:'unavailable'} : {json:{cities:[origin]}});
  });
  await page.goto('/#/trips/create');
  await expect(page.getByRole('heading',{name:'Создать заявку',exact:true,level:1})).toBeVisible();
  await page.screenshot({path:testInfo.outputPath('form.png'),fullPage:true});
  const field = page.getByRole('combobox',{name:'Откуда'});
  await field.fill('Кур');
  await expect(page.getByRole('button',{name:'Повторить поиск'})).toBeVisible();
  fail = false;
  await page.getByRole('button',{name:'Повторить поиск'}).click();
  await field.click();
  await page.getByRole('option',{name:'Курск, Россия (URS)'}).click();
  await field.fill('Изменённый текст');
  await page.getByRole('combobox',{name:'Куда',exact:true}).fill('Не выбран');
  await page.getByRole('textbox',{name:'Самая ранняя дата выезда',exact:true}).fill('2027-01-10');
  await page.getByRole('textbox',{name:'Самая поздняя дата выезда',exact:true}).fill('2027-01-12');
  await page.getByRole('button',{name:'Сохранить заявку',exact:true}).click();
  await expect(page.getByText('Выберите оба города из подсказок.')).toBeVisible();
  expect(saves).toBe(0);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBeTruthy();
});
