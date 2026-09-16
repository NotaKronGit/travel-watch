import {test,expect} from '@playwright/test';

test('route progress refreshes history and duration without losing a comment draft',async({page})=>{
 let ready=false;
 let offline=false;
 const started=new Date(Date.now()-3000).toISOString();
 await page.route('**/travelwatch.cabinet.v1.AuthService/*',r=>r.fulfill({json:{user:{id:'test-user',email:'progress@example.com'}}}));
 await page.route('**/travelwatch.cabinet.v1.TripService/GetTrip',async r=>{
  if(offline){await r.fulfill({status:502,json:{code:'unavailable'}});return;}
  await r.fulfill({json:{trip:{id:'progress-trip',origin:{name:'Курск'},destination:{name:'Паттайя'},departureFrom:'2027-01-10',departureTo:'2027-01-12',adults:1,status:'TRIP_STATUS_RUNNING',comment:'',createdAt:started,buildingStage:ready?'awaiting_schedules':'building',history:[
   {revision:'1',stage:'queued',occurredAt:started},
   {revision:'2',stage:'building',occurredAt:started,startedAt:started,attempt:1},
   ...(ready?[{revision:'3',stage:'awaiting_schedules',occurredAt:new Date().toISOString(),startedAt:started,finishedAt:new Date().toISOString(),attempt:1,durationMs:'42000',routeCount:4,incomplete:true}]:[])
  ]}}});
 });
 await page.goto('/#/trips/progress-trip');
 await expect(page.getByText(/Прошло:/)).toBeVisible();
 await page.getByRole('textbox',{name:'Комментарий к заявке'}).fill('Черновик сохраняется');
 offline=true;
 await expect(page.getByText('Не удалось обновить данные. Повторяем автоматически.')).toBeVisible({timeout:12000});
 await expect(page.getByRole('textbox',{name:'Комментарий к заявке'})).toHaveValue('Черновик сохраняется');
 offline=false;ready=true;
 await expect(page.getByText('Найдено схем до проверки стыковок: 4')).toBeVisible({timeout:12000});
 await expect(page.getByText('Длительность построения: 0 мин 42 с')).toBeVisible();
 await expect(page.getByText(/Прошло:/)).toHaveCount(0);
 await expect(page.getByRole('textbox',{name:'Комментарий к заявке'})).toHaveValue('Черновик сохраняется');
 await expect(page.getByText('Найдено схем до проверки стыковок: 4')).toHaveCount(1);
});
