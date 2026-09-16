import {test,expect} from '@playwright/test';
const trip = {id:'11111111-1111-1111-1111-111111111111',origin:{name:'Курск',country:'Россия',region:'Курская область'},destination:{name:'Бангкок',country:'Таиланд'},departureFrom:'2027-01-10',departureTo:'2027-01-12',adults:2,status:'TRIP_STATUS_SAVED'};
test.beforeEach(async ({page}) => {
 await page.route('**/travelwatch.cabinet.v1.AuthService/*', r=>r.fulfill({json:{user:{id:'test',email:'test@example.com'}}}));
});
test('list pagination and direct detail survive reload on mobile',async ({page},testInfo)=>{
 await page.setViewportSize({width:390,height:844});
 await page.route('**/travelwatch.cabinet.v1.TripService/ListTrips',r=>r.fulfill({json:{trips:[{...trip,id:r.request().postDataJSON().offset ? 'second' : trip.id}],hasMore:!r.request().postDataJSON().offset}}));
 await page.route('**/travelwatch.cabinet.v1.TripService/GetTrip',r=>r.fulfill({json:{trip}}));
 await page.goto('/#/trips');
 await expect(page.getByText('Курск → Бангкок')).toBeVisible();
 await page.getByRole('button',{name:'Далее',exact:true}).click();
 await expect(page.getByText('Страница 2')).toBeVisible();
 await expect(page.getByRole('button',{name:'Далее',exact:true})).toBeDisabled();
 await page.getByRole('button',{name:'Назад',exact:true}).click();
 await page.getByRole('link',{name:'Открыть заявку'}).click();
 await expect(page).toHaveURL(new RegExp(trip.id+'$'));
 await page.reload();
 await expect(page.getByText('Курская область',{exact:false})).toBeVisible();
 await expect(page.getByText('Сохранена, поиск ещё не запущен')).toBeVisible();
 await page.screenshot({path:testInfo.outputPath('detail-mobile.png'),fullPage:true});
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBeTruthy();
});
test('empty list, retry and unavailable detail are explicit',async ({page})=>{
 let fail = true;
 await page.route('**/travelwatch.cabinet.v1.TripService/ListTrips',r=>r.fulfill(fail ? {status:503,json:{code:'unavailable'}} : {json:{trips:[]}}));
 await page.goto('/#/trips');
 await expect(page.getByRole('alert')).toContainText('Не удалось загрузить');
 fail=false;
 await page.getByRole('button',{name:'Повторить загрузку'}).click();
 await expect(page.getByText('У вас пока нет заявок')).toBeVisible();
 await page.route('**/travelwatch.cabinet.v1.TripService/GetTrip',r=>r.fulfill({status:404,json:{code:'not_found'}}));
 await page.goto('/#/trips/'+trip.id);
 await expect(page.getByRole('alert')).toHaveText('Заявка не найдена');
 await expect(page.getByText('Курск → Бангкок')).toHaveCount(0);
});

test('failed loading of another list page offers retry',async ({page})=>{
 let fail=true;
 await page.route('**/travelwatch.cabinet.v1.TripService/ListTrips',r=>{
  const second=!!r.request().postDataJSON().offset;
  return r.fulfill(second && fail ? {status:503,json:{code:'unavailable'}} : {json:{trips:second ? [] : [trip],hasMore:!second}});
 });
 await page.goto('/#/trips');
 await expect(page.getByText('Курск → Бангкок')).toBeVisible();
 await page.getByRole('button',{name:'Далее',exact:true}).click();
 await expect(page.getByRole('alert')).toContainText('Не удалось загрузить');
 await expect(page.getByText('Курск → Бангкок')).toHaveCount(0);
 fail=false;
 await page.getByRole('button',{name:'Повторить загрузку'}).click();
 await expect(page.getByText('У вас пока нет заявок')).toBeVisible();
 await expect(page.getByText('Страница 2')).toBeVisible();
});

