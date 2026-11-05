import { svelte } from '@sveltejs/vite-plugin-svelte';
import { svelteTesting } from '@testing-library/svelte/vite';
import { defineConfig } from 'vitest/config';

import { bookmarklet } from './vite-plugin-bookmarklet.ts';

export default defineConfig({
  plugins: [bookmarklet(), svelte(), svelteTesting()],
  build: {
    // dist/ is embedded into the Go binary; public/.gitkeep is copied back in
    // on every build so the committed placeholder survives emptyOutDir.
    outDir: 'dist',
    emptyOutDir: true,
  },
  test: {
    environment: 'jsdom',
    include: ['src/**/*.test.ts'],
    setupFiles: ['./vitest-setup.ts'],
  },
});
