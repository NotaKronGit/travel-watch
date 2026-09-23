import { useEffect, useState } from 'react';
import { Alert, Box, Button, Chip, Divider, Stack, ToggleButton, ToggleButtonGroup, Typography } from '@mui/material';
import { Code, ConnectError } from '@connectrpc/connect';
import { useNavigate } from 'react-router-dom';
import { tripClient } from './api';
import type { ScheduleCheck } from './gen/travelwatch/search/v1/routes_pb';
import { checkLabels as labels, connectionText as connection, scheduleToJson, journeySummary, overnight, overnightBefore, overnightNote, schemeLabels, sortLabels, sortSchemes, stationTime, transferCounts, transferLimitText, type JourneyFilter, type JourneySort, expired, expiredCount } from './scheduleExport';
import { copyText, downloadText } from './share';
function at(ts:{seconds:bigint}){return new Date(Number(ts.seconds)*1000).toLocaleString('ru-RU');}
// Journeys shown per scheme before "show all"; sorting decides which ones.
const shownJourneys=3;
export function TripSchedules({id,cancelled,expired:tripExpired=false}:{id:string;cancelled:boolean;expired?:boolean}){
 const [result,setResult]=useState<ScheduleCheck>();
 const [error,setError]=useState('');
 const [retry,setRetry]=useState(0);
 const [shareMessage,setShareMessage]=useState('');
 const [shareError,setShareError]=useState(false);
 const [sort,setSort]=useState<JourneySort>('wait');
 // A re-check in progress: the previous result stays on screen.
 const refreshing=result?.state==='running' && !!result.checkedAt;
 const [expanded,setExpanded]=useState<Set<number>>(new Set());
 // undefined: any number of transfers.
 const [maxTransfers,setMaxTransfers]=useState<number>();
 const counts=result ? transferCounts(result.schemes) : [];
 // After the data reloads the chosen limit may cover every journey; then it filters nothing.
 const limit=maxTransfers!==undefined && counts.some(n=>n>maxTransfers) ? maxTransfers : undefined;
 const [noOvernight,setNoOvernight]=useState(false);
 const [showExpired,setShowExpired]=useState(false);
 // Read once per render; the tab re-renders when the saved result is re-read.
 const now=Date.now();
 const expiredJourneys=result ? expiredCount(result.schemes,now) : 0;
 const anyOvernight=!!result && result.schemes.some(s=>s.journeys.some(overnight));
 const filter:JourneyFilter={maxTransfers:limit,noOvernight:noOvernight && anyOvernight,showExpired,now};
 const visible=result ? sortSchemes(result.schemes,sort,filter) : [];
 // Expired journeys have their own counter; this one covers the transfer and overnight filters.
 const hidden=result ? result.schemes.reduce((n,s)=>n+s.journeys.length,0)-sortSchemes(result.schemes,sort,{...filter,showExpired:true}).reduce((n,s)=>n+s.journeys.length,0) : 0;
 const navigate=useNavigate();
 useEffect(()=>{
  const controller=new AbortController();let timer:ReturnType<typeof setTimeout>|undefined;
  async function load(){
   let again=true;let delay=5000;
   try{
    const response=await tripClient.getTripRoutes({id,pageSize:1,plannerId:'graph'},{signal:controller.signal});
    if(controller.signal.aborted)return;
    setResult(response.result?.scheduleCheck);setError('');
    const state=response.result?.scheduleCheck?.state;
    // An expired trip gets no new checks; waiting for one would poll forever. After a
    // finished check the saved result is re-read once a minute to pick up re-checks;
    // reading never runs a check.
    again=!cancelled && !tripExpired;
    delay=!state || state==='pending' || state==='running' ? 5000 : 60000;
   }catch(err){
    if(controller.signal.aborted)return;
    if(err instanceof ConnectError && err.code===Code.Unauthenticated){navigate('/login');return;}
    if(err instanceof ConnectError && err.code===Code.NotFound){setResult(undefined);again=false;}
    setError('Не удалось загрузить проверку стыковок. Повторите загрузку.');
   }finally{if(!controller.signal.aborted && again)timer=setTimeout(()=>void load(),delay);}
  }
  void load();return()=>{controller.abort();if(timer)clearTimeout(timer);};
 },[id,cancelled,tripExpired,retry,navigate]);
 return <Stack component="section" aria-label="Расписания и стыковки" spacing={2}>
  <Typography component="h2" variant="h5">Расписания и стыковки</Typography>
  <Typography color="text.secondary">Результаты проверки схем нашего алгоритма по датам заявки. Время отправления и прибытия указано по местному времени станции с часовым смещением.</Typography>
  {error && <Alert severity="warning" action={<Button color="inherit" onClick={()=>setRetry(n=>n+1)}>Повторить</Button>}>{error}</Alert>}
  <Chip sx={{alignSelf:'flex-start'}} label={cancelled ? labels.cancelled : refreshing ? 'Обновляем расписания' : labels[result?.state || 'pending'] || 'Состояние неизвестно'}/>
  {(!result || result.state==='pending') && tripExpired && <Alert severity="info">Проверка расписаний не выполнялась: заявка истекла.</Alert>}
  {(!result || result.state==='pending') && !cancelled && !tripExpired && <Alert severity="info">Проверка появится после сохранения схем и запуска обработчика расписаний. Открытие вкладки не запускает запросы к перевозчикам.</Alert>}
  {result?.state==='running' && !refreshing && <Typography role="status">Получаем расписания и проверяем время между участками…</Typography>}
  {refreshing && <Alert role="status" severity="info">Обновляем расписания. Пока показан результат предыдущей проверки.</Alert>}
  {result?.refreshFailedAt && <Alert severity="warning">Не удалось обновить расписания {at(result.refreshFailedAt)}. Показан предыдущий результат; это не означает, что рейсов нет.</Alert>}
  {result?.state==='failed' && <Alert severity="warning">Проверку не удалось завершить. Это не означает, что рейсов нет.</Alert>}
  {result?.checkedAt && <Typography variant="body2">Проверено: {at(result.checkedAt)} · Запросов к источнику: {result.requests}{result.nextCheckAt && !refreshing ? ` · Следующая проверка: около ${at(result.nextCheckAt)}` : ''}</Typography>}
  {result?.incomplete && <Alert severity="warning">Проверка неполная. Результаты относятся только к рассмотренным датам, участкам и сочетаниям.</Alert>}
  {result?.warnings.map((w,i)=><Typography variant="body2" color="text.secondary" key={i}>{w}</Typography>)}
  {result && result.schemes.some(s=>s.journeys.length>0) && <Stack direction="row" sx={{gap:1,flexWrap:'wrap'}}>
   <Button onClick={()=>void copyText(scheduleToJson(result,sort,filter)).then(()=>{setShareError(false);setShareMessage('Стыковки скопированы. Можно переслать сообщение.');},()=>{setShareError(true);setShareMessage('Не удалось скопировать. Скачайте файл JSON и перешлите его.');})}>Скопировать стыковки</Button>
   <Button onClick={()=>{downloadText('travel-watch-connections.json',scheduleToJson(result,sort,filter),'application/json');setShareMessage('');}}>Скачать стыковки JSON</Button>
  </Stack>}
  {shareMessage && <Alert role="status" severity={shareError?'warning':'success'}>{shareMessage}</Alert>}
  {result && result.schemes.some(s=>s.journeys.length>1) && <Stack spacing={0.5}>
   <Typography variant="body2" id="journey-sort">Сортировать сочетания</Typography>
   <ToggleButtonGroup size="small" exclusive value={sort} onChange={(_,value:JourneySort|null)=>{if(value)setSort(value);}} aria-labelledby="journey-sort" sx={{flexWrap:'wrap'}}>
    {(Object.keys(sortLabels) as JourneySort[]).map(key=><ToggleButton key={key} value={key}>{sortLabels[key]}</ToggleButton>)}
   </ToggleButtonGroup>
  </Stack>}
  {counts.length>1 && <Stack spacing={0.5}>
   <Typography variant="body2" id="journey-transfers">Пересадки</Typography>
   <ToggleButtonGroup size="small" exclusive value={limit ?? 'any'} onChange={(_,value:number|'any'|null)=>{if(value!==null)setMaxTransfers(value==='any'?undefined:value);}} aria-labelledby="journey-transfers" sx={{flexWrap:'wrap'}}>
    <ToggleButton value="any">Любое</ToggleButton>
    {counts.slice(0,-1).map(n=><ToggleButton key={n} value={n}>{transferLimitText(n)}</ToggleButton>)}
   </ToggleButtonGroup>
  </Stack>}
  {expiredJourneys>0 && !showExpired && !visible.some(v=>v.journeys.length>0) && <Alert severity="info">Все найденные сочетания уже отправились ({expiredJourneys}). Более поздние даты появятся после повторной проверки расписаний.</Alert>}
  {expiredJourneys>0 && <ToggleButton size="small" value="expired" selected={showExpired} onChange={()=>setShowExpired(v=>!v)} sx={{alignSelf:'flex-start'}}>{showExpired?'Скрыть истекшие':`Показать истекшие (${expiredJourneys})`}</ToggleButton>}
  {anyOvernight && <ToggleButton size="small" value="no-overnight" selected={noOvernight} onChange={()=>setNoOvernight(v=>!v)} sx={{alignSelf:'flex-start'}}>Без ночёвки</ToggleButton>}
  {hidden>0 && <Typography variant="body2" color="text.secondary">Скрыто фильтрами сочетаний: {hidden}</Typography>}
  {result && visible.map(({scheme:s,journeys},i)=>{
   const key=result.schemes.indexOf(s);const all=expanded.has(key);
   return <Box key={key} sx={{p:2,border:'1px solid',borderColor:'divider',borderRadius:2}}>
   <Typography variant="h6">Схема {s.schemeNumber} · Подвоз {s.accessVariant}</Typography>
   <Alert severity={s.state==='compatible'?'success':'info'} sx={{my:1}}>{schemeLabels[s.state] || s.state}</Alert>
   {s.warnings.map((w,j)=><Typography key={j} variant="body2" color="text.secondary" sx={{mb:1}}>{w}</Typography>)}
   {(all ? journeys : journeys.slice(0,shownJourneys)).map((journey,j)=><Stack spacing={1} key={j} sx={{mt:2}}>
    <Divider/>
    <Typography sx={{fontWeight:600}}>Сочетание {j+1}{!journey.timingVerified?' · предварительное':''}{expired(journey,now)?' · истекло: отправление уже прошло':''}{i===0 && j===0 && journeys.length>0?' · лучшее по выбранной сортировке':''}</Typography>
    <Typography variant="body2" color="text.secondary">{journeySummary(journey)}</Typography>
    {journey.legs.map((leg,k)=><Box key={k}>
     {k>0 && <Typography variant="body2">{connection(leg,journey.timingVerified)}</Typography>}
     {k>0 && overnightBefore(journey,k) && <Typography variant="body2" color="warning.main">{overnightNote}</Typography>}
     <Typography>{leg.mode==='train'?'Поезд':'Самолёт'} {leg.number}: {leg.from} → {leg.to}</Typography>
     <Typography variant="body2">Отправление: {stationTime(leg.departure)} · Прибытие: {stationTime(leg.arrival)}</Typography>
     {leg.observedAt && <Typography variant="caption" color="text.secondary">Наблюдение источника: {new Date(Number(leg.observedAt.seconds)*1000).toLocaleString('ru-RU')}</Typography>}
    </Box>)}
   </Stack>)}
   {journeys.length>shownJourneys && <Button sx={{mt:2}} onClick={()=>setExpanded(prev=>{const next=new Set(prev);if(all)next.delete(key);else next.add(key);return next;})}>{all?'Свернуть':`Показать все (${journeys.length})`}</Button>}
  </Box>;})}
  <Typography variant="caption">Источник расписаний: <a href="https://rasp.yandex.ru/" target="_blank" rel="noreferrer">Яндекс Расписания</a>. Цены и наличие мест не проверены. Перед покупкой нужно уточнить расписание и условия перевозчика.</Typography>
 </Stack>;
}
