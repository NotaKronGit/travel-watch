import { test, expect } from '@playwright/test';

async function stubTrip(page: import('@playwright/test').Page) {
  await page.route('**/travelwatch.cabinet.v1.AuthService/*',route=>route.fulfill({json:{user:{id:'test-user',email:'experiment@example.com'}}}));
  await page.route('**/travelwatch.cabinet.v1.TripService/GetTrip',route=>route.fulfill({json:{trip:{id:'experiment-trip',origin:{id:'origin',name:'Курск',country:'Россия'},destination:{id:'destination',name:'Паттайя',country:'Таиланд'},departureFrom:'2027-01-10',departureTo:'2027-01-12',adults:1,status:'TRIP_STATUS_SAVED'}}}));
}

test('comparison explicitly selects a research report and shows two independent planners',async({page},testInfo)=>{
  await stubTrip(page);
  await page.goto('/#/trips/experiment-trip');
  const comparison=page.getByRole('region',{name:'Сравнение планировщиков',exact:true});
  await expect(comparison).toContainText('не расчёт этой заявки');
  await expect(comparison.getByRole('heading',{name:'GraphPlanner',exact:true})).toHaveCount(0);
  await comparison.getByRole('combobox',{name:'Направление отчёта'}).click();
  await page.getByRole('option',{name:'Курск → Паттайя',exact:true}).click();
  await expect(page.getByRole('listbox',{includeHidden:true})).toHaveCount(0);
  for(const name of ['GraphPlanner','Gemini'])await expect(comparison.getByRole('region',{name,exact:true})).toBeVisible();
  await expect(comparison.getByRole('region',{name:'GraphPlanner',exact:true})).toContainText('Независимый источник транспортных связей ещё не подключён');
  await expect(comparison).not.toContainText('Rome2Rio');
  await page.screenshot({path:testInfo.outputPath('comparison-desktop.png'),fullPage:true});
});

test('comparison fits a mobile screen',async({page},testInfo)=>{
  await page.setViewportSize({width:390,height:844});
  await stubTrip(page);
  await page.goto('/#/trips/experiment-trip');
  const comparison=page.getByRole('region',{name:'Сравнение планировщиков',exact:true});
  await comparison.getByRole('combobox',{name:'Направление отчёта'}).click();
  await page.getByRole('option',{name:'Курск → Паттайя',exact:true}).click();
  await expect(page.getByRole('listbox',{includeHidden:true})).toHaveCount(0);
  await expect(comparison.getByRole('heading',{name:'Gemini',exact:true})).toBeVisible();
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true);
  await page.screenshot({path:testInfo.outputPath('comparison-mobile.png'),fullPage:true});
});

test('missing independent source is not a successful empty search',async({page})=>{
  await stubTrip(page);await page.goto('/#/trips/experiment-trip');
  await page.getByRole('combobox').click();
  await page.getByRole('option').nth(3).click();
  const graph=page.getByRole('region',{name:'GraphPlanner',exact:true});
  await expect(graph.getByRole('alert')).toBeVisible();
  await expect(graph.getByRole('list')).toHaveCount(0);
  await page.reload();
  await expect(page.getByRole('heading',{name:'GraphPlanner',exact:true})).toHaveCount(0);
});
