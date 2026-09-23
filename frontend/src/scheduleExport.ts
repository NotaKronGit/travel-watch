import type { ScheduleCheck, ScheduledJourney, ScheduledLeg, ScheduledScheme } from './gen/travelwatch/search/v1/routes_pb';
import { formatMinutes } from './duration';

export const checkLabels: Record<string, string> = { pending: 'Ожидает проверки', running: 'Проверяем расписания', done: 'Проверка выполнена', failed: 'Проверка не завершена', cancelled: 'Заявка отменена' };
export const schemeLabels: Record<string, string> = { compatible: 'Время согласуется с заданными запасами', unverified: 'Не удалось подтвердить стыковку', no_match: 'Подходящих сочетаний в проверенной выборке нет' };

// Preserve station-local time and its UTC offset; browser timezone must not shift it.
export function stationTime(value: string) { return value.replace('T', ' ').replace(/:00(?=[+-]|Z$)/, ' '); }

// The transfer duration is the traveller's call: show what is left after check-in or boarding.
export function connectionText(leg: ScheduledLeg, verified: boolean) {
  if (leg.transferFrom) {
    const boarding = leg.mode === 'train' ? 'на посадку' : 'на регистрацию';
    return `Переезд ${leg.transferFrom} → ${leg.transferTo}: на переезд ${formatMinutes(leg.connectionMinutes - leg.boardingMinutes)} (${formatMinutes(leg.connectionMinutes)} между участками − ${formatMinutes(leg.boardingMinutes)} ${boarding})`;
  }
  return `Между участками: ${formatMinutes(leg.connectionMinutes)} · Учтённый минимум: ${formatMinutes(leg.requiredMinutes)}${!verified ? ' (без неизвестных переездов)' : ''}`;
}

export type JourneySort = 'wait' | 'duration' | 'departure';
export const sortLabels: Record<JourneySort, string> = { wait: 'Меньше ожидание', duration: 'Меньше время в пути', departure: 'Раньше отправление' };

// Changes between timetable legs; a ground transfer is part of a change, not an extra one.
export function transferCount(journey: ScheduledJourney) { return Math.max(0, journey.legs.length - 1); }
export function transfersText(n: number) {
  if (n === 0) return 'без пересадок';
  const tens = n % 100, ones = n % 10;
  const word = tens >= 11 && tens <= 14 ? 'пересадок' : ones === 1 ? 'пересадка' : ones >= 2 && ones <= 4 ? 'пересадки' : 'пересадок';
  return `${n} ${word}`;
}
// Filter wording in the genitive: "не больше 1 пересадки", "не больше 2 пересадок".
export function transferLimitText(n: number) {
  if (n === 0) return 'Только без пересадок';
  return `Не больше ${n} ${n % 10 === 1 && n % 100 !== 11 ? 'пересадки' : 'пересадок'}`;
}
// Distinct transfer counts present in the check, ascending: the filter offers only these.
export function transferCounts(schemes: ScheduledScheme[]) {
  return [...new Set(schemes.flatMap(s => s.journeys.map(transferCount)))].sort((a, b) => a - b);
}

