import { useState } from 'react';
import { Accordion, AccordionDetails, AccordionSummary, Alert, Box, Button, Chip, Divider, Stack, Typography } from '@mui/material';

export type RouteStepView = {
  description: string;
  mode: string;
  evidence: string;
};
export type RouteSourceView = {
  id: string;
  stage: string;
  outcome: string;
  durationMs: number;
  finishedAt?: string;
  incomplete: boolean;
  total: number;
  offset: number;
  hasMore: boolean;
  routes: {steps: RouteStepView[]; warnings: string[]}[];
  warnings: string[];
};
const sourceNames: Record<string,string> = {graph:'Наш алгоритм',gemini:'Gemini'};
const stateNames: Record<string,string> = {
  queued:'В очереди',building:'Строит схемы',awaiting_schedules:'Схемы подготовлены',
  no_routes:'Нет вариантов',failed:'Ошибка источника',cancelled:'Отменён',
};
const modeNames: Record<string,string> = {flight:'Самолёт',plane:'Самолёт',train:'Поезд',transfer:'Переезд',bus:'Автобус'};
const errors: Record<string,string> = {
  timeout:'Источник не успел ответить за отведённое время.',
  unavailable:'Источник недоступен. Проверьте его настройки.',
  error:'Не удалось получить результат этого источника.',
};

const warningLabels: Record<string,string> = {
 'Rail access variant limit reached':'Достигнут лимит вариантов подвоза поездом для одной схемы. Сохранены не все вокзалы.',
 'Unique route scheme limit reached':'Достигнут лимит разных схем поездки. Выборка неполная.',
 'Google Flights samples departure dates; route coverage and timetable compatibility are incomplete':'Google Flights проверен на выборочных датах. Показаны схемы; время стыковок и доступность на другие даты не подтверждены.',
 'Google Flights request budget reached':'Достигнут лимит запросов Google Flights. Некоторые направления или даты не проверены.',
 'Google Flights request failed':'Часть запросов Google Flights завершилась ошибкой. Результаты Яндекса сохранены.',
 'Invalid Google Flights route response':'Часть ответов Google Flights не прошла проверку формата и была пропущена.',

 'Provider coverage and timetable compatibility are not complete; transfers are assumptions':'Проверены не все транспортные связи. Переезды между вокзалами, аэропортами и городами пока предполагаемые; время стыковок ещё не проверено.',
 'Transfers are assumed; availability and connection times are not verified':'Нужно проверить доступность переездов и достаточность времени на пересадки.',
 'No destination airports in catalog/radius':'В выбранном радиусе рядом с местом назначения не найдены аэропорты.',
 'Flight endpoint mapping missing or mismatched':'Часть рейсов пропущена: не удалось однозначно определить аэропорты.',
 'Hub airport has no unambiguous catalog mapping':'Часть аэропортов пересадки пропущена: не удалось сопоставить их со справочником.',
 'Empty page before provider total':'Источник вернул не все заявленные данные. Некоторые варианты могли не попасть в результат.',
 'Ambiguous IATA excluded':'Исключён аэропорт с неоднозначным кодом IATA.',
};
export function warningText(message:string) {
 if (warningLabels[message]) return warningLabels[message];
 if (message === 'Collector thread failed') return 'Не удалось получить остановки части рейсов или поездов. Некоторые варианты могли быть пропущены.';
 if (message === 'Incomplete provider page: thread') return 'Источник вернул неполный список остановок для части рейсов или поездов.';
 if (/^Collector \w+ failed$/.test(message)) return 'Не удалось выполнить часть запросов к транспортному источнику. Результат может быть неполным.';
 if (message.startsWith('Incomplete provider page: ')) return 'Источник вернул неполные данные. Некоторые варианты могли быть пропущены.';
 return message;
}

