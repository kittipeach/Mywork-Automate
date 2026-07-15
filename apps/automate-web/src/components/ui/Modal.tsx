'use client';

import { useEffect, type ReactNode } from 'react';
import { X } from 'lucide-react';
import { cn } from '@/lib/cn';

export interface ModalProps {
  open: boolean;
  onClose: () => void;
  title: ReactNode;
  children: ReactNode;
  /** Footer actions (e.g. Cancel / Save). */
  footer?: ReactNode;
  className?: string;
}

/**
 * Minimal accessible modal dialog: role="dialog" + aria-modal, Escape to close,
 * click-on-backdrop to close, and a labelled close button. No portal — rendered
 * inline (fixed-positioned) which is sufficient for this app's single-overlay
 * usage and keeps it trivially testable in jsdom.
 */
export function Modal({ open, onClose, title, children, footer, className }: ModalProps) {
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [open, onClose]);

  if (!open) return null;

  return (
    <div
      className="fixed inset-0 z-50 grid place-items-center bg-black/40 p-4"
      onClick={onClose}
      data-testid="modal-backdrop"
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-label={typeof title === 'string' ? title : undefined}
        className={cn(
          'w-full max-w-lg rounded-lg border border-border bg-surface shadow-panel',
          className,
        )}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between border-b border-border px-5 py-4">
          <h2 className="text-sm font-semibold text-ink">{title}</h2>
          <button
            type="button"
            aria-label="Close dialog"
            onClick={onClose}
            className="text-ink-muted transition-colors hover:text-ink"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
        <div className="max-h-[70vh] overflow-auto px-5 py-4">{children}</div>
        {footer && (
          <div className="flex items-center justify-end gap-2 border-t border-border px-5 py-4">
            {footer}
          </div>
        )}
      </div>
    </div>
  );
}
