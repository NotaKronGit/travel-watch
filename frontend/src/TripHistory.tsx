import { useEffect, useState } from 'react';
import { Alert, Divider, Stack, Typography } from '@mui/material';
import { TripStatus, type TripDetails } from './gen/travelwatch/cabinet/v1/trips_pb';

export const buildingLabels: Record<string, string> = {
  queued: 'Ожидает построения маршрутов',
  building: 'Построение маршрутов',
  awaiting_schedules: 'Маршруты построены, ожидает проверки расписаний',
  no_routes: 'Варианты маршрута не найдены в проверенной выборке',
  failed: 'Не удалось завершить построение маршрутов',
  cancelled: 'Построение маршрутов отменено',
};
const ms = (t: {seconds: bigint; nanos: number}) => Number(t.seconds) * 1000 + t.nanos / 1e6;
const elapsed = (value: number) => {
  const seconds = Math.max(0, Math.floor(value / 1000));
  return `${Math.floor(seconds / 60)} мин ${seconds % 60} с`;
};

export function TripHistory({trip}: {trip: TripDetails}) {
  const [now, setNow] = useState(Date.now());
  const active = trip.status !== TripStatus.CANCELLED && trip.status !== TripStatus.COMPLETED && ['queued','building'].includes(trip.buildingStage);
  useEffect(() => {
    if (!active) return;
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [active]);
  const history = [...trip.history].sort((a,b) => a.revision < b.revision ? -1 : a.revision > b.revision ? 1 : 0);
  const latest = history.at(-1);
  return <Stack spacing={2} aria-label="История заявки">
    <Divider/>
    <Typography variant="h6">История заявки</Typography>
    {trip.createdAt && <Typography>Заявка создана · {new Date(ms(trip.createdAt)).toLocaleString('ru-RU')}</Typography>}
    {history.map(item => <Stack key={String(item.revision)} spacing={0.5}>
      <Typography>{buildingLabels[item.stage] || 'Неизвестный этап'}</Typography>
      {item.plannerId && <Typography variant="caption" color="text.secondary">Планировщик: {item.plannerId === 'graph' ? 'Наш алгоритм' : item.plannerId}</Typography>}
      {item.occurredAt && <Typography variant="body2" color="text.secondary">{new Date(ms(item.occurredAt)).toLocaleString('ru-RU')}{item.attempt > 1 ? ` · Попытка ${item.attempt}` : ''}</Typography>}
      {active && item === latest && item.startedAt && <Typography>Прошло: {elapsed(now - ms(item.startedAt))}</Typography>}
      {active && item === latest && item.stage === 'queued' && item.occurredAt && <Typography>В очереди: {elapsed(now - ms(item.occurredAt))}</Typography>}
      {item.finishedAt && <Typography>Длительность построения: {elapsed(Number(item.durationMs))}</Typography>}
      {(item.stage === 'awaiting_schedules' || item.stage === 'no_routes') && <Typography>Найдено схем до проверки стыковок: {item.routeCount}</Typography>}
      {item.stage === 'awaiting_schedules' && <Alert severity="info">Расписания и время стыковок ещё не проверены.{item.incomplete ? ' Выборка маршрутов неполная.' : ''}</Alert>}
      {item.stage === 'no_routes' && <Alert severity="info">Это не означает, что маршрутов нет: покрытие источника и перебор ограничены.</Alert>}
      {item.stage === 'failed' && <Alert severity="warning">Этап завершился ошибкой. Проверка расписаний не запущена.</Alert>}
    </Stack>)}
    {trip.cancelledAt && <Typography>Заявка отменена · {new Date(ms(trip.cancelledAt)).toLocaleString('ru-RU')}</Typography>}
  </Stack>;
}
