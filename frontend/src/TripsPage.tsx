import { useEffect, useState } from 'react';
import { Title } from 'react-admin';
import { Alert, Box, Button, Chip, Paper, Stack, Typography } from '@mui/material';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { Code, ConnectError } from '@connectrpc/connect';
import { tripClient } from './api';
import type { TripDetails } from './gen/travelwatch/cabinet/v1/trips_pb';

const status = 'Сохранена, поиск ещё не запущен';
const date = (s: string) => s.split('-').reverse().join('.');
function TripSummary({trip}: {trip: TripDetails}) {
  return <Stack spacing={2}>
    <Typography variant="h6" sx={{overflowWrap:'anywhere'}}>{trip.origin?.name} → {trip.destination?.name}</Typography>
    <Typography color="text.secondary">{[trip.origin?.country,trip.origin?.region].filter(Boolean).join(', ')} → {[trip.destination?.country,trip.destination?.region].filter(Boolean).join(', ')}</Typography>
    <Typography>Выезд с {date(trip.departureFrom)} по {date(trip.departureTo)} включительно · Взрослых: {trip.adults}</Typography>
    <Chip label={status} sx={{alignSelf:'flex-start',maxWidth:'100%',height:'auto', '& .MuiChip-label':{whiteSpace:'normal',py:1},bgcolor:'#eef2e5',color:'#183e38'}}/>
  </Stack>;
}
export function TripsPage({detail = false}: {detail?: boolean}) {
  const {id = ''} = useParams();
  return <TripContent key={detail ? id : 'list'} id={detail ? id : undefined}/>;
}
function TripContent({id}: {id?: string}) {
  const [offset,setOffset] = useState(0);
  const [retry,setRetry] = useState(0);
  const [result,setResult] = useState<{trips:TripDetails[];more:boolean;offset:number} | null>(null);
  const [loading,setLoading] = useState(true);
  const [error,setError] = useState('');
  const [missing,setMissing] = useState(false);
  const navigate = useNavigate();
  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);setError('');setMissing(false);
    const request = id === undefined
      ? tripClient.listTrips({offset},{signal:controller.signal}).then(r => ({trips:r.trips,more:r.hasMore,offset}))
      : tripClient.getTrip({id},{signal:controller.signal}).then(r => {if (!r.trip) throw new Error('Missing trip'); return {trips:[r.trip],more:false,offset};});
    request.then(r => {if (!controller.signal.aborted) setResult(r);}).catch(err => {
      if (controller.signal.aborted) return;
      if (err instanceof ConnectError && err.code === Code.Unauthenticated) {navigate('/login');return;}
      const notFound = err instanceof ConnectError && err.code === Code.NotFound;
      setMissing(notFound);setError(notFound ? 'Заявка не найдена' : 'Не удалось загрузить данные. Повторите попытку.');
    }).finally(() => {if (!controller.signal.aborted) setLoading(false);});
    return () => controller.abort();
  },[id,offset,retry,navigate]);
  const title = id === undefined ? 'Мои заявки' : 'Заявка';
  return <Box sx={{maxWidth:1000,mx:'auto',p:{xs:2,md:4}}}>
    <Title title={title}/>
    <Stack direction="row" sx={{justifyContent:'space-between',alignItems:'center',flexWrap:'wrap',gap:2,mb:3}}>
      <Typography component="h1" variant="h4" sx={{fontWeight:650}}>{title}</Typography>
      <Button component={Link} to="/trips/create" variant="contained">Создать заявку</Button>
    </Stack>
    {id !== undefined && <Button component={Link} to="/trips" sx={{mb:2}}>← Мои заявки</Button>}
    {loading ? <Typography role="status">Загружаем заявки…</Typography> : error ? <Stack spacing={2}><Alert severity={missing ? 'info' : 'error'}>{error}</Alert>{!missing && <Button onClick={() => setRetry(n=>n+1)}>Повторить загрузку</Button>}</Stack> : result?.offset === offset && <Stack spacing={2}>
      {result.trips.length === 0 && <Paper variant="outlined" sx={{p:4,borderRadius:4}}><Typography variant="h6">У вас пока нет заявок</Typography><Typography color="text.secondary" sx={{mt:1}}>Создайте первую заявку, чтобы сохранить параметры поездки.</Typography></Paper>}
      {result.trips.map(trip => <Paper key={trip.id} variant="outlined" sx={{p:{xs:2,md:3},borderRadius:4}}>
        <TripSummary trip={trip}/>
        {id === undefined ? <Button component={Link} to={`/trips/${trip.id}`} sx={{mt:2}}>Открыть заявку</Button> : <Stack spacing={2} sx={{mt:3}}>
          <Typography>Поездка в одну сторону. Диапазон относится к выезду, обратный билет не включён.</Typography>
          <Alert severity="info">Поиск билетов и уведомления ещё не подключены.</Alert>
          {trip.createdAt && <Typography variant="body2" color="text.secondary">Создана: {new Date(Number(trip.createdAt.seconds)*1000).toLocaleString('ru-RU')}</Typography>}
          <Typography variant="caption" sx={{overflowWrap:'anywhere'}}>Номер заявки: {trip.id}</Typography>
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
