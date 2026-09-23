import {test,expect} from '@playwright/test';
const id='11111111-1111-4111-8111-111111111111';
test('timetable tab preserves station time, shows the transfer window and stops polling after completion',async({page})=>{
 await page.setViewportSize({width:390,height:844});let ready=false;let calls=0;
 await page.route('**/travelwatch.cabinet.v1.AuthService/*',r=>r.fulfill({json:{user:{id:'owner',email:'test@example.com'}}}));
 await page.route('**/travelwatch.cabinet.v1.TripService/GetTrip',r=>r.fulfill({json:{trip:{id,origin:{name:'Курск'},destination:{name:'Сочи'},departureFrom:'2027-01-10',departureTo:'2027-01-10',adults:1,status:'TRIP_STATUS_RUNNING',buildingStage:'awaiting_schedules',history:[]}}}));
 await page.route('**/travelwatch.cabinet.v1.TripService/GetTripRoutes',r=>{
  const q=r.request().postDataJSON();if(q.pageSize!==1)return r.fulfill({json:{result:{sources:[]}}});
  calls++;
  return r.fulfill({json:{result:{sources:[],scheduleCheck:ready?{state:'done',incomplete:true,checkedAt:'2026-09-20T10:00:00Z',requests:12,warnings:['Тестовые расписания'],schemes:[{schemeNumber:1,accessVariant:2,state:'compatible',warnings:[],journeys:[{timingVerified:true,legs:[{from:'Курск',to:'Москва',mode:'train',number:'ТЕСТ-123',departure:'2027-01-10T23:00:00+03:00',arrival:'2027-01-11T06:00:00+03:00'},{from:'Тестовый аэропорт',to:'Бангкок',mode:'plane',number:'ТЕСТ-456',departure:'2027-01-11T17:01:00+03:00',arrival:'2027-01-12T06:00:00+07:00',connectionMinutes:'661',requiredMinutes:'255',transferFrom:'Тестовый вокзал',transferTo:'Тестовый аэропорт',boardingMinutes:'180'}]}]}]}:{state:'running'}}}});
 });
 await page.goto('/#/trips/'+id);
 await page.getByRole('button',{name:'Этап 3: Стыковки',exact:true}).click();
 await expect(page.getByText('Получаем расписания и проверяем время между участками…')).toBeVisible();ready=true;
 await expect(page.getByText('Время согласуется с заданными запасами',{exact:true})).toBeVisible({timeout:10000});
 await expect(page.getByText('Схема 1 · Подвоз 2')).toBeVisible();
 await expect(page.getByText('Отправление:',{exact:false}).first()).toContainText('23:00');
 await expect(page.getByText('Отправление:',{exact:false}).first()).toContainText('+03:00');
 await expect(page.getByText('Переезд Тестовый вокзал → Тестовый аэропорт: на переезд 8 ч 1 мин (11 ч 1 мин между участками − 3 ч на регистрацию)',{exact:true})).toBeVisible();
 await expect(page.getByText('Неизвестно время переезда:',{exact:false})).toHaveCount(0);
 const last=calls;await page.waitForTimeout(5500);expect(calls).toBe(last);
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBeTruthy();
 await page.screenshot({path:'test-results/schedules-mobile.png',fullPage:true});
});
