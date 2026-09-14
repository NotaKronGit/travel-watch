import { useState, type FormEvent } from 'react';
import { useLogin } from 'react-admin';
import { ConnectError } from '@connectrpc/connect';
import { Alert, Box, Button, Chip, Paper, Stack, TextField, Typography } from '@mui/material';
import RouteIcon from '@mui/icons-material/Route';

export function LoginPage() {
  const login = useLogin();
  const [register, setRegister] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  async function submit(event: FormEvent) {
    event.preventDefault();
    setError(''); setBusy(true);
    try { await login({ email, password, register }, '/'); }
    catch (err) { setError(err instanceof ConnectError ? err.rawMessage : 'Не удалось связаться с сервером. Попробуйте ещё раз'); }
    finally { setBusy(false); }
  }
  return <Box sx={{ minHeight: '100dvh', bgcolor: '#f5f4ef', display: 'grid', gridTemplateColumns: { xs: '1fr', md: '1fr 1fr' } }}>
    <Box sx={{ bgcolor: '#183e38', color: '#fff', p: { xs: 4, md: 8 }, display: 'flex', flexDirection: 'column', justifyContent: 'space-between', gap: 6 }}>
      <Stack direction="row" sx={{ alignItems: "center" }} spacing={1.5}><RouteIcon fontSize="large"/><Typography variant="h6">Travel Watch</Typography></Stack>
      <Box sx={{ maxWidth: 520 }}>
        <Chip label="ПУТЕШЕСТВИЯ С ПЛАНОМ" sx={{ bgcolor: '#31554c', color: '#d4e8b1', mb: 3, letterSpacing: 1 }}/>
        <Typography component="h1" sx={{ fontSize: { xs: 36, md: 54 }, lineHeight: 1.12, fontWeight: 650, letterSpacing: '-1.5px' }}>Ваш следующий маршрут начинается здесь.</Typography>
        <Typography sx={{ mt: 3, color: '#c1d2cb', fontSize: 18, lineHeight: 1.7 }}>Планируйте составные поездки и следите за ценами в одном месте.</Typography>
      </Box>
      <Typography variant="body2" sx={{ color: '#b8cbc3' }}>Первый шаг — ваш личный кабинет.</Typography>
    </Box>
    <Box sx={{ p: { xs: 3, md: 7 }, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
      <Paper elevation={0} sx={{ p: { xs: 3, md: 5 }, width: '100%', maxWidth: 460, border: '1px solid #e4e4da', borderRadius: 4 }}>
        <Typography variant="overline" color="text.secondary">ЛИЧНЫЙ КАБИНЕТ</Typography>
        <Typography component="h2" variant="h4" sx={{ mt: 1, mb: 1, fontWeight: 650 }}>{register ? 'Создать аккаунт' : 'С возвращением'}</Typography>
        <Typography color="text.secondary" sx={{ mb: 4 }}>{register ? 'Укажите email и придумайте пароль.' : 'Войдите, чтобы продолжить.'}</Typography>
        <Box component="form" onSubmit={submit}>
          <Stack spacing={2.5}>
            {error && <Alert severity="error">{error}</Alert>}
            <TextField label="Email" name="email" type="email" autoComplete="email" value={email} onChange={e => setEmail(e.target.value)} required fullWidth disabled={busy}/>
            <TextField label="Пароль" name="password" type="password" autoComplete={register ? 'new-password' : 'current-password'} value={password} onChange={e => setPassword(e.target.value)} required fullWidth disabled={busy} slotProps={{ htmlInput: { minLength: 15, maxLength: 1024 } }} helperText={register ? 'Минимум 15 символов. Можно использовать длинную фразу.' : undefined}/>
            <Button type="submit" variant="contained" size="large" disabled={busy} sx={{ py: 1.5 }}>{busy ? 'Подождите…' : register ? 'Создать аккаунт' : 'Войти'}</Button>
            <Button disabled={busy} onClick={() => { setRegister(!register); setError(''); setPassword(''); }}>{register ? 'Уже есть аккаунт? Войти' : 'Нет аккаунта? Зарегистрироваться'}</Button>
          </Stack>
        </Box>
      </Paper>
    </Box>
  </Box>;
}
