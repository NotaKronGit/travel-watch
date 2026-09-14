import { Title, useGetIdentity } from 'react-admin';
import { Alert, Box, Card, CardContent, Chip, Skeleton, Stack, Typography } from '@mui/material';
import RouteIcon from '@mui/icons-material/Route';

export function Dashboard() {
  const { data, isPending, error } = useGetIdentity();
  return <Box sx={{ maxWidth: 1000, mx: 'auto', py: { xs: 3, md: 6 }, px: 2 }}>
    <Title title="Личный кабинет"/>
    <Chip label="TRAVEL WATCH" variant="outlined" sx={{ mb: 2 }}/>
    <Typography variant="h3" component="h1" sx={{ fontWeight: 650, mb: 2 }}>Добро пожаловать</Typography>
    <Typography color="text.secondary" sx={{ mb: 4 }}>Здесь будут ваши поездки и наблюдения за ценами.</Typography>
    <Stack spacing={3}>
      <Card variant="outlined"><CardContent sx={{ p: 3 }}>
        <Typography variant="h6" sx={{ mb: 1 }}>Ваш аккаунт</Typography>
        {isPending ? <Skeleton width={220}/> : error ? <Alert severity="error">Не удалось загрузить профиль. Обновите страницу.</Alert> : <Typography>{data?.fullName}</Typography>}
      </CardContent></Card>
      <Card variant="outlined" sx={{ bgcolor: '#eef3e8' }}><CardContent sx={{ p: 4 }}>
        <RouteIcon sx={{ fontSize: 40, color: 'primary.main', mb: 2 }}/>
        <Typography variant="h5" sx={{ mb: 1 }}>Маршруты — следующий шаг</Typography>
        <Typography color="text.secondary" sx={{ maxWidth: 580 }}>Кабинет уже доступен. Создание заявок, поиск поездок и уведомления появятся позже.</Typography>
      </CardContent></Card>
    </Stack>
  </Box>;
}
