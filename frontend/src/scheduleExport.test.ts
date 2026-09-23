import { describe, expect, it } from 'vitest';
import { create } from '@bufbuild/protobuf';
import { ScheduleCheckSchema } from './gen/travelwatch/search/v1/routes_pb';
import { formatScheduleExport, journeySummary, sortSchemes, transferCounts, transferLimitText, transfersText, travelMinutes, waitMinutes } from './scheduleExport';

// Synthetic check: one scheme with a train + flight journey, one scheme without journeys.
const check = create(ScheduleCheckSchema, {
  state: 'done', incomplete: true, requests: 12, warnings: ['Тестовое предупреждение'],
  schemes: [
    { schemeNumber: 1, accessVariant: 2, state: 'compatible', warnings: ['Перебор сочетаний ограничен; показаны не все варианты.'], journeys: [{ timingVerified: true, legs: [
      { from: 'Тестовый город', to: 'Тестовый вокзал', mode: 'train', number: 'ТЕСТ-1', departure: '2027-01-10T23:00:00+03:00', arrival: '2027-01-11T06:00:00+03:00' },
      { from: 'Тестовый аэропорт', to: 'Бангкок', mode: 'plane', number: 'ТЕСТ-2', departure: '2027-01-11T17:01:00+03:00', arrival: '2027-01-12T06:00:00+07:00', connectionMinutes: 661n, requiredMinutes: 255n, transferFrom: 'Тестовый вокзал', transferTo: 'Тестовый аэропорт', boardingMinutes: 180n },
    ] }] },
    { schemeNumber: 2, accessVariant: 1, state: 'no_match', journeys: [] },
  ],
});

describe('formatScheduleExport', () => {
  it('lists found journeys with station time, transfer window and caveats', () => {
    const text = formatScheduleExport(check);
    expect(text).toContain('Цены и наличие мест не проверены');
    expect(text).toContain('Проверка неполная.');
    expect(text).toContain('Предупреждение: Тестовое предупреждение');
    expect(text).toContain('Схема 1 · Подвоз 2\nВремя согласуется с заданными запасами');
    expect(text).toContain('1. Поезд ТЕСТ-1: Тестовый город → Тестовый вокзал\n   Отправление: 2027-01-10 23:00 +03:00');
    expect(text).toContain('   Переезд Тестовый вокзал → Тестовый аэропорт: на переезд 8 ч 1 мин (11 ч 1 мин между участками − 3 ч на регистрацию)\n2. Самолёт ТЕСТ-2');
    expect(text).toContain('Прибытие: 2027-01-12 06:00 +07:00');
    expect(text).not.toContain('Схема 2');
    expect(text).toContain('Схем без подходящих сочетаний: 1.');
  });
  it('says so when nothing connected', () => {
    const text = formatScheduleExport(create(ScheduleCheckSchema, { state: 'done', schemes: [{ schemeNumber: 1, state: 'no_match' }] }));
    expect(text).toContain('Состыкованных сочетаний нет.');
  });
});

// Synthetic journeys: train then flight, differing in waiting time and train speed.
function journey(train: [string, string], flight: [string, string], wait: bigint) {
  return { timingVerified: true, legs: [
    { from: 'Тестовый город', to: 'Тестовый вокзал', mode: 'train', number: `П-${train[0]}`, departure: train[0], arrival: train[1] },
    { from: 'Тестовый аэропорт', to: 'Бангкок', mode: 'plane', number: 'ТЕСТ-2', departure: flight[0], arrival: flight[1], connectionMinutes: wait },
  ] };
}
const flight: [string, string] = ['2027-01-11T22:25:00+03:00', '2027-01-12T11:45:00+07:00'];
const sorting = create(ScheduleCheckSchema, { state: 'done', schemes: [
  { schemeNumber: 1, accessVariant: 1, state: 'compatible', journeys: [
    journey(['2027-01-11T03:19:00+03:00', '2027-01-11T13:45:00+03:00'], flight, 520n),
    journey(['2027-01-11T00:42:00+03:00', '2027-01-11T08:03:00+03:00'], flight, 862n),
  ] },
  { schemeNumber: 1, accessVariant: 2, state: 'no_match', journeys: [] },
  { schemeNumber: 1, accessVariant: 3, state: 'compatible', journeys: [
    journey(['2027-01-11T05:36:00+03:00', '2027-01-11T11:24:00+03:00'], flight, 661n),
    journey(['2027-01-11T09:14:00+03:00', '2027-01-11T14:37:00+03:00'], flight, 468n),
  ] },
] });

