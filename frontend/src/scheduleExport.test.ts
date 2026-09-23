import { describe, expect, it } from 'vitest';
import { create } from '@bufbuild/protobuf';
import { ScheduleCheckSchema } from './gen/travelwatch/search/v1/routes_pb';
import { formatScheduleExport } from './scheduleExport';

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
