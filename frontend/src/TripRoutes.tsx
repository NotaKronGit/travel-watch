import { useState } from 'react';
import { Accordion, AccordionDetails, AccordionSummary, Alert, Box, Button, Chip, Divider, Stack, Typography } from '@mui/material';
import type { AllRoutes } from './routeExport';
import { copyText, downloadText } from './share';

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
export function TripRoutes({sources,loading,onPage,onExportAll}: {sources: RouteSourceView[];loading:boolean;onPage:(source:string,offset:number)=>void;onExportAll:()=>Promise<AllRoutes>}) {
  const [shareMessage,setShareMessage]=useState('');
  const [shareError,setShareError]=useState(false);
  const [exporting,setExporting]=useState(false);
  const loadFailed='Не удалось загрузить все маршруты. Повторите выгрузку.';
  async function copyRoutes(selected:RouteSourceView[]) {
    try {
      await copyText(routesToJson(selected));
      setShareError(false);setShareMessage('Маршруты скопированы. Можно переслать сообщение.');
    } catch {
      setShareError(true);setShareMessage('Не удалось скопировать. Скачайте файл JSON и перешлите его.');
    }
  }
  async function allRoutesText() {
    const all=await onExportAll();
    return routesToJson(all.sources,all.complete?'all':'partial');
  }
  async function copyAllRoutes() {
    setExporting(true);
    let failedToLoad=false;
    const text=allRoutesText().catch(err=>{failedToLoad=true;throw err;});
    try {
      await copyText(text);
      setShareError(false);setShareMessage('Все маршруты скопированы. Можно переслать сообщение.');
    } catch {
      setShareError(true);setShareMessage(failedToLoad ? loadFailed : 'Не удалось скопировать. Скачайте файл JSON и перешлите его.');
    } finally { setExporting(false); }
  }
  async function downloadAllRoutes() {
    setExporting(true);
    try {
      downloadText('travel-watch-routes.json',await allRoutesText(),'application/json');
      setShareMessage('');
    } catch {
      setShareError(true);setShareMessage(loadFailed);
    } finally { setExporting(false); }
  }
  return <Box component="section" aria-label="Маршруты этой заявки">
    <Divider sx={{mb:3}}/>
    <Typography component="h2" variant="h5" sx={{mb:1}}>Маршруты этой заявки</Typography>
    <Typography color="text.secondary" sx={{mb:3}}>Независимые схемы поездки. Варианты подвоза поездом к одному аэропорту сгруппированы; конкретные вокзалы и поезда выбираются на этапе стыковок. Результат проверки расписаний смотрите на вкладке «Стыковки». Цены ещё не проверены; совпадения между источниками пока не объединены.</Typography>
    <Stack direction="row" sx={{gap:1,flexWrap:'wrap',mb:1}}>
      <Button disabled={loading || exporting || !sources.some(s=>s.routes.length)} onClick={()=>void copyAllRoutes()}>Скопировать все маршруты</Button>
      <Button disabled={loading || exporting || !sources.some(s=>s.routes.length)} onClick={()=>void downloadAllRoutes()}>Скачать все маршруты JSON</Button>
    </Stack>
    <Typography variant="caption" color="text.secondary" sx={{display:'block',mb:2}}>{exporting ? 'Загружаем все варианты…' : 'Выгружаются в JSON все сохранённые варианты каждого источника со всех страниц вместе с предупреждениями.'}</Typography>
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

const exportScopes = {
  selected:'Выгрузка содержит только выбранные варианты.',
  all:'Выгрузка содержит все сохранённые варианты каждого источника.',
  partial:'Выгрузка неполная: достигнут предел выгрузки, перечислены не все сохранённые варианты.',
};
// JSON export: codes for programs next to the same Russian labels and caveats as the screen.
export function routesToJson(sources:RouteSourceView[],scope:keyof typeof exportScopes='selected'):string {
  const exported=sources.filter(source=>source.routes.length>0).map(source=>({
    planner:source.id,
    plannerName:sourceNames[source.id] || source.id,
    stage:source.stage,
    stageName:stateNames[source.stage] || source.stage,
    total:source.total,
    incomplete:source.incomplete,
    finishedAt:source.finishedAt ?? null,
    outcome:source.outcome ? errors[source.outcome] || errors.error : null,
    notice:source.id==='gemini' ? 'Предложения модели. Транспортные связи не подтверждены.' : null,
    warnings:source.warnings.map(warningText),
    routes:source.routes.map((route,index)=>({
      number:source.offset+index+1,
      steps:route.steps.map(step=>({mode:step.mode,modeName:modeNames[step.mode] || step.mode,description:step.description,evidence:step.evidence})),
      warnings:route.warnings.map(warningText),
    })),
  }));
  return JSON.stringify({
    format:'travel-watch.routes.v1',
    complete:scope!=='partial',
    notice:['Расписания, цены, наличие билетов и допустимость стыковок не подтверждены.',exportScopes[scope]],
    sources:exported,
  },null,2);
}
