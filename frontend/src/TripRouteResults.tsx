import { useEffect, useState } from 'react';
import { Alert, Button, Stack, Typography } from '@mui/material';
import { Code, ConnectError } from '@connectrpc/connect';
import { useNavigate } from 'react-router-dom';
import { tripClient } from './api';
import { TripRoutes, type RouteSourceView } from './TripRoutes';
import { collectAllRoutes } from './routeExport';
import type { SourceRoutes } from './gen/travelwatch/search/v1/routes_pb';
const pageSize=5;
// Largest page Cabinet accepts; used only for export.
const exportPageSize=10;
function view(source: SourceRoutes):RouteSourceView {
 return {id:source.plannerId,stage:source.stage,outcome:source.outcome,durationMs:Number(source.durationMs),
  finishedAt:source.finishedAt ? new Date(Number(source.finishedAt.seconds)*1000).toISOString():undefined,
  incomplete:source.incomplete,total:source.total,offset:source.offset,hasMore:source.hasMore,
  routes:source.routes.map(route=>({steps:route.steps.map(step=>({description:step.description,mode:step.mode,evidence:step.evidence})),warnings:route.warnings})),warnings:source.warnings};
}
export function TripRouteResults({id,revision}:{id:string;revision:string}){
 const [sources,setSources]=useState<RouteSourceView[]|null>(null);
 const [offsets,setOffsets]=useState<Record<string,number>>({});
 const [retry,setRetry]=useState(0);
 const [error,setError]=useState('');
 const [loading,setLoading]=useState(true);
 const navigate=useNavigate();
 useEffect(()=>{
  const controller=new AbortController();
  setLoading(true);setError('');
  async function load(){
   try{
    const pages=Object.entries(offsets);
    const queries=pages.length ? pages.map(([plannerId,offset])=>({id,plannerId,offset,pageSize})):[{id,pageSize}];
    const responses=await Promise.all(queries.map(q=>tripClient.getTripRoutes(q,{signal:controller.signal})));
    if(controller.signal.aborted)return;
    if(responses.some(r=>!r.result))throw new Error('Missing route result');
    setSources(responses.flatMap(r=>r.result!.sources.map(view)).sort((a,b)=>a.id==='graph'?-1:b.id==='graph'?1:a.id.localeCompare(b.id)));
    setError('');
   }catch(err){
    if(controller.signal.aborted)return;
    if(err instanceof ConnectError && err.code===Code.Unauthenticated){navigate('/login');return;}
    if(err instanceof ConnectError && err.code===Code.NotFound){setSources(null);setError('Заявка не найдена или недоступна.');}
    else setError('Не удалось обновить маршруты. Сохранённые данные оставлены на экране.');
   }finally{
    if(!controller.signal.aborted){setLoading(false);}
   }
  }
  void load();return()=>{controller.abort();};
 },[id,revision,offsets,retry,navigate]);
 // Reads every saved page on demand; the on-screen pages stay as they are.
 function exportAll(){
  return collectAllRoutes((sources ?? []).map(s=>s.id),async(plannerId,offset)=>{
   const response=await tripClient.getTripRoutes({id,plannerId,offset,pageSize:exportPageSize});
   if(!response.result)throw new Error('Missing route result');
   const source=response.result.sources.find(s=>s.plannerId===plannerId);
   return {revision:response.result.revision,source:source ? view(source):undefined};
  });
 }
 return <Stack spacing={2}>
  {error && <Alert severity="warning" action={<Button color="inherit" onClick={()=>setRetry(n=>n+1)}>Повторить загрузку маршрутов</Button>}>{sources===null ? 'Не удалось загрузить маршруты. Карточка и история заявки доступны.' : error}</Alert>}
  {loading && sources===null ? <Typography role="status">Загружаем маршруты…</Typography> : sources!==null && <TripRoutes sources={sources} loading={loading} onPage={(source,offset)=>setOffsets(Object.fromEntries(sources.map(s=>[s.id,s.id===source?offset:s.offset])))} onExportAll={exportAll}/>}
 </Stack>;
}
