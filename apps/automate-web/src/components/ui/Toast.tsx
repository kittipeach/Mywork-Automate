'use client';

import { createContext, useCallback, useContext, useState, type ReactNode } from 'react';
import { CheckCircle2, AlertTriangle, X } from 'lucide-react';
import { cn } from '@/lib/cn';

export type ToastKind = 'success' | 'error';
export type Toast = { id: number; kind: ToastKind; message: string };

type ToastContextValue = {
  toasts: Toast[];
  push: (kind: ToastKind, message: string) => void;
  dismiss: (id: number) => void;
};

const ToastContext = createContext<ToastContextValue | null>(null);

let seq = 0;

/**
 * Tiny toast store. Kept context-based (no timers in the provider) so it's fully
 * unit-testable; callers dismiss explicitly or the viewport's close button does.
 */
export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);

  const dismiss = useCallback((id: number) => {
    setToasts((t) => t.filter((x) => x.id !== id));
  }, []);

  const push = useCallback((kind: ToastKind, message: string) => {
    seq += 1;
    const id = seq;
    setToasts((t) => [...t, { id, kind, message }]);
  }, []);

  return (
    <ToastContext.Provider value={{ toasts, push, dismiss }}>
      {children}
      <ToastViewport toasts={toasts} dismiss={dismiss} />
    </ToastContext.Provider>
  );
}

export function useToast() {
  const ctx = useContext(ToastContext);
  if (!ctx) throw new Error('useToast must be used within a ToastProvider');
  return { toast: ctx.push };
}

function ToastViewport({ toasts, dismiss }: { toasts: Toast[]; dismiss: (id: number) => void }) {
  if (toasts.length === 0) return null;
  return (
    <div className="fixed bottom-4 right-4 z-[60] flex w-80 flex-col gap-2" role="region" aria-label="Notifications">
      {toasts.map((t) => {
        const Icon = t.kind === 'success' ? CheckCircle2 : AlertTriangle;
        return (
          <div
            key={t.id}
            role="status"
            className={cn(
              'flex items-start gap-2 rounded-md border bg-surface px-3 py-2 text-sm shadow-panel',
              t.kind === 'success' ? 'border-success/30 text-ink' : 'border-danger/30 text-ink',
            )}
          >
            <Icon
              className={cn('mt-0.5 h-4 w-4 shrink-0', t.kind === 'success' ? 'text-success' : 'text-danger')}
              aria-hidden
            />
            <span className="flex-1">{t.message}</span>
            <button
              type="button"
              aria-label="Dismiss notification"
              onClick={() => dismiss(t.id)}
              className="text-ink-subtle transition-colors hover:text-ink"
            >
              <X className="h-3.5 w-3.5" />
            </button>
          </div>
        );
      })}
    </div>
  );
}
