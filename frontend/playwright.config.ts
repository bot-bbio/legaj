/**
 * The real-browser layout suite (R70). It serves the production build
 * (`npm run build` first) on loopback and drives Microsoft Edge, the engine
 * WebView2 runs on, with its own temporary profile. Edge ships with Windows
 * and the CI runners, so no browser is downloaded. The Go backend is faked
 * (e2e/fake-backend.ts).
 */
import { defineConfig } from '@playwright/test';

// The config runs in Node; this frontend doesn't install Node's type
// definitions, so the one field used is declared here.
declare const process: { readonly execPath: string };

const port = 4179;

export default defineConfig({
  testDir: './e2e',
  testMatch: '**/*.e2e.ts',
  outputDir: './e2e/out/results',
  fullyParallel: true,
  forbidOnly: true,
  retries: 0,
  reporter: [['list']],
  use: {
    channel: 'msedge',
    baseURL: `http://127.0.0.1:${port}`,
  },
  webServer: {
    // Loopback only, so Windows never asks about the firewall.
    command: `"${process.execPath}" node_modules/vite/bin/vite.js preview --host 127.0.0.1 --port ${port} --strictPort`,
    url: `http://127.0.0.1:${port}`,
    reuseExistingServer: false,
    timeout: 60_000,
  },
});