test('cancel keeps history and private comment, supports retry after lost response', async ({page}) => {
  await page.route('**/travelwatch.cabinet.v1.AuthService/*', route=>route.fulfill({json:{user:{id:'test-user',email:'test@example.com'}}}));
  const trip = {id:'cancel-test',origin:{name:'Курск'},destination:{name:'Москва'},departureFrom:'2027-01-01',departureTo:'2027-01-02',adults:1,status:'TRIP_STATUS_SAVED',comment:'',cancelledAt:undefined as string|undefined};
  let attempts=0;let comments=0;
  await page.route('**/travelwatch.cabinet.v1.TripService/*', async route=>{
    const method=route.request().url().split('/').pop();
    if(method==='CancelTrip'){trip.status='TRIP_STATUS_CANCELLED';trip.cancelledAt='2027-01-01T12:30:00Z';attempts++;await route.fulfill(attempts===1 ? {status:503,json:{code:'unavailable',message:'Повторите отмену'}} : {json:{}});}
    else if(method==='UpdateTripComment'){comments++;if(comments===1){await route.fulfill({status:503,json:{code:'unavailable',message:'Повторите сохранение'}});return;}trip.comment=route.request().postDataJSON().comment;await route.fulfill({json:{}});}
    else if(method==='ListTrips')await route.fulfill({json:{trips:[trip]}});
    else await route.fulfill({json:{trip}});
  });
  await page.goto('/#/trips/cancel-test');
  const note=page.getByRole('textbox',{name:'Комментарий к заявке'});
  await note.fill('Личная заметка <script>');
  await page.getByRole('button',{name:'Сохранить комментарий'}).click();
  await expect(page.getByText('Повторите сохранение')).toBeVisible();
  await expect(note).toHaveValue('Личная заметка <script>');
  await page.getByRole('button',{name:'Сохранить комментарий'}).click();
  await expect(page.getByText('Комментарий сохранён')).toBeVisible();
  await note.fill('Несохранённый черновик');
  await page.getByRole('button',{name:'Отменить заявку',exact:true}).click();
  await page.getByRole('button',{name:'Оставить заявку'}).click();expect(attempts).toBe(0);
  await page.getByRole('button',{name:'Отменить заявку',exact:true}).click();
  await page.getByRole('button',{name:'Подтвердить отмену'}).click();
  await expect(page.getByRole('dialog')).toContainText('Повторите отмену');
  await page.getByRole('button',{name:'Подтвердить отмену'}).click();
  await expect(page.getByText('Отменена',{exact:true})).toBeVisible();
  await expect(page.getByText(/^Отменена:/)).toBeVisible();
  await expect(note).toHaveValue('Несохранённый черновик');
  await expect(page.getByRole('button',{name:'Отменить заявку',exact:true})).toHaveCount(0);
  await page.reload();await expect(note).toHaveValue('Личная заметка <script>');
  await page.getByRole('link',{name:'← Мои заявки'}).click();
  await expect(page.getByText('Отменена',{exact:true})).toBeVisible();
  await expect(page.getByRole('link',{name:'Открыть заявку'})).toBeVisible();
});

test('failed refresh after cancellation can be retried without cancelling again or losing draft', async ({page}) => {
  const trip={id:'refresh-test',origin:{name:'Курск'},destination:{name:'Москва'},departureFrom:'2027-01-01',departureTo:'2027-01-02',adults:1,status:'TRIP_STATUS_SAVED',comment:'Сохранённая заметка',cancelledAt:undefined as string|undefined};
  let cancellations=0;let failRefresh=true;
  await page.route('**/travelwatch.cabinet.v1.TripService/*',async route=>{
    if(route.request().url().endsWith('/CancelTrip')){
      cancellations++;trip.status='TRIP_STATUS_CANCELLED';trip.cancelledAt='2027-01-01T12:30:00Z';await route.fulfill({json:{}});
    } else if(cancellations && failRefresh){
      failRefresh=false;await route.fulfill({status:503,json:{code:'unavailable'}});
    } else await route.fulfill({json:{trip}});
  });
  await page.goto('/#/trips/refresh-test');
  const note=page.getByRole('textbox',{name:'Комментарий к заявке'});
  await note.fill('Черновик');
  await page.getByRole('button',{name:'Отменить заявку',exact:true}).click();
  await page.getByRole('button',{name:'Подтвердить отмену'}).click();
  await expect(page.getByText('Отменена',{exact:true})).toBeVisible();
  await expect(page.getByText(/Заявка отменена, но не удалось обновить/)).toBeVisible();
  await expect(note).toHaveValue('Черновик');
  await page.getByRole('button',{name:'Повторить обновление заявки'}).click();
  await expect(page.getByText(/^Отменена:/)).toBeVisible();
  await expect(note).toHaveValue('Черновик');
  await expect(page.getByRole('button',{name:'Повторить обновление заявки'})).toHaveCount(0);
  expect(cancellations).toBe(1);
});