// Presentation of saved results only. Opening an alternative never runs a planner.
export function TripRoutes({sources,loading,onPage}: {sources: RouteSourceView[];loading:boolean;onPage:(source:string,offset:number)=>void}) {
  const [shareMessage,setShareMessage]=useState('');
  const [shareError,setShareError]=useState(false);
  async function copyRoutes(selected:RouteSourceView[]) {
    try {
      await navigator.clipboard.writeText(formatRouteExport(selected));
      setShareError(false);setShareMessage('Маршруты скопированы. Можно переслать сообщение.');
    } catch {
      setShareError(true);setShareMessage('Не удалось скопировать. Скачайте TXT и перешлите файл.');
    }
  }
  function downloadRoutes() {
    const url=URL.createObjectURL(new Blob([formatRouteExport(sources)],{type:'text/plain;charset=utf-8'}));
    const link=document.createElement('a');link.href=url;link.download='travel-watch-routes.txt';
    document.body.appendChild(link);link.click();link.remove();
    setTimeout(()=>URL.revokeObjectURL(url),1000);
  }
  return <Box component="section" aria-label="Маршруты этой заявки">
    <Divider sx={{mb:3}}/>
    <Typography component="h2" variant="h5" sx={{mb:1}}>Маршруты этой заявки</Typography>
    <Typography color="text.secondary" sx={{mb:3}}>Независимые схемы поездки. Варианты подвоза поездом к одному аэропорту сгруппированы; конкретные вокзалы и поезда выбираются на этапе стыковок. Расписания и цены ещё не проверены; совпадения между источниками пока не объединены.</Typography>
    <Stack direction="row" sx={{gap:1,flexWrap:'wrap',mb:1}}>
      <Button disabled={loading || !sources.some(s=>s.routes.length)} onClick={()=>void copyRoutes(sources)}>Скопировать показанные маршруты</Button>
      <Button disabled={loading || !sources.some(s=>s.routes.length)} onClick={downloadRoutes}>Скачать показанные маршруты TXT</Button>
    </Stack>
    <Typography variant="caption" color="text.secondary" sx={{display:'block',mb:2}}>Выгружаются варианты с текущих страниц каждого источника вместе с предупреждениями.</Typography>
    {shareMessage && <Alert role="status" severity={shareError?'warning':'success'} sx={{mb:2}}>{shareMessage}</Alert>}
    {sources.length === 0 ? <Alert severity="info">Сохранённых результатов пока нет. Они появятся после начала построения маршрутов.</Alert> : <Stack spacing={3} divider={<Divider/>}>
      {sources.map(source=><Box component="section" aria-label={`Маршруты: ${sourceNames[source.id] || source.id}`} key={source.id}>
        <Stack direction="row" sx={{alignItems:'center',gap:1,flexWrap:'wrap',mb:1}}>
          <Typography component="h3" variant="h6">{sourceNames[source.id] || source.id}</Typography>
          <Chip size="small" variant="outlined" label={source.stage==='awaiting_schedules' && source.incomplete ? 'Частичный результат' : stateNames[source.stage] || 'Состояние неизвестно'}/>
          <Typography variant="body2" color="text.secondary">Схем: {source.total}</Typography>
        </Stack>
        {source.finishedAt && <Typography variant="caption" color="text.secondary" sx={{display:'block',mb:2}}>Получено: {new Date(source.finishedAt).toLocaleString('ru-RU')} · Длительность: {Math.floor(source.durationMs/60000)} мин {Math.floor(source.durationMs/1000)%60} с</Typography>}
        {source.id === 'gemini' && <Alert severity="info" sx={{mb:2}}>Предложения модели. Транспортные связи не подтверждены.</Alert>}
        {source.outcome && <Alert severity="warning" sx={{mb:2}}>{errors[source.outcome] || errors.error} Результаты других источников сохраняются.</Alert>}
        {source.incomplete && source.routes.length > 0 && <Typography color="text.secondary" variant="body2" sx={{mb:2}}>Выборка неполная: показаны найденные варианты в пределах покрытия и ограничений источника.</Typography>}
        {source.stage === 'no_routes' && <Alert severity="info">В проверенной выборке варианты не найдены. Это не означает, что поездка невозможна.</Alert>}
        {source.stage === 'building' && <Typography role="status" color="text.secondary">Поиск этого источника продолжается…</Typography>}
        {source.warnings.map((warning,index)=><Alert key={index} severity="warning" sx={{mb:1}}>{warningText(warning)}</Alert>)}
        {source.routes.map((route,index)=><Accordion key={index} disableGutters elevation={0} sx={{border:'1px solid',borderColor:'divider',borderRadius:2,mt:1,'&:before':{display:'none'}}}>
          <AccordionSummary expandIcon={<span aria-hidden="true">⌄</span>}>
            <Stack spacing={0.5} sx={{minWidth:0}}>
              <Typography variant="subtitle1">Вариант {source.offset+index+1} · Участков: {route.steps.length}</Typography>
              <Typography variant="body2" color="text.secondary" sx={{overflowWrap:'anywhere'}}>{route.steps.filter(step=>step.mode!=='transfer').map(step=>`${modeNames[step.mode] || ''} ${step.description}`.trim()).join(' → ')}</Typography>
            </Stack>
          </AccordionSummary>
          <AccordionDetails>
            <Button sx={{mb:2}} onClick={()=>void copyRoutes([{...source,offset:source.offset+index,routes:[route]}])}>Скопировать вариант {source.offset+index+1}</Button>
            <Box component="ol" sx={{pl:3,m:0}}>
              {route.steps.map((step,i)=><Box component="li" key={i} sx={{mb:2,pl:0.5}}>
                {step.mode && <Typography variant="caption" color="text.secondary">{modeNames[step.mode] || step.mode}</Typography>}
                <Typography sx={{overflowWrap:'anywhere'}}>{step.description}</Typography>
                {step.evidence && <Typography variant="caption" color="text.secondary" sx={{overflowWrap:'anywhere'}}>{step.evidence}</Typography>}
              </Box>)}
            </Box>
            {route.warnings.map((warning,i)=><Typography key={i} variant="body2" color="error.main" sx={{mt:1,overflowWrap:'anywhere'}}>Требует проверки: {warningText(warning)}</Typography>)}
          </AccordionDetails>
        </Accordion>)}
        {(source.hasMore || source.offset>0) && <Stack direction="row" spacing={1} sx={{alignItems:'center',mt:2}}>
          <Button disabled={loading || source.offset===0} onClick={()=>onPage(source.id,Math.max(0,source.offset-5))}>Предыдущие варианты</Button>
          <Typography variant="body2">Страница {Math.floor(source.offset/5)+1}</Typography>
          <Button disabled={loading || !source.hasMore} onClick={()=>onPage(source.id,source.offset+5)}>Следующие варианты</Button>
        </Stack>}
      </Box>)}
    </Stack>}
  </Box>;
}

