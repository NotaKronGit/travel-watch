import { useEffect, useRef, useState, type FormEvent } from 'react';
import { Title } from 'react-admin';
import { Alert, Autocomplete, Box, Button, Chip, Link, Paper, Stack, TextField, Typography } from '@mui/material';
import RouteIcon from '@mui/icons-material/Route';
import { Code, ConnectError } from '@connectrpc/connect';
import { useNavigate } from 'react-router-dom';
import { tripClient } from './api';
import type { CityOption } from './gen/travelwatch/cabinet/v1/trips_pb';

const cityLabel = (c: CityOption) => `${c.name}, ${c.country}${c.region ? `, ${c.region}` : ''}${c.iataCode ? ` (${c.iataCode})` : ''}`;
function CityField({ label, value, onChange, disabled }: {label: string; value: CityOption | null; onChange: (city: CityOption | null) => void; disabled: boolean}) {
  const [input, setInput] = useState(value ? cityLabel(value) : '');
  const [options, setOptions] = useState<CityOption[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [retry, setRetry] = useState(0);
  const navigate = useNavigate();
  useEffect(() => {
    const controller = new AbortController();
    setOptions([]); setError('');
    if (value || input.trim().length < 2) { setLoading(false); return () => controller.abort(); }
    setLoading(true);
    const timer = setTimeout(() => {
      tripClient.searchCities({query: input.trim()}, {signal: controller.signal}).then(response => {
        if (!controller.signal.aborted) setOptions(response.cities);
      }).catch(err => {
        if (controller.signal.aborted) return;
        if (err instanceof ConnectError && err.code === Code.Unauthenticated) navigate('/login');
        else setError('Не удалось загрузить города.');
      }).finally(() => { if (!controller.signal.aborted) setLoading(false); });
    }, 250);
    return () => { clearTimeout(timer); controller.abort(); };
  }, [input, value, retry, navigate]);
  return <Box><Autocomplete options={options} value={value} inputValue={input} disabled={disabled} loading={loading}
    filterOptions={items => items} getOptionLabel={cityLabel} getOptionKey={c => c.id} isOptionEqualToValue={(a,b) => a.id === b.id}
    onChange={(_, city) => { onChange(city); setInput(city ? cityLabel(city) : ''); }}
    onInputChange={(_, text, reason) => { if (reason === 'input' || reason === 'clear') { setInput(text); if (value) onChange(null); } }}
    loadingText="Ищем города…" noOptionsText={error ? <Stack spacing={1}><span>{error}</span><Button size="small" onMouseDown={e => e.preventDefault()} onClick={() => setRetry(n => n + 1)}>Повторить поиск</Button></Stack> : (input.trim().length < 2 ? 'Введите минимум 2 буквы' : 'Город не найден')}
    renderOption={(props, city) => {
      const {key, ...optionProps} = props;
      return <Box component="li" key={key} {...optionProps} aria-label={cityLabel(city)} sx={{gap: 2}}>
        <Box sx={{flex: 1, minWidth: 0}}><Typography component="span">{city.name}, </Typography><Typography component="span" sx={{color: '#767676'}}>{city.country}{city.region ? `, ${city.region}` : ''}</Typography></Box>
        {city.iataCode && <Typography variant="body2" sx={{color: 'text.secondary', letterSpacing: 1}}>{city.iataCode}</Typography>}
      </Box>;
    }}
    renderInput={params => <TextField {...params} label={label} required error={!!error} helperText={error || 'Выберите город из подсказок'} slotProps={{...params.slotProps, htmlInput: {...params.slotProps.htmlInput, maxLength: 100}}}/>}
  /></Box>;
}

export function CreateTripPage() {
  const [origin, setOrigin] = useState<CityOption | null>(null);
  const [destination, setDestination] = useState<CityOption | null>(null);
  const [from, setFrom] = useState(''); const [to, setTo] = useState('');
  const [adults, setAdults] = useState('1');
  const [busy, setBusy] = useState(false); const [error, setError] = useState(''); const [saved, setSaved] = useState('');
  // Retain the key for retries after a lost response; edits create a new operation.
  const operation = useRef<{payload: string; id: string} | null>(null);
  const navigate = useNavigate();
  async function submit(event: FormEvent) {
    event.preventDefault(); if (busy) return; setError('');
    if (!origin || !destination) { setError('Выберите оба города из подсказок.'); return; }
    if (origin.id === destination.id) { setError('Города отправления и назначения должны отличаться.'); return; }
    if (!from || !to || to < from) { setError('Укажите корректный диапазон дат.'); return; }
    const payload = {originId: origin.id, destinationId: destination.id, departureFrom: from, departureTo: to, adults: Number(adults)};
    const key = JSON.stringify(payload);
    if (operation.current?.payload !== key) operation.current = {payload: key, id: crypto.randomUUID()};
    setBusy(true);
    try { const result = await tripClient.createTrip({...payload, requestId: operation.current.id}); setSaved(result.id); }
    catch (err) {
      if (err instanceof ConnectError && err.code === Code.Unauthenticated) navigate('/login');
      else setError(err instanceof ConnectError ? err.rawMessage : 'Не удалось подтвердить сохранение. Повторите попытку.');
    } finally { setBusy(false); }
  }
  return <Box sx={{maxWidth: 1180, mx: 'auto', py: {xs: 2, md: 5}, px: {xs: 1, md: 3}}}>
    <Title title="Создать заявку"/>
    <Paper elevation={0} sx={{overflow: 'hidden', borderRadius: 4, border: '1px solid #e4e4da', display: 'grid', gridTemplateColumns: {xs: '1fr', md: '0.85fr 1.15fr'}}}>
      <Stack spacing={4} sx={{bgcolor: '#183e38', color: '#fff', p: {xs: 3, md: 5}, justifyContent: 'space-between'}}>
        <Stack direction="row" spacing={1.5} sx={{alignItems: 'center'}}><RouteIcon fontSize="large"/><Typography variant="h6">Travel Watch</Typography></Stack>
        <Box><Chip label="ПУТЕШЕСТВИЯ С ПЛАНОМ" sx={{bgcolor: '#31554c', color: '#d4e8b1', mb: 3}}/>
          <Typography component="h2" sx={{fontSize: {xs: 30, md: 42}, fontWeight: 650, lineHeight: 1.15}}>Куда отправимся?</Typography>
          <Typography sx={{mt: 3, color: '#c1d2cb', lineHeight: 1.7}}>Выберите начало и конец поездки. Город пересадки между поездом и прямым рейсом система подберёт при поиске.</Typography>
        </Box>
        <Typography variant="body2" sx={{color: '#b8cbc3'}}>Одна поездка — несколько возможностей.</Typography>
      </Stack>
      <Box sx={{p: {xs: 3, md: 5}, minWidth: 0}}>
        <Typography variant="overline" color="text.secondary">НОВАЯ ПОЕЗДКА</Typography>
        <Typography component="h1" variant="h4" sx={{fontWeight: 650, mt: 1, mb: 1}}>{saved ? 'Заявка сохранена' : 'Создать заявку'}</Typography>
        {saved ? <Stack spacing={3} sx={{mt: 3}}>
          <Alert severity="success">Параметры поездки сохранены в вашем аккаунте.</Alert>
          <Typography>{origin?.name} → {destination?.name}</Typography>
          <Typography>Выезд в любой день с {from} по {to} · Взрослых: {adults}</Typography>
          <Alert severity="info">Поиск ещё не запущен. Подбор маршрутов и отслеживание цен появятся позже.</Alert>
          <Typography variant="caption" sx={{overflowWrap: 'anywhere'}}>Номер заявки: {saved}</Typography>
          <Button variant="contained" onClick={() => {setSaved(''); operation.current = null;}}>Создать ещё одну</Button>
          <Button onClick={() => navigate('/account')}>В личный кабинет</Button>
        </Stack> : <Box component="form" onSubmit={submit} sx={{mt: 3}}>
          <Stack spacing={2.5}>
            <Typography color="text.secondary">Поездка в одну сторону. Укажите удобные даты отправления.</Typography>
            {error && <Alert severity="error">{error}</Alert>}
            <CityField label="Откуда" value={origin} onChange={setOrigin} disabled={busy}/>
            <CityField label="Куда" value={destination} onChange={setDestination} disabled={busy}/>
            <Box><Typography sx={{fontWeight: 600, mb: 1}}>Когда можете выехать?</Typography>
            <Typography variant="body2" color="text.secondary" sx={{mb: 2}}>Выезд в любой день указанного диапазона, включая обе даты. Это поездка в одну сторону, без обратного билета. Для выезда в конкретный день укажите одинаковые даты.</Typography>
            <Stack direction={{xs: 'column', sm: 'row'}} spacing={2}>
              <TextField label="Самая ранняя дата выезда" type="date" value={from} onChange={e => setFrom(e.target.value)} required fullWidth disabled={busy} slotProps={{inputLabel:{shrink:true}}}/>
              <TextField label="Самая поздняя дата выезда" type="date" value={to} onChange={e => setTo(e.target.value)} required fullWidth disabled={busy} slotProps={{inputLabel:{shrink:true},htmlInput:{min:from}}}/>
            </Stack></Box>
            <TextField label="Взрослые" type="number" value={adults} onChange={e => setAdults(e.target.value)} required disabled={busy} slotProps={{htmlInput:{min:1,max:9,step:1}}} helperText="От 1 до 9 пассажиров"/>
            <Alert severity="info">Сейчас можно сохранить заявку. Поиск билетов и уведомления появятся позже.</Alert>
            <Button type="submit" size="large" variant="contained" disabled={busy} sx={{py:1.5}}>{busy ? 'Сохраняем…' : 'Сохранить заявку'}</Button>
            <Typography variant="caption" color="text.secondary">Справочник: <Link href="https://www.geonames.org/" target="_blank" rel="noreferrer">GeoNames</Link>, <Link href="https://creativecommons.org/licenses/by/4.0/" target="_blank" rel="noreferrer">CC BY 4.0</Link>. Наличие города не гарантирует доступность билетов.</Typography>
          </Stack>
        </Box>}
      </Box>
    </Paper>
  </Box>;
}
