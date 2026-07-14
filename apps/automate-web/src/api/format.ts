// Pure, framework-free formatting helpers shared across the runs/files pages.
// Kept in src/api (gated ≥90%) so they're unit-tested rather than duplicated
// inline in each presentational page.
import type { RunStatus } from '@/components/ui/Badge';

/** Human-friendly duration. 0 → em dash (nothing ran / still running). */
export function fmtDuration(ms: number): string {
  if (!ms || ms <= 0) return '—';
  if (ms < 1000) return `${ms}ms`;
  const s = Math.round(ms / 1000);
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  return `${m}m ${s % 60}s`;
}

/** Human-friendly byte size (used by the Files page). */
export function fmtBytes(bytes: number): string {
  if (!bytes || bytes <= 0) return '—';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let n = bytes;
  let i = 0;
  while (n >= 1024 && i < units.length - 1) {
    n /= 1024;
    i += 1;
  }
  const rounded = i === 0 ? n : Math.round(n * 10) / 10;
  return `${rounded} ${units[i]}`;
}

/** Compact date/time for tables. Falls back to em dash on empty/invalid input. */
export function fmtDateTime(iso: string): string {
  if (!iso) return '—';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '—';
  return d.toISOString().slice(0, 16).replace('T', ' ');
}

/** Tailwind bg class for a status dot in the step timeline. */
export function stepDotColor(status: RunStatus): string {
  switch (status) {
    case 'success':
      return 'bg-success';
    case 'failed':
      return 'bg-danger';
    case 'running':
      return 'bg-info animate-pulse';
    case 'cancelled':
    case 'skipped':
      return 'bg-ink-subtle';
    default:
      return 'bg-border';
  }
}
