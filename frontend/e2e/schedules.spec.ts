import {test,expect} from '@playwright/test';
const id='11111111-1111-4111-8111-111111111111';
test('timetable tab preserves station time, shows the transfer window and stops polling after completion',async({page})=>{
 await page.setViewportSize({width:390,height:844});let ready=false;let calls=0;
 await page.route('**/travelwatch.cabinet.v1.AuthService/*',r=>r.fulfill({json:{user:{id:'owner',email:'test@example.com'}}}));
 await page.route('**/travelwatch.cabinet.v1.TripService/GetTrip',r=>r.fulfill({json:{trip:{id,origin:{name:'Курск'},destination:{name:'Сочи'},departureFrom:'2027-01-10',departureTo:'2027-01-10',adults:1,status:'TRIP_STATUS_RUNNING',buildingStage:'awaiting_schedules',history:[]}}}));
 await page.route('**/travelwatch.cabinet.v1.TripService/GetTripRoutes',r=>{
  const q=r.request().postDataJSON();if(q.pageSize!==1)return r.fulfill({json:{result:{sources:[]}}});
  calls++;
  return r.fulfill({json:{result:{sources:[],scheduleCheck:ready?{state:'done',incomplete:true,checkedAt:'2026-09-20T10:00:00Z',requests:12,warnings:['Тестовые расписания'],schemes:[{schemeNumber:1,accessVariant:2,state:'compatible',warnings:[],journeys:[{timingVerified:true,legs:[{from:'Курск',to:'Москва',mode:'train',number:'ТЕСТ-123',departure:'2027-01-10T23:00:00+03:00',arrival:'2027-01-11T06:00:00+03:00'},{from:'Тестовый аэропорт',to:'Бангкок',mode:'plane',number:'ТЕСТ-456',departure:'2027-01-11T17:01:00+03:00',arrival:'2027-01-12T06:00:00+07:00',connectionMinutes:'661',requiredMinutes:'255',transferFrom:'Тестовый вокзал',transferTo:'Тестовый аэропорт',boardingMinutes:'180'}]}]},{schemeNumber:1,accessVariant:3,state:'compatible',warnings:[],journeys:[
   {timingVerified:true,legs:[{from:'Курск',to:'Москва',mode:'train',number:'ТЕСТ-ДОЛГО',departure:'2027-01-10T03:00:00+03:00',arrival:'2027-01-10T09:00:00+03:00'},{from:'Тестовый аэропорт',to:'Бангкок',mode:'plane',number:'ТЕСТ-456',departure:'2027-01-10T22:25:00+03:00',arrival:'2027-01-11T11:45:00+07:00',connectionMinutes:'805',requiredMinutes:'255',transferFrom:'Тестовый вокзал',transferTo:'Тестовый аэропорт',boardingMinutes:'180'}]},
   {timingVerified:true,legs:[{from:'Курск',to:'Москва',mode:'train',number:'ТЕСТ-БЫСТРО',departure:'2027-01-10T09:14:00+03:00',arrival:'2027-01-10T14:37:00+03:00'},{from:'Тестовый аэропорт',to:'Бангкок',mode:'plane',number:'ТЕСТ-456',departure:'2027-01-10T22:25:00+03:00',arrival:'2027-01-11T11:45:00+07:00',connectionMinutes:'468',requiredMinutes:'255',transferFrom:'Тестовый вокзал',transferTo:'Тестовый аэропорт',boardingMinutes:'180'}]}]},{schemeNumber:1,accessVariant:4,state:'compatible',warnings:[],journeys:[
   {timingVerified:true,legs:[{from:'Курск',to:'Москва',mode:'train',number:'ТЕСТ-НОЧЬ',departure:'2027-01-10T12:29:00+03:00',arrival:'2027-01-10T22:40:00+03:00'},{from:'Тестовый аэропорт',to:'Бангкок',mode:'plane',number:'ТЕСТ-456',departure:'2027-01-11T22:25:00+03:00',arrival:'2027-01-12T11:45:00+07:00',connectionMinutes:'1425',requiredMinutes:'255',transferFrom:'Тестовый вокзал',transferTo:'Тестовый аэропорт',boardingMinutes:'180'}]}]}]}:{state:'running'}}}});
 });
 await page.goto('/#/trips/'+id);
 await page.getByRole('button',{name:'Этап 3: Стыковки',exact:true}).click();
 await expect(page.getByText('Получаем расписания и проверяем время между участками…')).toBeVisible();ready=true;
 await expect(page.getByText('Время согласуется с заданными запасами',{exact:true}).first()).toBeVisible({timeout:10000});
 await expect(page.getByText('Схема 1 · Подвоз 2')).toBeVisible();
 // Default sort is by waiting: variant 3 (7 h 48 min) goes above variant 2 (11 h 1 min), its faster train first.
 await expect(page.getByRole('button',{name:'Меньше ожидание',exact:true})).toHaveAttribute('aria-pressed','true');
 const headings=page.getByText(/^Схема \d+ · Подвоз \d+$/);
 await expect(headings.first()).toHaveText('Схема 1 · Подвоз 3');
 await expect(page.getByText('1 пересадка, ожидание 7 ч 48 мин, в пути',{exact:false})).toBeVisible();
 await expect(page.getByText('Пересадки',{exact:true})).toHaveCount(0);
 const trains=page.getByText(/^Поезд ТЕСТ-/);
 await expect(trains.first()).toContainText('ТЕСТ-БЫСТРО');
 await page.getByRole('button',{name:'Раньше отправление',exact:true}).click();
 await expect(trains.first()).toContainText('ТЕСТ-ДОЛГО');
 await page.getByRole('button',{name:'Меньше ожидание',exact:true}).click();
 await expect(page.getByText('Свободное время до регистрации или посадки захватывает ночь',{exact:false})).toHaveCount(1);
 await expect(page.getByText('ожидание 23 ч 45 мин с ночёвкой',{exact:false})).toBeVisible();
 await page.getByRole('button',{name:'Без ночёвки',exact:true}).click();
 await expect(page.getByText('Схема 1 · Подвоз 4')).toHaveCount(0);
 await expect(page.getByText('Скрыто фильтрами сочетаний: 1')).toBeVisible();
 await page.getByRole('button',{name:'Без ночёвки',exact:true}).click();
 await expect(page.getByText('Схема 1 · Подвоз 4')).toBeVisible();
 await expect(page.getByText('Отправление: 2027-01-10 23:00 +03:00',{exact:false})).toBeVisible();
 await expect(page.getByText('Переезд Тестовый вокзал → Тестовый аэропорт: на переезд 8 ч 1 мин (11 ч 1 мин между участками − 3 ч на регистрацию)',{exact:true})).toBeVisible();
 await expect(page.getByText('Неизвестно время переезда:',{exact:false})).toHaveCount(0);
 await page.evaluate(()=>Object.defineProperty(navigator,'clipboard',{configurable:true,value:{writeText:async(text:string)=>{(window as unknown as {copied:string}).copied=text;}}}));
 await page.getByRole('button',{name:'Скопировать стыковки',exact:true}).click();
 await expect(page.getByText('Стыковки скопированы.',{exact:false})).toBeVisible();
 const copied=await page.evaluate(()=>(window as unknown as {copied:string}).copied);
 const json=JSON.parse(copied);
 expect(json.format).toBe('travel-watch.connections.v1');
 expect(json.schemes.map((s:{accessVariant:number})=>s.accessVariant)).toEqual([3,2,4]);
 expect(json.schemes[2].journeys[0].overnight).toBe(true);
 expect(json.schemes[0].journeys[0].legs[0].number).toBe('ТЕСТ-БЫСТРО');
 const variant2=json.schemes[1].journeys[0];
 expect(variant2.legs[0].departure).toBe('2027-01-10T23:00:00+03:00');
 expect(variant2.legs[1].connectionBefore.transfer).toEqual({from:'Тестовый вокзал',to:'Тестовый аэропорт',boardingMinutes:180,availableMinutes:481});
 const downloadPromise=page.waitForEvent('download');
 await page.getByRole('button',{name:'Скачать стыковки JSON',exact:true}).click();
 const download=await downloadPromise;
 expect(download.suggestedFilename()).toBe('travel-watch-connections.json');
 const chunks:Buffer[]=[];for await(const chunk of (await download.createReadStream())!)chunks.push(Buffer.from(chunk));
 expect(Buffer.concat(chunks).toString('utf8')).toBe(copied);
 const last=calls;await page.waitForTimeout(5500);expect(calls).toBe(last);
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBeTruthy();
 await page.screenshot({path:'test-results/schedules-mobile.png',fullPage:true});
});
