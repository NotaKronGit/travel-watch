import { defineConfig, devices } from '@playwright/test';
export default defineConfig({
  testDir: './e2e', workers: 1, retries: 0,
  use: { baseURL: process.env.PLAYWRIGHT_BASE_URL || 'http://localhost:5173', trace: 'retain-on-failure' },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
  webServer: process.env.PLAYWRIGHT_BASE_URL ? undefined : { command: 'npm run dev', url: 'http://localhost:5173', reuseExistingServer: !process.env.CI },
});
