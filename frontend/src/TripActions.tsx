import { useRef, useState } from 'react';
import { Alert, Button, Dialog, DialogActions, DialogContent, DialogTitle, Stack, TextField } from '@mui/material';
import { Code, ConnectError } from '@connectrpc/connect';
import { useNavigate } from 'react-router-dom';
import { tripClient } from './api';
import { TripStatus, type TripDetails } from './gen/travelwatch/cabinet/v1/trips_pb';

export function TripActions({trip,onChange}: {trip:TripDetails;onChange:(trip:TripDetails)=>void}) {
  const [comment,setComment] = useState(trip.comment);
  const [busy,setBusy] = useState(false);
  const pending = useRef(false);
  const [confirm,setConfirm] = useState(false);
  const [error,setError] = useState('');
  const [saved,setSaved] = useState(false);
  const [needsRefresh,setNeedsRefresh] = useState(false);
  const navigate = useNavigate();
  const canCancel = trip.status === TripStatus.SAVED || trip.status === TripStatus.RUNNING;
  const tooLong = Array.from(comment).length > 2000;
  async function refreshTrip() {
    try {
      const response = await tripClient.getTrip({id:trip.id});
      if (!response.trip) throw new Error('Missing trip');
      onChange(response.trip);setNeedsRefresh(false);
      // Keep the local comment draft: refreshing server fields must not overwrite it.
    } catch (err) {
      if (err instanceof ConnectError && err.code === Code.Unauthenticated) navigate('/login');
      else {setNeedsRefresh(true);setError('Заявка отменена, но не удалось обновить её данные. Повторите обновление.');}
    }
  }
  async function mutate(action:'cancel'|'comment'|'refresh') {
    if (pending.current) return;
    pending.current = true; setBusy(true);setError('');setSaved(false);
    try {
      if (action === 'refresh') {
        await refreshTrip();
      } else if (action === 'cancel') {
        await tripClient.cancelTrip({id:trip.id});
        onChange({...trip,status:TripStatus.CANCELLED});setConfirm(false);
        await refreshTrip();
      } else {
        await tripClient.updateTripComment({id:trip.id,comment});
        onChange({...trip,comment});setSaved(true);
      }
    } catch (err) {
      if (err instanceof ConnectError && err.code === Code.Unauthenticated) navigate('/login');
      else setError(err instanceof ConnectError ? err.rawMessage : 'Не удалось подтвердить изменение. Повторите попытку.');
    } finally {pending.current=false;setBusy(false);}
  }
  return <Stack spacing={2}>
    {!confirm && error && <Alert severity="error">{error}</Alert>}
    {needsRefresh && <Button disabled={busy} onClick={()=>void mutate('refresh')}>Повторить обновление заявки</Button>}
    <TextField label="Комментарий к заявке" multiline minRows={3} value={comment} disabled={busy}
      onChange={e=>{setComment(e.target.value);setSaved(false);}}
      error={tooLong} helperText={`Личная заметка, видна только вам. ${Array.from(comment).length}/2000`}/>
    <Button variant="outlined" disabled={busy || tooLong || comment===trip.comment} onClick={()=>void mutate('comment')}>Сохранить комментарий</Button>
    {saved && <Alert severity="success">Комментарий сохранён</Alert>}
    {canCancel && <Button color="error" disabled={busy} onClick={()=>{setError('');setConfirm(true);}}>Отменить заявку</Button>}
    <Dialog open={confirm} onClose={()=>{if(!busy)setConfirm(false);}} aria-labelledby="cancel-trip-title">
      <DialogTitle id="cancel-trip-title">Отменить заявку?</DialogTitle>
      <DialogContent><Stack spacing={2}><span>Заявка останется в истории со статусом «Отменена». Возобновить её нельзя; для нового поиска создайте новую заявку.</span>{confirm && error && <Alert severity="error">{error}</Alert>}</Stack></DialogContent>
      <DialogActions><Button disabled={busy} onClick={()=>setConfirm(false)}>Оставить заявку</Button><Button color="error" disabled={busy} onClick={()=>void mutate('cancel')}>Подтвердить отмену</Button></DialogActions>
    </Dialog>
  </Stack>;
}
