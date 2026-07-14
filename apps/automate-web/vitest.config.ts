import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';
import { resolve } from 'node:path';

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: { '@': resolve(__dirname, './src') },
  },
  test: {
    globals: true,
    environment: 'jsdom',
    include: ['src/**/*.{test,spec}.{ts,tsx}'],
    coverage: {
      provider: 'v8',
      reporter: ['text', 'json-summary', 'json'],
      reportsDirectory: './coverage',
      include: ['src/**/*.{ts,tsx}'],
      // src/app/** are Next App Router route shells (server components) covered
      // by Playwright e2e, not unit tests — same rationale as excluding Go
      // cmd/ mains from the Go gate. Components/hooks/lib remain gated at >=90%.
      exclude: ['src/**/*.{test,spec}.{ts,tsx}', 'src/app/**'],
    },
  },
});