describe('sortSchemes', () => {
  const first = (by: 'wait' | 'duration' | 'departure') => sortSchemes(sorting.schemes, by).map(s => `${s.scheme.accessVariant}:${s.journeys.map(j => j.legs[0].number).join(',')}`);
  it('orders journeys and access variants by waiting, keeping empty ones last', () => {
    expect(first('wait')).toEqual(['3:П-2027-01-11T09:14:00+03:00,П-2027-01-11T05:36:00+03:00', '1:П-2027-01-11T03:19:00+03:00,П-2027-01-11T00:42:00+03:00', '2:']);
  });
  it('orders by first departure and by travel time across time zones', () => {
    expect(first('departure')[0]).toBe('1:П-2027-01-11T00:42:00+03:00,П-2027-01-11T03:19:00+03:00');
    // 09:14 MSK to 11:45 +07 next day is 22 h 31 min, the shortest here.
    expect(travelMinutes(sorting.schemes[2].journeys[1])).toBe(22 * 60 + 31);
    expect(first('duration')[0].startsWith('3:П-2027-01-11T09:14')).toBe(true);
  });
  it('sums waiting between legs and summarises it', () => {
    expect(waitMinutes(sorting.schemes[2].journeys[1])).toBe(468);
    expect(journeySummary(sorting.schemes[2].journeys[1])).toBe('1 пересадка, ожидание 7 ч 48 мин, в пути 22 ч 31 мин');
  });
  it('exports in the chosen order', () => {
    const text = formatScheduleExport(sorting, 'wait');
    expect(text).toContain('Сортировка: меньше ожидание');
    expect(text.indexOf('Подвоз 3')).toBeLessThan(text.indexOf('Подвоз 1'));
  });
});

describe('transfer filter', () => {
  // Synthetic: a train + flight (1 transfer) and a train + two flights (2 transfers).
  const leg = (number: string, departure: string, arrival: string, wait?: bigint) => ({ from: 'А', to: 'Б', mode: number.startsWith('П') ? 'train' : 'plane', number, departure, arrival, connectionMinutes: wait ?? 0n });
  const check = create(ScheduleCheckSchema, { state: 'done', schemes: [
    { schemeNumber: 1, accessVariant: 1, state: 'compatible', journeys: [{ timingVerified: true, legs: [
      leg('П-1', '2027-01-11T09:00:00+03:00', '2027-01-11T14:00:00+03:00'), leg('Р-1', '2027-01-11T22:00:00+03:00', '2027-01-12T10:00:00+07:00', 480n)] }] },
    { schemeNumber: 2, accessVariant: 1, state: 'compatible', journeys: [{ timingVerified: true, legs: [
      leg('П-2', '2027-01-11T09:00:00+03:00', '2027-01-11T14:00:00+03:00'), leg('Р-2', '2027-01-11T18:00:00+03:00', '2027-01-11T20:00:00+03:00', 240n), leg('Р-3', '2027-01-11T21:00:00+03:00', '2027-01-12T08:00:00+07:00', 60n)] }] },
  ] });
  it('counts transfers and offers the counts present', () => {
    expect(transferCounts(check.schemes)).toEqual([1, 2]);
    expect(journeySummary(check.schemes[1].journeys[0])).toMatch(/^2 пересадки, ожидание 5 ч, в пути/);
    expect([0, 1, 2, 5, 11, 21].map(transfersText)).toEqual(['без пересадок', '1 пересадка', '2 пересадки', '5 пересадок', '11 пересадок', '21 пересадка']);
    expect([0, 1, 2, 11, 21].map(transferLimitText)).toEqual(['Только без пересадок', 'Не больше 1 пересадки', 'Не больше 2 пересадок', 'Не больше 11 пересадок', 'Не больше 21 пересадки']);
  });
  it('drops longer journeys and their emptied schemes, on screen and in export', () => {
    // Unfiltered, the 2-transfer journey waits less (5 h vs 8 h) and goes first.
    expect(sortSchemes(check.schemes, 'wait').map(s => s.scheme.schemeNumber)).toEqual([2, 1]);
    expect(sortSchemes(check.schemes, 'wait', 1).map(s => s.scheme.schemeNumber)).toEqual([1]);
    const text = formatScheduleExport(check, 'wait', 1);
    expect(text).toContain('Фильтр: не больше 1 пересадки. Не выгружено сочетаний с большим числом пересадок: 1.');
    expect(text).not.toContain('Р-3');
  });
});