// Night hours by the connection's local clock: a wait covering any part of them may
// need somewhere to sleep. A display policy estimate, not a carrier or hotel rule.
export const nightWindow = { fromHour: 1, toHour: 5 };
function offsetMinutes(value: string) {
  const m = /([+-])(\d{2}):(\d{2})$/.exec(value);
  return m ? (m[1] === '-' ? -1 : 1) * (Number(m[2]) * 60 + Number(m[3])) : 0;
}
// Whether the free time before leg k (k > 0) overlaps the night window, on the clock of
// the station the previous leg arrives at (both ends of a connection are in one city).
// Free time ends when check-in or boarding starts: a 22:00 arrival for a 02:45 flight
// is at the airport by 23:45 and needs no bed. Boarding time comes with a ground
// transfer; without one (same airport) the wait runs to departure.
export function overnightBefore(journey: ScheduledJourney, k: number) {
  const arrival = journey.legs[k - 1]?.arrival, departure = journey.legs[k]?.departure;
  if (!arrival || !departure) return false;
  const shift = offsetMinutes(arrival) * 60000;
  const from = Date.parse(arrival) + shift;
  const to = Date.parse(departure) + shift - Number(journey.legs[k].boardingMinutes) * 60000;
  const day = 24 * 3600000;
  for (let start = Math.floor(from / day) * day; start < to; start += day) {
    if (start + nightWindow.fromHour * 3600000 < to && start + nightWindow.toHour * 3600000 > from) return true;
  }
  return false;
}
export function overnight(journey: ScheduledJourney) {
  return journey.legs.some((_, k) => k > 0 && overnightBefore(journey, k));
}
export const overnightNote = `Свободное время до регистрации или посадки захватывает ночь (${String(nightWindow.fromHour).padStart(2, '0')}:00–${String(nightWindow.toHour).padStart(2, '0')}:00 по местному времени): может понадобиться ночлег.`;

// Waiting between legs, including the ground transfer the traveller plans themselves.
export function waitMinutes(journey: ScheduledJourney) {
  return journey.legs.slice(1).reduce((sum, leg) => sum + Number(leg.connectionMinutes), 0);
}
function departureTime(journey: ScheduledJourney) { return Date.parse(journey.legs[0]?.departure ?? ''); }
// From the first departure to the last arrival; offsets make the instants comparable.
export function travelMinutes(journey: ScheduledJourney) {
  return (Date.parse(journey.legs[journey.legs.length - 1]?.arrival ?? '') - departureTime(journey)) / 60000;
}
function sortValue(journey: ScheduledJourney, by: JourneySort) {
  if (by === 'wait') return waitMinutes(journey);
  if (by === 'duration') return travelMinutes(journey);
  return departureTime(journey);
}
function compareJourneys(a: ScheduledJourney, b: ScheduledJourney, by: JourneySort) {
  return sortValue(a, by) - sortValue(b, by) || departureTime(a) - departureTime(b);
}

// A journey whose first departure has passed can no longer be taken. Kept in the data
// for history and future notifications, hidden unless asked for.
export function expired(journey: ScheduledJourney, now: number) {
  const departure = Date.parse(journey.legs[0]?.departure ?? '');
  return !Number.isNaN(departure) && departure <= now;
}
export function expiredCount(schemes: ScheduledScheme[], now: number) {
  return schemes.reduce((n, s) => n + s.journeys.filter(j => expired(j, now)).length, 0);
}

// now defaults to the current time; showExpired keeps journeys already departed.
export type JourneyFilter = { maxTransfers?: number; noOvernight?: boolean; showExpired?: boolean; now?: number };
function matches(journey: ScheduledJourney, filter: JourneyFilter) {
  return (filter.maxTransfers === undefined || transferCount(journey) <= filter.maxTransfers) && (!filter.noOvernight || !overnight(journey))
    && (filter.showExpired || !expired(journey, filter.now ?? Date.now()));
}

// Journeys within each scheme, and schemes by their best journey; schemes without
// journeys keep their order at the end. Filtered-out journeys are dropped and schemes
// left without journeys by the filter are omitted. Only reorders saved data.
export function sortSchemes(schemes: ScheduledScheme[], by: JourneySort, filter: JourneyFilter = {}): { scheme: ScheduledScheme; journeys: ScheduledJourney[] }[] {
  const sorted = schemes.map(scheme => ({ scheme, journeys: scheme.journeys.filter(j => matches(j, filter)).sort((a, b) => compareJourneys(a, b, by)) }));
  const found = sorted.filter(s => s.journeys.length > 0).sort((a, b) => compareJourneys(a.journeys[0], b.journeys[0], by));
  return [...found, ...sorted.filter(s => s.journeys.length === 0 && s.scheme.journeys.length === 0)];
}