export function formatRouteExport(sources:RouteSourceView[]):string {
  const lines=['Travel Watch — схемы маршрутов',
    'Расписания, цены, наличие билетов и допустимость стыковок не подтверждены.',
    'Экспорт содержит только перечисленные ниже варианты с текущих страниц.'];
  for(const source of sources){
    if(!source.routes.length)continue;
    lines.push('',sourceNames[source.id] || source.id,
      `Состояние: ${stateNames[source.stage] || source.stage}`,
      `Варианты ${source.offset+1}–${source.offset+source.routes.length} из ${source.total}`);
    if(source.finishedAt)lines.push(`Получено: ${new Date(source.finishedAt).toLocaleString('ru-RU')}`);
    if(source.incomplete)lines.push('Выборка неполная.');
    if(source.id==='gemini')lines.push('Предложения модели. Транспортные связи не подтверждены.');
    if(source.outcome)lines.push(errors[source.outcome] || errors.error);
    source.warnings.forEach(w=>lines.push(`Предупреждение: ${warningText(w)}`));
    source.routes.forEach((route,index)=>{
      lines.push('',`Вариант ${source.offset+index+1}`);
      route.steps.forEach((step,i)=>{
        lines.push(`${i+1}. ${modeNames[step.mode] || step.mode}: ${step.description}`);
        if(step.evidence)lines.push(`   Источник / обоснование: ${step.evidence}`);
      });
      route.warnings.forEach(w=>lines.push(`Требует проверки: ${warningText(w)}`));
    });
  }
  return lines.join('\n');
}
