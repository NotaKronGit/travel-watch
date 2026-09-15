import { createClient } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-web';
import { TripService } from './gen/travelwatch/cabinet/v1/trips_pb';
import { AuthService } from './gen/travelwatch/cabinet/v1/auth_pb';

const transport = createConnectTransport({
  baseUrl: window.location.origin,
  defaultTimeoutMs: 8000,
  fetch: (input, init) => fetch(input, { ...init, credentials: 'same-origin' }),
  interceptors: [next => async request => {
    request.header.set('X-Travel-Watch-CSRF', '1');
    return next(request);
  }],
});
export const authClient = createClient(AuthService, transport);

export const tripClient = createClient(TripService, transport);
