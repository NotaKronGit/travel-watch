import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest';
import { create } from '@bufbuild/protobuf';
import { ScheduleCheckSchema } from './gen/travelwatch/search/v1/routes_pb';
import { expired, journeySummary, overnight, overnightBefore, scheduleToJson, sortSchemes, transferCounts, transferLimitText, transfersText, travelMinutes, waitMinutes } from './scheduleExport';

// Fixtures use January 2027; pin "now" before them so expiry does not depend on the real date.
beforeAll(() => { vi.useFakeTimers({ toFake: ['Date'] }); vi.setSystemTime(new Date('2027-01-01T00:00:00Z')); });
afterAll(() => { vi.useRealTimers(); });

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

describe('scheduleToJson', () => {
  it('exports found journeys with station time, transfer window and caveats', () => {
    const json = JSON.parse(scheduleToJson(check));
    expect(json.format).toBe('travel-watch.connections.v1');
    expect(json.notice[0]).toContain('Цены и наличие мест не проверены');
    expect(json).toMatchObject({ incomplete: true, warnings: ['Тестовое предупреждение'], sort: 'wait', maxTransfers: null, noOvernight: false, hiddenByFilter: 0, schemesWithoutJourneys: 1 });
    expect(json.schemes).toHaveLength(1);
    const scheme = json.schemes[0];
    expect(scheme).toMatchObject({ schemeNumber: 1, accessVariant: 2, stateName: 'Время согласуется с заданными запасами' });
    const journey = scheme.journeys[0];
    expect(journey).toMatchObject({ number: 1, preliminary: false, transfers: 1, waitMinutes: 661 });
    expect(journey.legs[0]).toMatchObject({ mode: 'train', number: 'ТЕСТ-1', departure: '2027-01-10T23:00:00+03:00', connectionBefore: null });
    expect(journey.legs[1].arrival).toBe('2027-01-12T06:00:00+07:00');
    expect(journey.legs[1].connectionBefore).toEqual({ minutes: 661, requiredMinutes: 255,
      transfer: { from: 'Тестовый вокзал', to: 'Тестовый аэропорт', boardingMinutes: 180, availableMinutes: 481 },
      text: 'Переезд Тестовый вокзал → Тестовый аэропорт: на переезд 8 ч 1 мин (11 ч 1 мин между участками − 3 ч на регистрацию)', overnight: false });
    expect(journey.overnight).toBe(false);
  });
  it('exports an empty list when nothing connected', () => {
    const json = JSON.parse(scheduleToJson(create(ScheduleCheckSchema, { state: 'done', schemes: [{ schemeNumber: 1, state: 'no_match' }] })));
    expect(json.schemes).toEqual([]);
    expect(json.schemesWithoutJourneys).toBe(1);
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
    const json = JSON.parse(scheduleToJson(sorting, 'wait'));
    expect(json.sortName).toBe('Меньше ожидание');
    expect(json.schemes.map((s: { accessVariant: number }) => s.accessVariant)).toEqual([3, 1]);
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
    expect(sortSchemes(check.schemes, 'wait', { maxTransfers: 1 }).map(s => s.scheme.schemeNumber)).toEqual([1]);
    const json = JSON.parse(scheduleToJson(check, 'wait', { maxTransfers: 1 }));
    expect(json).toMatchObject({ maxTransfers: 1, hiddenByFilter: 1 });
    expect(JSON.stringify(json.schemes)).not.toContain('Р-3');
  });
});

