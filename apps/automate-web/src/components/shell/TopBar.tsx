import type { ReactNode } from 'react';

export function TopBar({ title, actions }: { title: string; actions?: ReactNode }) {
  return (
    <header className="flex h-14 shrink-0 items-center justify-between border-b border-border bg-surface px-6">
      <h1 className="text-base font-semibold text-ink">{title}</h1>
      <div className="flex items-center gap-3">
        {actions}
        <div className="flex items-center gap-2 rounded-full border border-border py-1 pl-1 pr-3">
          <div className="grid h-6 w-6 place-items-center rounded-full bg-brand-100 text-xs font-semibold text-brand-700">
            HR
          </div>
          <span className="text-sm text-ink-muted">HR Ops</span>
        </div>
      </div>
    </header>
  );
}
