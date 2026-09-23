import { useEffect, useState } from 'react';
import { Alert, Box, Button, Chip, Divider, Stack, Typography } from '@mui/material';
import { Code, ConnectError } from '@connectrpc/connect';
import { useNavigate } from 'react-router-dom';
import { tripClient } from './api';
import type { ScheduleCheck } from './gen/travelwatch/search/v1/routes_pb';
import { checkLabels as labels, connectionText as connection, formatScheduleExport, schemeLabels, stationTime } from './scheduleExport';
import { copyText, downloadText } from './share';
export function TripSchedules({id,cancelled}:{id:string;cancelled:boolean}){
 const [result,setResult]=useState<ScheduleCheck>();
 const [error,setError]=useState('');
 const [retry,setRetry]=useState(0);
 const [shareMessage,setShareMessage]=useState('');
 const [shareError,setShareError]=useState(false);
 const navigate=useNavigate();
 useEffect(()=>{
  const controller=new AbortController();let timer:ReturnType<typeof setTimeout>|undefined;
  async function load(){
   let again=true;
   try{
    const response=await tripClient.getTripRoutes({id,pageSize:1,plannerId:'graph'},{signal:controller.signal});
    if(controller.signal.aborted)return;
    setResult(response.result?.scheduleCheck);setError('');
    const state=response.result?.scheduleCheck?.state;
    again=!cancelled && (!state || state==='pending' || state==='running');
   }catch(err){
    if(controller.signal.aborted)return;
    if(err instanceof ConnectError && err.code===Code.Unauthenticated){navigate('/login');return;}
    if(err instanceof ConnectError && err.code===Code.NotFound){setResult(undefined);again=false;}
    setError('Не удалось загрузить проверку стыковок. Повторите загрузку.');
   }finally{if(!controller.signal.aborted && again)timer=setTimeout(()=>void load(),5000);}
  }
  void load();return()=>{controller.abort();if(timer)clearTimeout(timer);};
 },[id,cancelled,retry,navigate]);
 return <Stack component="section" aria-label="Расписания и стыковки" spacing={2}>
  <Typography component="h2" variant="h5">Расписания и стыковки</Typography>
  <Typography color="text.secondary">Результаты проверки схем нашего алгоритма по датам заявки. Время отправления и прибытия указано по местному времени станции с часовым смещением.</Typography>
  {error && <Alert severity="warning" action={<Button color="inherit" onClick={()=>setRetry(n=>n+1)}>Повторить</Button>}>{error}</Alert>}
  <Chip sx={{alignSelf:'flex-start'}} label={cancelled ? labels.cancelled : labels[result?.state || 'pending'] || 'Состояние неизвестно'}/>
  {(!result || result.state==='pending') && !cancelled && <Alert severity="info">Проверка появится после сохранения схем и запуска обработчика расписаний. Открытие вкладки не запускает запросы к перевозчикам.</Alert>}
  {result?.state==='running' && <Typography role="status">Получаем расписания и проверяем время между участками…</Typography>}
  {result?.state==='failed' && <Alert severity="warning">Проверку не удалось завершить. Это не означает, что рейсов нет.</Alert>}
  {result?.checkedAt && <Typography variant="body2">Проверено: {new Date(Number(result.checkedAt.seconds)*1000).toLocaleString('ru-RU')} · Запросов к источнику: {result.requests}</Typography>}
  {result?.incomplete && <Alert severity="warning">Проверка неполная. Результаты относятся только к рассмотренным датам, участкам и сочетаниям.</Alert>}
  {result?.warnings.map((w,i)=><Typography variant="body2" color="text.secondary" key={i}>{w}</Typography>)}
  {result && result.schemes.some(s=>s.journeys.length>0) && <Stack direction="row" sx={{gap:1,flexWrap:'wrap'}}>
   <Button onClick={()=>void copyText(formatScheduleExport(result)).then(()=>{setShareError(false);setShareMessage('Стыковки скопированы. Можно переслать сообщение.');},()=>{setShareError(true);setShareMessage('Не удалось скопировать. Скачайте TXT и перешлите файл.');})}>Скопировать стыковки</Button>
   <Button onClick={()=>{downloadText('travel-watch-connections.txt',formatScheduleExport(result));setShareMessage('');}}>Скачать стыковки TXT</Button>
  </Stack>}
  {shareMessage && <Alert role="status" severity={shareError?'warning':'success'}>{shareMessage}</Alert>}
  {result?.schemes.map((s,i)=><Box key={i} sx={{p:2,border:'1px solid',borderColor:'divider',borderRadius:2}}>
   <Typography variant="h6">Схема {s.schemeNumber} · Подвоз {s.accessVariant}</Typography>
   <Alert severity={s.state==='compatible'?'success':'info'} sx={{my:1}}>{schemeLabels[s.state] || s.state}</Alert>
   {s.warnings.map((w,j)=><Typography key={j} variant="body2" color="text.secondary" sx={{mb:1}}>{w}</Typography>)}
   {s.journeys.map((journey,j)=><Stack spacing={1} key={j} sx={{mt:2}}>
    <Divider/>
    <Typography sx={{fontWeight:600}}>Сочетание {j+1}{!journey.timingVerified?' · предварительное':''}</Typography>
    {journey.legs.map((leg,k)=><Box key={k}>
     {k>0 && <Typography variant="body2">{connection(leg,journey.timingVerified)}</Typography>}
     <Typography>{leg.mode==='train'?'Поезд':'Самолёт'} {leg.number}: {leg.from} → {leg.to}</Typography>
     <Typography variant="body2">Отправление: {stationTime(leg.departure)} · Прибытие: {stationTime(leg.arrival)}</Typography>
     {leg.observedAt && <Typography variant="caption" color="text.secondary">Наблюдение источника: {new Date(Number(leg.observedAt.seconds)*1000).toLocaleString('ru-RU')}</Typography>}
    </Box>)}
   </Stack>)}
  </Box>)}
  <Typography variant="caption">Источник расписаний: <a href="https://rasp.yandex.ru/" target="_blank" rel="noreferrer">Яндекс Расписания</a>. Цены и наличие мест не проверены. Перед покупкой нужно уточнить расписание и условия перевозчика.</Typography>
 </Stack>;
}
