'use client';

// Cmd/Ctrl+Z = undo, Shift+Cmd/Ctrl+Z (or Ctrl+Y) = redo. Ignores keystrokes
// originating from form controls so typing config isn't intercepted.
import { useEffect } from 'react';
import { useFlowStore } from './store';

export function useUndoRedoHotkeys() {
  const undo = useFlowStore((s) => s.undo);
  const redo = useFlowStore((s) => s.redo);

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      const target = e.target as HTMLElement | null;
      const tag = target?.tagName;
      if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return;
      const mod = e.metaKey || e.ctrlKey;
      if (!mod) return;
      const key = e.key.toLowerCase();
      if (key === 'z') {
        e.preventDefault();
        if (e.shiftKey) redo();
        else undo();
      } else if (key === 'y') {
        e.preventDefault();
        redo();
      }
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [undo, redo]);
}
