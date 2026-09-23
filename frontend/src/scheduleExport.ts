import type { ScheduleCheck, ScheduledLeg } from './gen/travelwatch/search/v1/routes_pb';
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

function legText(leg: ScheduledLeg) {
  return `${leg.mode === 'train' ? 'Поезд' : 'Самолёт'} ${leg.number}: ${leg.from} → ${leg.to}`;
}

// Plain-text export of every scheme with found journeys, with the same caveats as the tab.
export function formatScheduleExport(result: ScheduleCheck): string {
  const lines = ['Travel Watch — расписания и стыковки',
    'Источник расписаний: Яндекс Расписания. Цены и наличие мест не проверены. Перед покупкой нужно уточнить расписание и условия перевозчика.',
    'Время отправления и прибытия — местное время станции с часовым смещением.',
    `Состояние: ${checkLabels[result.state] || result.state}`];
  if (result.checkedAt) lines.push(`Проверено: ${new Date(Number(result.checkedAt.seconds) * 1000).toLocaleString('ru-RU')} · Запросов к источнику: ${result.requests}`);
  if (result.incomplete) lines.push('Проверка неполная. Результаты относятся только к рассмотренным датам, участкам и сочетаниям.');
  result.warnings.forEach(w => lines.push(`Предупреждение: ${w}`));
  const found = result.schemes.filter(s => s.journeys.length > 0);
  const without = result.schemes.length - found.length;
  if (!found.length) lines.push('', 'Состыкованных сочетаний нет.');
  for (const scheme of found) {
    lines.push('', `Схема ${scheme.schemeNumber} · Подвоз ${scheme.accessVariant}`, schemeLabels[scheme.state] || scheme.state);
    scheme.warnings.forEach(w => lines.push(`Требует проверки: ${w}`));
    scheme.journeys.forEach((journey, j) => {
      lines.push('', `Сочетание ${j + 1}${!journey.timingVerified ? ' · предварительное' : ''}`);
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
