'use client';

// Editor top bar: flow name, Undo/Redo, and the mock Validate / Test run /
// Publish actions that surface a transient result banner. Keyboard: Cmd/Ctrl+Z
// undo, Shift+Cmd/Ctrl+Z redo (wired at the page level via useUndoRedoHotkeys).
import { useState, useCallback } from 'react';
import { Undo2, Redo2, Play, CheckCircle2, UploadCloud, X } from 'lucide-react';
import { Button } from '@/components/ui/Button';
import { cn } from '@/lib/cn';
import { useFlowStore } from './store';
import { validateFlow, testRun, publishFlow, type Banner } from './actions';

const toneClasses: Record<Banner['tone'], string> = {
  success: 'bg-success/10 text-success border-success/30',
  warning: 'bg-warning/10 text-warning border-warning/30',
  danger: 'bg-danger/10 text-danger border-danger/30',
  info: 'bg-info/10 text-info border-info/30',
};

export function EditorToolbar({ flowName }: { flowName: string }) {
  const nodes = useFlowStore((s) => s.nodes);
  const config = useFlowStore((s) => s.configByNode);
  const undo = useFlowStore((s) => s.undo);
  const redo = useFlowStore((s) => s.redo);
  const canUndo = useFlowStore((s) => s.past.length > 0);
  const canRedo = useFlowStore((s) => s.future.length > 0);

  const [banner, setBanner] = useState<Banner | null>(null);

  const run = useCallback((fn: (n: typeof nodes, c: typeof config) => Banner) => {
    setBanner(fn(useFlowStore.getState().nodes, useFlowStore.getState().configByNode));
  }, []);

  return (
    <div className="relative">
      <header className="flex h-14 shrink-0 items-center justify-between border-b border-border bg-surface px-6">
        <div className="flex items-center gap-3">
          <h1 className="text-base font-semibold text-ink">{flowName}</h1>
          <div className="flex items-center gap-1">
            <Button
              variant="ghost"
              size="sm"
              aria-label="Undo"
              disabled={!canUndo}
              onClick={() => undo()}
            >
              <Undo2 className="h-4 w-4" />
            </Button>
            <Button
              variant="ghost"
              size="sm"
              aria-label="Redo"
              disabled={!canRedo}
              onClick={() => redo()}
            >
              <Redo2 className="h-4 w-4" />
            </Button>
          </div>
        </div>

        <div className="flex items-center gap-2">
          <Button variant="secondary" size="sm" onClick={() => run(validateFlow)}>
            <CheckCircle2 className="h-4 w-4" /> Validate
          </Button>
          <Button variant="secondary" size="sm" onClick={() => run(testRun)}>
            <Play className="h-4 w-4" /> Test run
          </Button>
          <Button size="sm" onClick={() => run(publishFlow)}>
            <UploadCloud className="h-4 w-4" /> Publish
          </Button>
        </div>
      </header>

      {banner && (
        <div
          role="status"
          className={cn(
            'absolute left-1/2 top-16 z-10 flex w-[36rem] max-w-[90%] -translate-x-1/2 items-start gap-3 rounded-md border px-4 py-3 shadow-pop',
            toneClasses[banner.tone],
          )}
        >
          <div className="min-w-0 flex-1">
            <div className="text-sm font-semibold">{banner.title}</div>
            {banner.detail && <div className="mt-0.5 truncate text-xs opacity-90">{banner.detail}</div>}
          </div>
          <button
            type="button"
            aria-label="Dismiss"
            onClick={() => setBanner(null)}
            className="shrink-0 opacity-70 hover:opacity-100"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
      )}
    </div>
  );
}
