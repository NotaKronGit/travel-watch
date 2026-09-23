import {test,expect} from '@playwright/test';
test.beforeEach(async ({page})=>{ await page.route('**/travelwatch.cabinet.v1.TripService/GetTripRoutes',r=>r.fulfill({json:{result:{sources:[]}}})); });

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

test('source result leaves the overall stage building until the other source finishes',async({page})=>{
 let done=false;
 const started=new Date(Date.now()-3000).toISOString();
 await page.route('**/travelwatch.cabinet.v1.AuthService/*',r=>r.fulfill({json:{user:{id:'user',email:'test@example.com'}}}));
 await page.route('**/travelwatch.cabinet.v1.TripService/GetTrip',r=>r.fulfill({json:{trip:{id:'multi',status:'TRIP_STATUS_RUNNING',buildingStage:done?'awaiting_schedules':'building',history:[
  {revision:'1',plannerId:'all',stage:'building',startedAt:started,attempt:1},
  {revision:'2',plannerId:'graph',stage:'building',startedAt:started,attempt:1},
  {revision:'3',plannerId:'gemini',stage:'failed',startedAt:started,finishedAt:started,outcome:'timeout',incomplete:true,attempt:1},
  ...(done?[
   {revision:'4',plannerId:'graph',stage:'awaiting_schedules',startedAt:started,finishedAt:started,routeCount:3,attempt:1},
   {revision:'5',plannerId:'all',stage:'awaiting_schedules',startedAt:started,finishedAt:started,routeCount:3,incomplete:true,attempt:1},
  ]:[])
 ]}}}));
 await page.goto('/#/trips/multi');
 await expect(page.getByText('Источник не ответил за отведённое время.',{exact:false})).toBeVisible();
 await expect(page.getByText('Планировщик: Gemini',{exact:true})).toBeVisible();
 await expect(page.getByText('Планировщик: Общий этап',{exact:true})).toBeVisible();
 await expect(page.getByText(/Прошло:/)).toHaveCount(2);
 await expect(page.getByText('Маршруты построены, ожидает проверки расписаний',{exact:true})).toHaveCount(0);
 done=true;
 await expect(page.getByText('Маршруты построены, ожидает проверки расписаний',{exact:true})).toHaveCount(2,{timeout:12000});
 await expect(page.getByText('Схемы маршрутов построены',{exact:true})).toBeVisible();
 await expect(page.getByText(/Прошло:/)).toHaveCount(0);
 await expect(page.getByText('Найдено схем до проверки стыковок: 3',{exact:true})).toHaveCount(2);
});
