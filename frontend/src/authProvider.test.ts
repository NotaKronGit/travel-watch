import { beforeEach, describe, expect, it, vi } from 'vitest';
import { Code, ConnectError } from '@connectrpc/connect';
vi.mock('./api', () => ({ authClient: { login: vi.fn(), register: vi.fn(), logout: vi.fn(), getCurrentUser: vi.fn() } }));
import { authClient } from './api';
import { authProvider } from './authProvider';

beforeEach(() => vi.resetAllMocks());
describe('session authentication', () => {
  it('uses registration endpoint only for registration', async () => {
    await authProvider.login({ email: 'user@example.com', password: 'long password phrase', register: true });
    expect(authClient.register).toHaveBeenCalledWith({ email: 'user@example.com', password: 'long password phrase' });
    expect(authClient.login).not.toHaveBeenCalled();
  });
  it('does not treat an unavailable server as successful authentication', async () => {
    const error = new ConnectError('unavailable', Code.Unavailable);
    vi.mocked(authClient.getCurrentUser).mockRejectedValue(error);
    await expect(authProvider.checkAuth({})).rejects.toBe(error);
  });
  it('propagates failed logout so the UI cannot claim the session ended', async () => {
    const error = new ConnectError('unavailable', Code.Unavailable);
    vi.mocked(authClient.logout).mockRejectedValue(error);
    await expect(authProvider.logout({})).rejects.toBe(error);
  });
  it('requests reauthentication for an expired session, not every API error', async () => {
    await expect(authProvider.checkError(new ConnectError('expired', Code.Unauthenticated))).rejects.toBeInstanceOf(ConnectError);
    await expect(authProvider.checkError(new ConnectError('unavailable', Code.Unavailable))).resolves.toBeUndefined();
  });
});
