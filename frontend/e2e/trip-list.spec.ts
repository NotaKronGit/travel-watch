import {test,expect} from '@playwright/test';
const trip = {id:'11111111-1111-1111-1111-111111111111',origin:{name:'Курск',country:'Россия',region:'Курская область'},destination:{name:'Бангкок',country:'Таиланд'},departureFrom:'2027-01-10',departureTo:'2027-01-12',adults:2};
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
