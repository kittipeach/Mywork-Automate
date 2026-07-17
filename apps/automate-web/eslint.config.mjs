import next from 'eslint-config-next/core-web-vitals';

// ESLint 9 flat config for Next.js 16. eslint-config-next ships a native flat
// config array (Linter.Config[]) as of Next 16, so it is spread in directly.
const config = [
  {
    ignores: ['.next/**', 'node_modules/**', 'coverage/**', 'next-env.d.ts'],
  },
  ...next,
  {
    // Test files define inline mock components; a display name adds no value there.
    files: ['**/*.test.ts', '**/*.test.tsx'],
    rules: { 'react/display-name': 'off' },
  },
  {
    // Next 16 bundles react-hooks v6, whose new "React Compiler readiness" rules
    // flag pre-existing, tested, functionally-correct code as errors. Keep them
    // as warnings (visible, non-blocking) rather than refactoring covered
    // components during a dependency upgrade; revisit when adopting the compiler.
    rules: {
      'react-hooks/set-state-in-effect': 'warn',
      'react-hooks/static-components': 'warn',
      'react-hooks/incompatible-library': 'warn',
    },
  },
];

export default config;
