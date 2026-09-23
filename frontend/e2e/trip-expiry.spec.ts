import {test,expect} from '@playwright/test';
const id='11111111-1111-4111-8111-111111111111';
test('expired trip is terminal, keeps results and stops waiting for checks',async({page})=>{
 await page.setViewportSize({width:390,height:844});
 let scheduleCalls=0;
 await page.route('**/travelwatch.cabinet.v1.AuthService/*',r=>r.fulfill({json:{user:{id:'owner',email:'test@example.com'}}}));
 await page.route('**/travelwatch.cabinet.v1.TripService/GetTrip',r=>r.fulfill({json:{trip:{id,origin:{name:'Курск'},destination:{name:'Паттайя'},departureFrom:'2026-09-21',departureTo:'2026-09-28',adults:1,status:'TRIP_STATUS_EXPIRED',buildingStage:'awaiting_schedules',history:[]}}}));
 await page.route('**/travelwatch.cabinet.v1.TripService/GetTripRoutes',r=>{
  if(r.request().postDataJSON().pageSize===1)scheduleCalls++;
  return r.fulfill({json:{result:{sources:[{plannerId:'graph',stage:'awaiting_schedules',total:1,routes:[{steps:[{description:'Курск → Москва (тестовая схема)',mode:'train'}],warnings:[]}]}],scheduleCheck:{state:'pending'}}}});
 });
 await page.goto('/#/trips/'+id);
 await expect(page.getByText('Истекла: даты поездки прошли',{exact:true})).toBeVisible();
 await expect(page.getByText('Заявка истекла: последний день выезда уже прошёл.',{exact:false})).toBeVisible();
 await expect(page.getByRole('button',{name:/Отменить/})).toHaveCount(0);
 // Saved schemes stay viewable.
 await page.getByRole('button',{name:'Этап 2: Схемы',exact:true}).click();
 await expect(page.getByText('Курск → Москва (тестовая схема)',{exact:false}).first()).toBeVisible();
 await page.getByRole('button',{name:'Этап 3: Стыковки',exact:true}).click();
 await expect(page.getByText('Проверка расписаний не выполнялась: заявка истекла.')).toBeVisible();
 const calls=scheduleCalls;await page.waitForTimeout(5500);expect(scheduleCalls).toBe(calls);
});
