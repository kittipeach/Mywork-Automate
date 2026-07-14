import type { Config } from 'tailwindcss';

// MyWork enterprise design tokens (banking-grade). Colors are exposed as CSS
// variables in globals.css so the palette can be themed per-tenant later.
const config: Config = {
  content: ['./src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      colors: {
        brand: {
          DEFAULT: 'hsl(var(--brand))',
          fg: 'hsl(var(--brand-fg))',
          muted: 'hsl(var(--brand-muted))',
          50: 'hsl(var(--brand-50))',
          100: 'hsl(var(--brand-100))',
          600: 'hsl(var(--brand-600))',
          700: 'hsl(var(--brand-700))',
        },
        surface: {
          DEFAULT: 'hsl(var(--surface))',
          sunken: 'hsl(var(--surface-sunken))',
          raised: 'hsl(var(--surface-raised))',
        },
        sidebar: {
          DEFAULT: 'hsl(var(--sidebar))',
          fg: 'hsl(var(--sidebar-fg))',
          muted: 'hsl(var(--sidebar-muted))',
          active: 'hsl(var(--sidebar-active))',
        },
        border: 'hsl(var(--border))',
        ink: {
          DEFAULT: 'hsl(var(--ink))',
          muted: 'hsl(var(--ink-muted))',
          subtle: 'hsl(var(--ink-subtle))',
        },
        success: 'hsl(var(--success))',
        warning: 'hsl(var(--warning))',
        danger: 'hsl(var(--danger))',
        info: 'hsl(var(--info))',
      },
      borderRadius: {
        md: '8px',
        lg: '12px',
        xl: '16px',
      },
      boxShadow: {
        panel: '0 1px 2px rgba(16,24,40,0.06), 0 1px 3px rgba(16,24,40,0.10)',
        pop: '0 8px 24px rgba(16,24,40,0.12)',
      },
      fontFamily: {
        sans: ['var(--font-sans)', 'system-ui', 'sans-serif'],
        mono: ['ui-monospace', 'SFMono-Regular', 'Menlo', 'monospace'],
      },
    },
  },
  plugins: [],
};

export default config;
