import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { Admin, CustomRoutes, defaultTheme } from 'react-admin';
import polyglotI18nProvider from 'ra-i18n-polyglot';
import russianMessages from 'ra-language-russian';
import { authProvider } from './authProvider';
import { Dashboard } from './Dashboard';
import { LoginPage } from './LoginPage';
import { Route, Navigate } from 'react-router-dom';
import { CabinetLayout } from './CabinetLayout';

const i18nProvider = polyglotI18nProvider(() => russianMessages, 'ru');
const theme = {
  ...defaultTheme,
  palette: { ...defaultTheme.palette, primary: { main: '#183e38' }, secondary: { main: '#61743d' }, background: { default: '#f5f4ef', paper: '#ffffff' } },
  shape: { borderRadius: 10 },
  typography: { fontFamily: 'Inter, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif', button: { textTransform: 'none' as const, fontWeight: 600 } },
};

createRoot(document.getElementById('root')!).render(
  <StrictMode><Admin title="Travel Watch" authProvider={authProvider} i18nProvider={i18nProvider} layout={CabinetLayout} loginPage={LoginPage} theme={theme} darkTheme={null} disableTelemetry><CustomRoutes><Route path="/" element={<Navigate to="/account" replace />} /><Route path="/account" element={<Dashboard />} /></CustomRoutes></Admin></StrictMode>,
);
