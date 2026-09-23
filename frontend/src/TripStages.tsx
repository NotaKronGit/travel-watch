import { useState, type ReactNode } from 'react';
import { Box, ButtonBase, Stack, Typography } from '@mui/material';

export function TripStages({stage,cancelled,creation,routes,schedules,footer}:{stage:string;cancelled:boolean;creation:ReactNode;routes:ReactNode;schedules:ReactNode;footer:ReactNode}) {
 const initial=stage && stage!=='not_started'?2:1;
 const [selected,setSelected]=useState(initial);
 const [visited,setVisited]=useState<number[]>([initial]);
 const ready=stage==='awaiting_schedules';
 const current=cancelled?0:ready?3:2;
 const stages=[{id:1,name:'Заявка',completed:true},{id:2,name:'Схемы',completed:ready},{id:3,name:'Стыковки',completed:false}];
 const panels=[creation,routes,schedules];
 return <Stack spacing={3} sx={{minWidth:0}}>
  {stages.map(step=><Box key={step.id} component="section" aria-label={`Панель: ${step.name}`} hidden={selected!==step.id}>
    {visited.includes(step.id) && panels[step.id-1]}
  </Box>)}
  {footer}
  <Box component="nav" aria-label="Этапы заявки" sx={{position:'sticky',bottom:12,zIndex:2,bgcolor:'background.paper',borderRadius:2,p:0.5,boxShadow:'0 4px 18px #183e3820',display:'flex',gap:0.25,minWidth:0}}>
   {stages.map((step,index)=><ButtonBase key={step.id} aria-label={`Этап ${step.id}: ${step.name}`} aria-pressed={selected===step.id} onClick={()=>{setSelected(step.id);setVisited(old=>old.includes(step.id)?old:[...old,step.id]);}} sx={{flex:1,minWidth:0,minHeight:68,py:1,pl:index?2:1,pr:2,clipPath:index===0?'polygon(0 0,calc(100% - 12px) 0,100% 50%,calc(100% - 12px) 100%,0 100%)':'polygon(0 0,calc(100% - 12px) 0,100% 50%,calc(100% - 12px) 100%,0 100%,12px 50%)',bgcolor:current===step.id?'#183e38':step.completed?'#e9ece5':'#f3f4ef',color:current===step.id?'white':step.completed?'#657468':'#526059',boxShadow:selected===step.id?'inset 0 -4px #95ac65':'none','&:focus-visible':{boxShadow:'inset 0 0 0 3px #95ac65'},'&:hover':{filter:'brightness(0.96)'}}}>
    <Stack sx={{alignItems:'center'}}><Typography variant="body2" sx={{fontWeight:650}}>{step.id}. {step.name}</Typography><Typography variant="caption">{step.completed?'Пройден':current===step.id?'Текущий':'Впереди'}</Typography></Stack>
   </ButtonBase>)}
  </Box>
 </Stack>;
}
