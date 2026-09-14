import { useState, type ComponentProps } from 'react';
import { AppBar, Layout, Menu, UserMenu, useAuthState, useLogout, useNotify, useUserMenu } from 'react-admin';
import { Alert, Box, Button, MenuItem, Stack, Typography } from '@mui/material';
import PersonOutlineIcon from '@mui/icons-material/AccountCircleOutlined';
import { Navigate } from 'react-router-dom';
import { Code, ConnectError } from '@connectrpc/connect';

function LogoutItem() {
  const logout = useLogout();
  const notify = useNotify();
  const userMenu = useUserMenu();
  const [busy, setBusy] = useState(false);
  async function handleLogout() {
    setBusy(true);
    try { await logout({}, '/login', false); }
    catch {
      userMenu?.onClose();
      notify('Не удалось подтвердить выход. Сессия могла остаться активной. Повторите выход.', { type: 'error' });
    } finally { setBusy(false); }
  }
  return <MenuItem disabled={busy} onClick={handleLogout}>{busy ? 'Выход…' : 'Выйти'}</MenuItem>;
}

const CabinetAppBar = () => <AppBar userMenu={<UserMenu><LogoutItem /></UserMenu>} />;
const CabinetMenu = () => <Menu><Menu.Item to="/account" primaryText="Личный кабинет" leftIcon={<PersonOutlineIcon />} /></Menu>;

// Guard all routes with a layout, without treating a failed request as a logout.
export function CabinetLayout(props: ComponentProps<typeof Layout>) {
  const { authenticated, error, isPending, isFetching, refetch } = useAuthState(undefined, false);
  if (isPending || isFetching) {
    return <Box sx={{ p: 4 }} role="status">Проверяем сессию…</Box>;
  }
  if (error instanceof ConnectError && error.code === Code.Unauthenticated) {
    return <Navigate to="/login" replace />;
  }
  if (error || !authenticated) {
    return <Stack spacing={3} sx={{ maxWidth: 520, mx: 'auto', p: 4, mt: 8 }}>
      <Typography variant="h4" component="h1">Кабинет временно недоступен</Typography>
      <Alert severity="warning">Не удалось проверить сессию. Проверьте соединение и попробуйте ещё раз.</Alert>
      <Button variant="contained" onClick={() => { void refetch(); }}>Повторить проверку</Button>
    </Stack>;
  }
  return <Layout {...props} menu={CabinetMenu} appBar={CabinetAppBar} />;
}
