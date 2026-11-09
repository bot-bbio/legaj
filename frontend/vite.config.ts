import { svelte } from '@sveltejs/vite-plugin-svelte';
import { svelteTesting } from '@testing-library/svelte/vite';
import { defineConfig } from 'vitest/config';

import { bookmarklet } from './vite-plugin-bookmarklet.ts';

export default defineConfig({
  plugins: [bookmarklet(), svelte(), svelteTesting()],
  server: {
    // The popup's tests read its real files. Only that folder is added: the
    // repo root around frontend/ can hold local files that must stay out.
    fs: { allow: ['.', '../internal/clip/static'] },
  },
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
