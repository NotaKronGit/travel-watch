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

// Journeys within each scheme, and schemes by their best journey; schemes without
// journeys keep their order at the end. With maxTransfers, longer journeys are dropped
// and schemes left without journeys by the filter are omitted. Only reorders saved data.
export function sortSchemes(schemes: ScheduledScheme[], by: JourneySort, maxTransfers?: number): { scheme: ScheduledScheme; journeys: ScheduledJourney[] }[] {
  const sorted = schemes.map(scheme => ({ scheme, journeys: scheme.journeys.filter(j => maxTransfers === undefined || transferCount(j) <= maxTransfers).sort((a, b) => compareJourneys(a, b, by)) }));
  const found = sorted.filter(s => s.journeys.length > 0).sort((a, b) => compareJourneys(a.journeys[0], b.journeys[0], by));
  return [...found, ...sorted.filter(s => s.journeys.length === 0 && (maxTransfers === undefined || s.scheme.journeys.length === 0))];
}

// One line to compare journeys: waiting between legs and first departure to last arrival.
export function journeySummary(journey: ScheduledJourney) {
  return `${transfersText(transferCount(journey))}, ожидание ${formatMinutes(waitMinutes(journey))}, в пути ${formatMinutes(Math.round(travelMinutes(journey)))}`;
}

function legText(leg: ScheduledLeg) {
  return `${leg.mode === 'train' ? 'Поезд' : 'Самолёт'} ${leg.number}: ${leg.from} → ${leg.to}`;
}

// Plain-text export of every scheme with found journeys, with the same caveats as the tab.
export function formatScheduleExport(result: ScheduleCheck, by: JourneySort = 'wait', maxTransfers?: number): string {
  const lines = ['Travel Watch — расписания и стыковки',
    'Источник расписаний: Яндекс Расписания. Цены и наличие мест не проверены. Перед покупкой нужно уточнить расписание и условия перевозчика.',
    'Время отправления и прибытия — местное время станции с часовым смещением.',
    `Состояние: ${checkLabels[result.state] || result.state}`];
  if (result.checkedAt) lines.push(`Проверено: ${new Date(Number(result.checkedAt.seconds) * 1000).toLocaleString('ru-RU')} · Запросов к источнику: ${result.requests}`);
  if (result.incomplete) lines.push('Проверка неполная. Результаты относятся только к рассмотренным датам, участкам и сочетаниям.');
  result.warnings.forEach(w => lines.push(`Предупреждение: ${w}`));
  const found = sortSchemes(result.schemes, by, maxTransfers).filter(s => s.journeys.length > 0);
  const without = result.schemes.filter(s => s.journeys.length === 0).length;
  const hidden = result.schemes.reduce((n, s) => n + s.journeys.length, 0) - found.reduce((n, s) => n + s.journeys.length, 0);
  if (maxTransfers !== undefined) lines.push(`Фильтр: ${transferLimitText(maxTransfers).toLowerCase()}. Не выгружено сочетаний с большим числом пересадок: ${hidden}.`);
  if (!found.length) lines.push('', 'Состыкованных сочетаний нет.');
  else lines.push(`Сортировка: ${sortLabels[by].toLowerCase()}`);
  for (const { scheme, journeys } of found) {
    lines.push('', `Схема ${scheme.schemeNumber} · Подвоз ${scheme.accessVariant}`, schemeLabels[scheme.state] || scheme.state);
    scheme.warnings.forEach(w => lines.push(`Требует проверки: ${w}`));
    journeys.forEach((journey, j) => {
      lines.push('', `Сочетание ${j + 1}${!journey.timingVerified ? ' · предварительное' : ''} · ${journeySummary(journey)}`);
      journey.legs.forEach((leg, k) => {
        if (k > 0) lines.push(`   ${connectionText(leg, journey.timingVerified)}`);
        lines.push(`${k + 1}. ${legText(leg)}`, `   Отправление: ${stationTime(leg.departure)} · Прибытие: ${stationTime(leg.arrival)}`);
      });
      journey.warnings.forEach(w => lines.push(`Требует проверки: ${w}`));
    });
  }
  if (without > 0) lines.push('', `Схем без подходящих сочетаний: ${without}. Они не выгружаются.`);
  return lines.join('\n');
}
