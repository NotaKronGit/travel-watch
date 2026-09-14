import type { AuthProvider } from 'react-admin';
import { Code, ConnectError } from '@connectrpc/connect';
import { authClient } from './api';

export const authProvider: AuthProvider = {
  async login({ email, password, register }: { email: string; password: string; register?: boolean }) {
    if (register) await authClient.register({ email, password });
    else await authClient.login({ email, password });
  },
  async logout() { await authClient.logout({}); },
  async checkAuth() { await authClient.getCurrentUser({}); },
  async checkError(error: unknown) {
    if (error instanceof ConnectError && error.code === Code.Unauthenticated) throw error;
  },
  async getIdentity() {
    const { user } = await authClient.getCurrentUser({});
    if (!user) throw new Error('Войдите в аккаунт');
    return { id: user.id, fullName: user.email };
  },
};