// One line to compare journeys: waiting between legs and first departure to last arrival.
export function journeySummary(journey: ScheduledJourney) {
  return `${transfersText(transferCount(journey))}, ожидание ${formatMinutes(waitMinutes(journey))}${overnight(journey) ? ' с ночёвкой' : ''}, в пути ${formatMinutes(Math.round(travelMinutes(journey)))}`;
}

// Present only between legs: waiting, check-in or boarding, and what is left for the ground transfer.
function connectionJson(leg: ScheduledLeg) {
  return {
    minutes: Number(leg.connectionMinutes),
    requiredMinutes: Number(leg.requiredMinutes),
    transfer: leg.transferFrom ? {
      from: leg.transferFrom, to: leg.transferTo,
      boardingMinutes: Number(leg.boardingMinutes),
      availableMinutes: Number(leg.connectionMinutes - leg.boardingMinutes),
    } : null,
    text: connectionText(leg, true),
  };
}

// JSON export of every scheme with found journeys, in the on-screen order and filter,
// with the same caveats as the tab. Times keep the station's UTC offset.
export function scheduleToJson(result: ScheduleCheck, by: JourneySort = 'wait', filter: JourneyFilter = {}): string {
  const now = filter.now ?? Date.now();
  filter = { ...filter, now };
  const found = sortSchemes(result.schemes, by, filter).filter(s => s.journeys.length > 0);
  const hidden = result.schemes.reduce((n, s) => n + s.journeys.length, 0) - found.reduce((n, s) => n + s.journeys.length, 0);
  return JSON.stringify({
    format: 'travel-watch.connections.v1',
    notice: ['Источник расписаний: Яндекс Расписания. Цены и наличие мест не проверены. Перед покупкой нужно уточнить расписание и условия перевозчика.',
      'Время отправления и прибытия — местное время станции с часовым смещением.'],
    state: result.state,
    stateName: checkLabels[result.state] || result.state,
    checkedAt: result.checkedAt ? new Date(Number(result.checkedAt.seconds) * 1000).toISOString() : null,
    requests: result.requests,
    incomplete: result.incomplete,
    warnings: result.warnings,
    sort: by,
    sortName: sortLabels[by],
    maxTransfers: filter.maxTransfers ?? null,
    noOvernight: filter.noOvernight ?? false,
    showExpired: filter.showExpired ?? false,
    expiredJourneys: expiredCount(result.schemes, now),
    nightWindow: `${String(nightWindow.fromHour).padStart(2, '0')}:00–${String(nightWindow.toHour).padStart(2, '0')}:00`,
    hiddenByFilter: hidden,
    schemesWithoutJourneys: result.schemes.filter(s => s.journeys.length === 0).length,
    schemes: found.map(({ scheme, journeys }) => ({
      schemeNumber: scheme.schemeNumber,
      accessVariant: scheme.accessVariant,
      state: scheme.state,
      stateName: schemeLabels[scheme.state] || scheme.state,
      warnings: scheme.warnings,
      journeys: journeys.map((journey, j) => ({
        number: j + 1,
        preliminary: !journey.timingVerified,
        transfers: transferCount(journey),
        overnight: overnight(journey),
        expired: expired(journey, now),
        waitMinutes: waitMinutes(journey),
        travelMinutes: Math.round(travelMinutes(journey)),
        summary: journeySummary(journey),
        warnings: journey.warnings,
        legs: journey.legs.map((leg, k) => ({
          mode: leg.mode, number: leg.number, from: leg.from, to: leg.to,
          departure: leg.departure, arrival: leg.arrival,
          observedAt: leg.observedAt ? new Date(Number(leg.observedAt.seconds) * 1000).toISOString() : null,
          connectionBefore: k > 0 ? { ...connectionJson(leg), overnight: overnightBefore(journey, k) } : null,
        })),
      })),
    })),
  }, null, 2);
}
