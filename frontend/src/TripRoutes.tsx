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
 'Provider coverage and timetable compatibility are not complete; transfers are assumptions':'Проверены не все транспортные связи. Переезды между вокзалами, аэропортами и городами пока предполагаемые; время стыковок ещё не проверено.',
 'Transfers are assumed; availability and connection times are not verified':'Нужно проверить доступность переездов и достаточность времени на пересадки.',
 'No destination airports in catalog/radius':'В выбранном радиусе рядом с местом назначения не найдены аэропорты.',
 'Flight endpoint mapping missing or mismatched':'Часть рейсов пропущена: не удалось однозначно определить аэропорты.',
 'Hub airport has no unambiguous catalog mapping':'Часть аэропортов пересадки пропущена: не удалось сопоставить их со справочником.',
 'Empty page before provider total':'Источник вернул не все заявленные данные. Некоторые варианты могли не попасть в результат.',
 'Ambiguous IATA excluded':'Исключён аэропорт с неоднозначным кодом IATA.',
};
function warningText(message:string) {
 if (warningLabels[message]) return warningLabels[message];
 if (message === 'Collector thread failed') return 'Не удалось получить остановки части рейсов или поездов. Некоторые варианты могли быть пропущены.';
 if (message === 'Incomplete provider page: thread') return 'Источник вернул неполный список остановок для части рейсов или поездов.';
 if (/^Collector \w+ failed$/.test(message)) return 'Не удалось выполнить часть запросов к транспортному источнику. Результат может быть неполным.';
 if (message.startsWith('Incomplete provider page: ')) return 'Источник вернул неполные данные. Некоторые варианты могли быть пропущены.';
 return message;
}

// Presentation of saved results only. Opening an alternative never runs a planner.
export function TripRoutes({sources,loading,onPage}: {sources: RouteSourceView[];loading:boolean;onPage:(source:string,offset:number)=>void}) {
  return <Box component="section" aria-label="Маршруты этой заявки">
    <Divider sx={{mb:3}}/>
    <Typography component="h2" variant="h5" sx={{mb:1}}>Маршруты этой заявки</Typography>
    <Typography color="text.secondary" sx={{mb:3}}>Независимые варианты поездки. Расписания, время стыковок и цены ещё не проверены; совпадающие схемы источников пока не объединены.</Typography>
    {sources.length === 0 ? <Alert severity="info">Сохранённых результатов пока нет. Они появятся после начала построения маршрутов.</Alert> : <Stack spacing={3} divider={<Divider/>}>
      {sources.map(source=><Box component="section" aria-label={`Маршруты: ${sourceNames[source.id] || source.id}`} key={source.id}>
        <Stack direction="row" sx={{alignItems:'center',gap:1,flexWrap:'wrap',mb:1}}>
          <Typography component="h3" variant="h6">{sourceNames[source.id] || source.id}</Typography>
          <Chip size="small" variant="outlined" label={source.stage==='awaiting_schedules' && source.incomplete ? 'Частичный результат' : stateNames[source.stage] || 'Состояние неизвестно'}/>
          <Typography variant="body2" color="text.secondary">Вариантов: {source.total}</Typography>
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
