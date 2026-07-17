import type { ReactNode } from 'react';
import { cn } from '@/lib/cn';

export type RunStatus =
  | 'draft'
  | 'published'
  | 'paused'
  | 'stopped'
  | 'queued'
  | 'running'
  | 'success'
  | 'failed'
  | 'cancelled'
  | 'skipped';

const styles: Record<RunStatus, string> = {
  draft: 'bg-surface-sunken text-ink-muted',
  published: 'bg-success/10 text-success',
  paused: 'bg-warning/10 text-warning',
  stopped: 'bg-ink-subtle/10 text-ink-muted',
  queued: 'bg-info/10 text-info',
  running: 'bg-info/10 text-info',
  success: 'bg-success/10 text-success',
  failed: 'bg-danger/10 text-danger',
  cancelled: 'bg-ink-subtle/10 text-ink-muted',
  skipped: 'bg-surface-sunken text-ink-subtle',
};

export function StatusBadge({ status }: { status: RunStatus }) {
  return (
    <span
      className={cn(
        'inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-medium capitalize',
        styles[status] ?? styles.draft,
      )}
    >
      <span
        className={cn(
          'h-1.5 w-1.5 rounded-full bg-current',
          status === 'running' && 'animate-pulse',
        )}
      />
      {status}
    </span>
  );
}

export function Badge({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <span className={cn('inline-flex items-center rounded-md bg-surface-sunken px-2 py-0.5 text-xs font-medium text-ink-muted', className)}>
      {children}
    </span>
  );
}