describe('overnight connections', () => {
  // Synthetic train + flight with the connection in a chosen window.
  const trip = (arrival: string, departure: string, boarding = 0n) => create(ScheduleCheckSchema, { state: 'done', schemes: [{ schemeNumber: 1, accessVariant: 4, state: 'compatible', journeys: [{ timingVerified: true, legs: [
    { from: 'А', to: 'Б', mode: 'train', number: 'П-1', departure: '2027-01-10T12:29:00+03:00', arrival },
    { from: 'Б', to: 'В', mode: 'plane', number: 'Р-1', departure, arrival: '2027-01-12T11:45:00+07:00', connectionMinutes: BigInt(Math.round((Date.parse(departure) - Date.parse(arrival)) / 60000)), boardingMinutes: boarding },
  ] }] }] }).schemes[0].journeys[0];
  it('marks a wait that covers the night window at the connection', () => {
    // 22:40 to 22:25 next day, like train 143Й and SU 272: free until 19:25.
    expect(overnightBefore(trip('2027-01-10T22:40:00+03:00', '2027-01-11T22:25:00+03:00', 180n), 1)).toBe(true);
    // Night flight: at 22:00 in town, check-in from 23:45 for 02:45, so the night is spent flying.
    expect(overnightBefore(trip('2027-01-10T22:00:00+03:00', '2027-01-11T02:45:00+03:00', 180n), 1)).toBe(false);
    // Night train at 06:00 with 30 min boarding: free until 05:30.
    expect(overnightBefore(trip('2027-01-10T23:00:00+03:00', '2027-01-11T06:00:00+03:00', 30n), 1)).toBe(true);
    // Same-airport layover without boarding data counts to departure.
    expect(overnightBefore(trip('2027-01-11T02:00:00+03:00', '2027-01-11T04:00:00+03:00'), 1)).toBe(true);
    // Daytime 11 h wait, and a late evening one ending before 01:00.
    expect(overnightBefore(trip('2027-01-11T06:00:00+03:00', '2027-01-11T17:01:00+03:00'), 1)).toBe(false);
    expect(overnightBefore(trip('2027-01-10T22:40:00+03:00', '2027-01-11T00:59:00+03:00'), 1)).toBe(false);
  });
  it('uses the local clock of the connection station', () => {
    // 22:00 UTC is 01:00 in +03:00 but 23:00 in +01:00.
    expect(overnightBefore(trip('2027-01-10T23:30:00+03:00', '2027-01-11T01:30:00+03:00'), 1)).toBe(true);
    expect(overnightBefore(trip('2027-01-10T21:30:00+01:00', '2027-01-10T23:30:00+01:00'), 1)).toBe(false);
  });
  it('shows in the summary and can be filtered out', () => {
    const night = trip('2027-01-10T22:40:00+03:00', '2027-01-11T22:25:00+03:00', 180n);
    expect(overnight(night)).toBe(true);
    expect(journeySummary(night)).toMatch(/^1 пересадка, ожидание 23 ч 45 мин с ночёвкой, в пути/);
    const check = create(ScheduleCheckSchema, { state: 'done', schemes: [{ schemeNumber: 1, accessVariant: 4, state: 'compatible', journeys: [night] }] });
    expect(sortSchemes(check.schemes, 'wait', { noOvernight: true })).toEqual([]);
    const json = JSON.parse(scheduleToJson(check, 'wait', { noOvernight: true }));
    expect(json).toMatchObject({ noOvernight: true, nightWindow: '01:00–05:00', hiddenByFilter: 1, schemes: [] });
    expect(JSON.parse(scheduleToJson(check)).schemes[0].journeys[0].legs[1].connectionBefore.overnight).toBe(true);
  });
});

describe('expired journeys', () => {
  const at = (departure: string) => create(ScheduleCheckSchema, { state: 'done', schemes: [{ schemeNumber: 1, accessVariant: 1, state: 'compatible', journeys: [{ timingVerified: true, legs: [
    { from: 'А', to: 'Б', mode: 'train', number: 'П-1', departure, arrival: '2027-01-12T10:00:00+03:00' }] }] }] });
  it('hides a journey once its first departure has passed, unless asked', () => {
    const check = at('2027-01-10T21:00:00+03:00');
    const before = Date.parse('2027-01-10T17:59:00Z'), after = Date.parse('2027-01-10T18:00:00Z');
    expect(expired(check.schemes[0].journeys[0], before)).toBe(false);
    expect(expired(check.schemes[0].journeys[0], after)).toBe(true);
    expect(sortSchemes(check.schemes, 'wait', { now: after })).toEqual([]);
    expect(sortSchemes(check.schemes, 'wait', { now: after, showExpired: true })).toHaveLength(1);
    const json = JSON.parse(scheduleToJson(check, 'wait', { now: after }));
    expect(json).toMatchObject({ showExpired: false, expiredJourneys: 1, hiddenByFilter: 1, schemes: [] });
    expect(JSON.parse(scheduleToJson(check, 'wait', { now: after, showExpired: true })).schemes[0].journeys[0].expired).toBe(true);
  });
});
