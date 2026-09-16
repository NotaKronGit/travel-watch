import { useState } from 'react';
import { Accordion, AccordionDetails, AccordionSummary, Alert, Box, Chip, Divider, FormControl, InputLabel, MenuItem, Select, Stack, Typography } from '@mui/material';
import data from './reports/route-comparison.json';

type Outcome = { status: string; message?: string; duration_ms: number; limit_reached?: boolean; paths: { steps: string[]; warnings?: string[] }[] };
const labels: Record<string,string> = {success:'Расчёт выполнен',hypothesis:'Предложения модели, не проверены',unavailable:'Источник не подключён',error:'Ошибка расчёта',unsupported:'Схема пока не поддерживается',not_configured:'Не запускался'};
function PlannerResult({name,outcome,description}: {name:string;outcome:Outcome;description:string}) {
  return <Box component="section" aria-label={name}>
    <Stack direction="row" sx={{gap:1,alignItems:'center',flexWrap:'wrap',mb:1}}>
      <Typography component="h3" variant="h6">{name}</Typography>
      <Chip size="small" label={labels[outcome.status] || 'Статус неизвестен'} variant="outlined"/>
    </Stack>
    <Typography variant="body2" color="text.secondary" sx={{mb:2}}>{description}</Typography>
    {outcome.status === 'error' || outcome.status === 'unsupported' || outcome.status === 'not_configured' || outcome.status === 'unavailable'
      ? <Alert severity={outcome.status === 'error' ? 'warning' : 'info'}>{outcome.status === 'error' ? 'Не удалось получить корректную схему маршрута. Результат не опубликован.' : outcome.message}</Alert>
      : outcome.paths.length === 0 ? <Typography>В пределах текущей модели варианты не найдены. Это не означает, что поездка невозможна.</Typography>
      : <Stack spacing={2}>{outcome.paths.map((path,index)=><Box key={index} sx={{borderLeft:'3px solid',borderColor:'divider',pl:2}}>
          <Typography variant="subtitle2">Вариант {index+1}</Typography>
          <Box component="ol" sx={{pl:2.5,mb:0}}>{path.steps.map((step,i)=><Typography component="li" variant="body2" key={i} sx={{mb:0.75,overflowWrap:'anywhere'}}>{step}</Typography>)}</Box>
          {path.warnings?.map((warning,i)=><Typography key={i} variant="body2" sx={{mt:1,color:'error.main'}}>Требует проверки: {warning}</Typography>)}
          <Typography variant="caption" color="text.secondary">Стыковки по времени не проверены.</Typography>
        </Box>)}</Stack>}
    {outcome.limit_reached && <Alert severity="info" sx={{mt:1}}>Достигнут предел перебора. Показаны не все возможные схемы.</Alert>}
    {outcome.status === 'success' && <Typography variant="caption" color="text.secondary" sx={{display:'block',mt:1}}>Время работы алгоритма: {outcome.duration_ms} мс</Typography>}
  </Box>;
}

// Reports are public, reviewed research artifacts; they contain no trip or user IDs.
// A report is explicitly selected, never assigned to a trip based on a city name.
export function PlannerComparison() {
  const [selected,setSelected] = useState('');
  const report = data.reports.find(item=>item.id===selected);
  return <Box component="section" aria-label="Сравнение планировщиков" sx={{mt:2}}>
    <Divider sx={{mb:3}}/>
    <Typography component="h2" variant="h5" sx={{mb:1}}>Сравнение планировщиков</Typography>
    <Typography color="text.secondary" sx={{mb:2}}>Экспериментальное сравнение самостоятельных поисков. Это отдельные примеры, а не расчёт этой заявки; даты и пассажиры заявки в них не учитываются. Готовность и подтверждение данных указаны для каждого источника отдельно.</Typography>
    <FormControl fullWidth size="small">
      <InputLabel id="route-report-label">Направление отчёта</InputLabel>
      <Select labelId="route-report-label" label="Направление отчёта" value={selected} onChange={e=>setSelected(e.target.value)}>
        <MenuItem value="">Выберите опубликованный отчёт</MenuItem>
        {data.reports.map(item=><MenuItem key={item.id} value={item.id}>{item.title}</MenuItem>)}
      </Select>
    </FormControl>
    {!report ? <Typography variant="body2" color="text.secondary" sx={{mt:2}}>Автоматическое построение маршрута для этой заявки ещё не подключено.</Typography> : <Stack spacing={3} sx={{mt:3}} divider={<Divider/>}>
      <Stack spacing={1}>
        <Typography component="h3" variant="h6">{report.title}</Typography>
        <Alert severity="info">Сравниваем схему поездки, без цен и расписаний. Подтверждение транспортных связей на дату поездки и проверка стыковок ещё требуются.</Alert>
        <Typography variant="body2">Поиски разделены: Gemini получает только направление и общие условия, без графа и ответов других источников. Независимый источник связей GraphPlanner ещё не подключён.</Typography>
        <Typography variant="body2">Расчёт: {new Date(data.generated_at).toLocaleString('ru-RU')}.</Typography>
      </Stack>
      <PlannerResult name="GraphPlanner" outcome={report.graph} description="Наш алгоритм должен самостоятельно строить маршруты по отдельному источнику транспортных связей."/>
      <Box><PlannerResult name="Gemini" outcome={report.gemini} description={`Модель ${data.model}. Самостоятельные предложения по знаниям модели; веб-поиск не подключён. Существование участков и стыковки ещё не подтверждены.`}/>
        <Accordion disableGutters elevation={0} sx={{mt:2}}><AccordionSummary>Точный системный запрос Gemini</AccordionSummary><AccordionDetails><Typography component="pre" variant="body2" sx={{whiteSpace:'pre-wrap',overflowWrap:'anywhere'}}>{data.gemini_prompt}</Typography></AccordionDetails></Accordion>
      </Box>
    </Stack>}
  </Box>;
}
