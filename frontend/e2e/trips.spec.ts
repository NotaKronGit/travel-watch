import { test, expect } from '@playwright/test';
test.beforeEach(async ({page})=>{ await page.route('**/travelwatch.cabinet.v1.TripService/GetTripRoutes',r=>r.fulfill({json:{result:{sources:[]}}})); });
const origin = {id: '11111111-1111-1111-1111-111111111111', name:'Курск', country:'Россия', region:'', iataCode:'URS', timezone:'Europe/Moscow'};
const destination = {id: '22222222-2222-2222-2222-222222222222', name:'Бангкок', country:'Таиланд', region:'', timezone:'Asia/Bangkok'};

test('create trip uses selected IDs, preserves retry key and reports saved state', async ({page}, testInfo) => {
  let checks = 0;
  let unavailable = false;
  await page.route('**/travelwatch.cabinet.v1.AuthService/*', async route => {
    checks++;
    await new Promise(resolve => setTimeout(resolve, 100));
    await route.fulfill(unavailable ? {status:502,json:{code:'unavailable'}} : {json:{user:{id:'test-user',email:'trip@example.com'}}});
  });
  const submissions: Record<string, unknown>[] = [];
  await page.route('**/travelwatch.cabinet.v1.TripService/*', async route => {
    const body = route.request().postDataJSON();
    if (route.request().url().endsWith('/GetTrip')) { await route.fulfill({json:{trip:{id:'saved-trip',origin,destination,departureFrom:'2027-01-10',departureTo:'2027-01-12',adults:1,status:'TRIP_STATUS_SAVED'}}}); return; }
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
  for (const fail of [false, true]) {
    unavailable = fail;
    const before = checks;
    await page.evaluate(() => {
      Object.defineProperty(document, 'visibilityState', {configurable:true,value:'hidden'});
      window.dispatchEvent(new Event('visibilitychange'));
      Object.defineProperty(document, 'visibilityState', {configurable:true,value:'visible'});
      window.dispatchEvent(new Event('visibilitychange'));
    });
    await expect.poll(() => checks).toBeGreaterThan(before);
    if (fail) await expect(page.getByText(/Введённые данные сохранены/)).toBeVisible();
    else await page.waitForTimeout(200);
    await expect(page.getByRole('textbox',{name:'Самая ранняя дата выезда',exact:true})).toHaveValue('2027-01-10');
    await expect(page.getByRole('combobox',{name:'Откуда'})).toHaveValue('Курск, Россия (URS)');
  }
  unavailable = false;
  await page.getByRole('button',{name:'Повторить проверку',exact:true}).click();
  await expect(page.getByText(/Введённые данные сохранены/)).toHaveCount(0);
  await page.getByRole('button',{name:'Сохранить заявку',exact:true}).click();
  await expect(page).toHaveURL(/trips\/saved-trip$/);
  await expect(page.getByRole('heading',{name:'Заявка',exact:true,level:1})).toBeVisible();
  expect(submissions).toHaveLength(2);
  expect(submissions[0]).toEqual(submissions[1]);
  expect(submissions[1].originId).toBe(origin.id);
  await expect(page.getByText(/Сохранена, поиск ещё не запущен/)).toBeVisible();
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

test('same-name cities retain readable regions after selection', async ({page}) => {
  await page.route('**/travelwatch.cabinet.v1.AuthService/*', route => route.fulfill({json:{user:{id:'test-user',email:'trip@example.com'}}}));
  await page.route('**/travelwatch.cabinet.v1.TripService/SearchCities', route => route.fulfill({json:{cities:[
    {...origin,name:'Тестовый город',iataCode:'',region:'Первая область'},
    {...destination,name:'Тестовый город',country:'Россия',iataCode:'',region:'Вторая область'},
  ]}}));
  await page.goto('/#/trips/create');
  const field = page.getByRole('combobox',{name:'Откуда'});
  await field.fill('Тест');
  await expect(page.getByRole('option',{name:'Тестовый город, Россия, Первая область',exact:true})).toBeVisible();
  await page.getByRole('option',{name:'Тестовый город, Россия, Вторая область',exact:true}).click();
  await expect(field).toHaveValue('Тестовый город, Россия, Вторая область');
});

test('expired session after editing redirects to login', async ({page}) => {
  let expired = false;
  await page.route('**/travelwatch.cabinet.v1.AuthService/*', route => route.fulfill(expired
    ? {status:401,json:{code:'unauthenticated',message:'Session expired'}}
    : {json:{user:{id:'test-user',email:'trip@example.com'}}}));
  await page.goto('/#/trips/create');
  await page.getByRole('textbox',{name:'Самая ранняя дата выезда',exact:true}).fill('2027-01-10');
  expired = true;
  await page.evaluate(() => {
    Object.defineProperty(document, 'visibilityState', {configurable:true,value:'hidden'});
    window.dispatchEvent(new Event('visibilitychange'));
    Object.defineProperty(document, 'visibilityState', {configurable:true,value:'visible'});
    window.dispatchEvent(new Event('visibilitychange'));
  });
  await expect(page.getByRole('heading',{name:'С возвращением'})).toBeVisible();
});
