import { useEffect, useState } from 'react';
import { Title } from 'react-admin';
import { Alert, Box, Button, Checkbox, FormControlLabel, Chip, Paper, Stack, Typography } from '@mui/material';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { Code, ConnectError } from '@connectrpc/connect';
import { tripClient } from './api';
import { TripStatus, type TripDetails } from './gen/travelwatch/cabinet/v1/trips_pb';

import { TripSchedules } from './TripSchedules';
import { TripStages } from './TripStages';
import { TripRouteResults } from './TripRouteResults';
import { TripActions } from './TripActions';
import { TripHistory, buildingLabels } from './TripHistory';
const statuses: Record<number,string> = {
  [TripStatus.SAVED]: 'Сохранена, поиск ещё не запущен',
  [TripStatus.RUNNING]: 'Выполняется',
  [TripStatus.CANCELLED]: 'Отменена',
  [TripStatus.COMPLETED]: 'Завершена',
};
const date = (s: string) => s.split('-').reverse().join('.');
function TripSummary({trip}: {trip: TripDetails}) {
  return <Stack spacing={2}>
    <Typography variant="h6" sx={{overflowWrap:'anywhere'}}>{trip.origin?.name} → {trip.destination?.name}</Typography>
    <Typography color="text.secondary">{[trip.origin?.country,trip.origin?.region].filter(Boolean).join(', ')} → {[trip.destination?.country,trip.destination?.region].filter(Boolean).join(', ')}</Typography>
    <Typography>Выезд с {date(trip.departureFrom)} по {date(trip.departureTo)} включительно · Взрослых: {trip.adults}</Typography>
    <Chip label={(trip.status === TripStatus.CANCELLED || trip.status === TripStatus.COMPLETED ? statuses[trip.status] : (trip.buildingStage==='awaiting_schedules'?'Схемы маршрутов построены':buildingLabels[trip.buildingStage]) || statuses[trip.status]) || 'Статус неизвестен'} sx={{alignSelf:'flex-start',maxWidth:'100%',height:'auto', '& .MuiChip-label':{whiteSpace:'normal',py:1},bgcolor:'#eef2e5',color:'#183e38'}}/>
  </Stack>;
}
export function TripsPage({detail = false}: {detail?: boolean}) {
  const {id = ''} = useParams();
  return <TripContent key={detail ? id : 'list'} id={detail ? id : undefined}/>;
}
function TripContent({id}: {id?: string}) {
  const [offset,setOffset] = useState(0);
  const [includeInactive,setIncludeInactive] = useState(false);
  const [retry,setRetry] = useState(0);
  const [result,setResult] = useState<{trips:TripDetails[];more:boolean;offset:number} | null>(null);
  const [loading,setLoading] = useState(true);
  const [error,setError] = useState('');
  const [missing,setMissing] = useState(false);
  const navigate = useNavigate();
  useEffect(() => {
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    let initial = true;
    setLoading(true);setError('');setMissing(false);
    async function load() {
      try {
        const r = id === undefined
          ? await tripClient.listTrips({offset,includeInactive},{signal:controller.signal}).then(r => ({trips:r.trips,more:r.hasMore,offset}))
          : await tripClient.getTrip({id},{signal:controller.signal}).then(r => {if (!r.trip) throw new Error('Missing trip'); return {trips:[r.trip],more:false,offset};});
        if (!controller.signal.aborted) {
          setResult(current => ({...r, trips:r.trips.map(next => mergeTrip(current?.trips.find(old => old.id === next.id),next))}));
          setError('');setMissing(false);
        }
      } catch (err) {
        if (controller.signal.aborted) return;
        if (err instanceof ConnectError && err.code === Code.Unauthenticated) {navigate('/login');return;}
        const notFound = err instanceof ConnectError && err.code === Code.NotFound;
        setMissing(notFound);setError(notFound ? 'Заявка не найдена' : 'Не удалось обновить данные. Повторяем автоматически.');
      } finally {
        if (!controller.signal.aborted) {
          if (initial) {setLoading(false);initial=false;}
          timer=setTimeout(() => { if (document.visibilityState === 'hidden') { timer=setTimeout(() => void load(),5000); } else {void load();} },5000);
        }
      }
    }
    void load();
    return () => {controller.abort();if(timer)clearTimeout(timer);};
  },[id,offset,includeInactive,retry,navigate]);
  const hasResult = result?.offset === offset;
  const title = id === undefined ? 'Мои заявки' : 'Заявка';
  return <Box sx={{maxWidth:1000,mx:'auto',p:{xs:2,md:4}}}>
    <Title title={title}/>
    <Stack direction="row" sx={{justifyContent:'space-between',alignItems:'center',flexWrap:'wrap',gap:2,mb:3}}>
      <Typography component="h1" variant="h4" sx={{fontWeight:650}}>{title}</Typography>
      <Button component={Link} to="/trips/create" variant="contained">Создать заявку</Button>
    </Stack>
    {id === undefined && <FormControlLabel sx={{mb:2}} control={<Checkbox checked={includeInactive} onChange={(_,checked)=>{setIncludeInactive(checked);setOffset(0);setResult(null);}}/>} label="Показать завершённые и отменённые"/>}
    {id !== undefined && <Button component={Link} to="/trips" sx={{mb:2}}>← Мои заявки</Button>}
    {loading ? <Typography role="status">Загружаем заявки…</Typography> : (missing || (error && !hasResult)) ? <Stack spacing={2}><Alert severity={missing ? 'info' : 'error'}>{missing ? error : 'Не удалось загрузить данные. Повторите попытку.'}</Alert>{!missing && <Button onClick={() => setRetry(n=>n+1)}>Повторить загрузку</Button>}</Stack> : result?.offset === offset && <Stack spacing={2}>
      {error && <Alert severity="warning">{error}</Alert>}
      {result.trips.length === 0 && <Paper variant="outlined" sx={{p:4,borderRadius:4}}><Typography variant="h6">{includeInactive ? 'У вас пока нет заявок' : 'У вас пока нет активных заявок'}</Typography><Typography color="text.secondary" sx={{mt:1}}>Создайте первую заявку, чтобы сохранить параметры поездки.</Typography></Paper>}
      {result.trips.map(trip => <Paper key={trip.id} variant="outlined" sx={{p:{xs:2,md:3},borderRadius:4}}>
        <TripSummary trip={trip}/>
        {id === undefined ? <Button component={Link} to={`/trips/${trip.id}`} sx={{mt:2}}>Открыть заявку</Button> : <Stack spacing={2} sx={{mt:3}}>
          <Typography variant="caption" sx={{overflowWrap:'anywhere'}}>Номер заявки: {trip.id}</Typography>
          <TripActions trip={trip} onChange={updated=>setResult(current=>current ? {...current,trips:current.trips.map(item=>item.id===updated.id ? mergeTrip(item,updated) : item)} : current)}/>
          <TripStages stage={trip.buildingStage} cancelled={trip.status===TripStatus.CANCELLED || trip.status===TripStatus.COMPLETED} creation={<Stack spacing={2}>
          <Typography component="h2" variant="h6">Параметры заявки</Typography>
          <Typography>Поездка в одну сторону. Диапазон относится к выезду, обратный билет не включён.</Typography>
          <Alert severity="info">Поиск билетов и уведомления ещё не подключены.</Alert>
          {trip.cancelledAt && <Typography variant="body2" color="text.secondary">Отменена: {new Date(Number(trip.cancelledAt.seconds)*1000).toLocaleString('ru-RU')}</Typography>}
          {trip.createdAt && <Typography variant="body2" color="text.secondary">Создана: {new Date(Number(trip.createdAt.seconds)*1000).toLocaleString('ru-RU')}</Typography>}
          </Stack>} routes={<TripRouteResults key={trip.id} id={trip.id} revision={String(trip.history.reduce((revision,event)=>event.revision>revision?event.revision:revision,0n))}/>} schedules={<TripSchedules key={trip.id} id={trip.id} cancelled={trip.status===TripStatus.CANCELLED}/>} footer={<TripHistory trip={trip}/>}/>
        </Stack>}
      </Paper>)}
    </Stack>}
    {id === undefined && <Stack direction="row" spacing={2} sx={{mt:3,alignItems:'center'}}>
      <Button disabled={loading || offset === 0} onClick={()=>setOffset(n=>Math.max(0,n-20))}>Назад</Button>
      <Typography>Страница {offset/20+1}</Typography>
      <Button disabled={loading || !!error || result?.offset !== offset || !result?.more} onClick={()=>setOffset(n=>n+20)}>Далее</Button>
    </Stack>}
  </Box>;
}

// A response started before cancellation must not reactivate the UI.
function mergeTrip(previous: TripDetails | undefined, next: TripDetails): TripDetails {
 if (!previous) return next;
 const terminal=previous.status===TripStatus.CANCELLED || previous.status===TripStatus.COMPLETED;
 const previousRevision=previous.history.reduce((v,e)=>e.revision>v?e.revision:v,0n);
 const nextRevision=next.history.reduce((v,e)=>e.revision>v?e.revision:v,0n);
 return {...next,
   ...(terminal ? {status:previous.status,cancelledAt:previous.cancelledAt || next.cancelledAt} : {}),
   ...(previousRevision>nextRevision ? {buildingStage:previous.buildingStage,history:previous.history} : {}),
 };
}
