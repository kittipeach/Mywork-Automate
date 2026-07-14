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
    setupFiles: ['./vitest.setup.ts'],
    include: ['src/**/*.{test,spec}.{ts,tsx}'],
    coverage: {
      provider: 'v8',
      reporter: ['text', 'json-summary', 'json'],
      reportsDirectory: './coverage',
      include: ['src/**/*.{ts,tsx}'],
      // Excluded from the unit gate (visual/e2e-covered, or non-prod fixtures) —
      // same rationale as excluding Go cmd/ mains:
      //   app/**        Next App Router route shells (server components)
      //   components/shell, Providers  layout/nav/framework wrappers
      //   lib/mock/**   mock API fixtures (replaced by the real Go API)
      // Reusable components (ui/**), hooks (api/**), lib logic and features/**
      // remain gated at >=90%.
      exclude: [
        'src/**/*.{test,spec}.{ts,tsx}',
        'src/app/**',
        'src/components/shell/**',
        'src/components/Providers.tsx',
        'src/lib/mock/**',
        // React Flow rendering wrappers + keyboard wiring: not unit-testable in
        // jsdom (RF needs a real layout engine); verified via the running app /
        // Playwright. Their LOGIC lives in the tested store/graph/actions.
        'src/features/flow-editor/Canvas.tsx',
        'src/features/flow-editor/AutomateNode.tsx',
        'src/features/flow-editor/ConfigPanel.tsx',
        'src/features/flow-editor/EditorToolbar.tsx',
        'src/features/flow-editor/useUndoRedoHotkeys.ts',
        'src/**/index.ts',
        'src/**/types.ts',
      ],
    },
  },
});
