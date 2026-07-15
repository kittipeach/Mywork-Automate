'use client';

// Live run status over Server-Sent Events (E5-S4). The control-plane exposes
// `GET /executions/{id}/stream` which emits `data: {execution json}` messages and
// closes once the run reaches a terminal state. `useExecutionStream` subscribes
// while enabled, mirrors each message into local React state, and tears the
// connection down on terminal / unmount. The pure parse + terminal helpers are
// split out so the event handling can be unit-tested without an EventSource.
import { useEffect, useState } from 'react';
import type { Execution } from '@/lib/mock/store';
import { BASE } from './hooks';

/** Run statuses that mean the stream is done (server closes on these). */
export const TERMINAL_STATUSES = ['success', 'failed', 'cancelled', 'skipped'] as const;

/** True once an execution has reached a terminal state (stop streaming). */
export function isTerminal(status?: string): boolean {
  return (
    status === 'success' ||
    status === 'failed' ||
    status === 'cancelled' ||
    status === 'skipped'
  );
}

/**
 * Parse one SSE `data:` payload into an Execution. Returns null for anything
 * that isn't a JSON object carrying at least an `id` + `status` (malformed
 * chunk, keep-alive comment already stripped by EventSource, etc.) so the caller
 * can safely ignore it rather than blowing up the stream.
 */
export function parseExecutionEvent(data: string): Execution | null {
  try {
    const obj = JSON.parse(data) as unknown;
    if (
      obj &&
      typeof obj === 'object' &&
      typeof (obj as { id?: unknown }).id === 'string' &&
      typeof (obj as { status?: unknown }).status === 'string'
    ) {
      return obj as Execution;
    }
    return null;
  } catch {
    return null;
  }
}

export type ExecutionStream = {
  /** Latest execution snapshot from the stream, or null before the first event. */
  execution: Execution | null;
  /** True while the EventSource is open (has fired onopen and not yet closed). */
  connected: boolean;
};

/**
 * Subscribe to `GET /executions/{id}/stream` while `enabled` is true. Each
 * `message` updates local state; the connection closes automatically on a
 * terminal status, on unmount, on `enabled` flipping false, or on error. SSR-
 * safe: if `EventSource` is unavailable (server render) it no-ops and reports
 * disconnected.
 */
export function useExecutionStream(id: string, enabled: boolean): ExecutionStream {
  const [execution, setExecution] = useState<Execution | null>(null);
  const [connected, setConnected] = useState(false);

  useEffect(() => {
    // Guard: nothing to do without an id/enable, or when EventSource is missing
    // (SSR / older environments). Reset connected so a prior open doesn't leak.
    if (!id || !enabled || typeof EventSource === 'undefined') {
      setConnected(false);
      return;
    }

    const es = new EventSource(`${BASE}/executions/${id}/stream`);

    const close = () => {
      es.close();
      setConnected(false);
    };

    es.onopen = () => setConnected(true);

    es.onmessage = (ev: MessageEvent) => {
      const next = parseExecutionEvent(ev.data);
      if (!next) return;
      setExecution(next);
      // The server closes on terminal, but close proactively too so we stop
      // reacting the instant we see a terminal snapshot.
      if (isTerminal(next.status)) close();
    };

    es.onerror = () => {
      // Browser auto-reconnects on transient errors, but once the server has
      // closed the stream (terminal run) that manifests as an error with the
      // connection CLOSED — treat any error as disconnected.
      setConnected(false);
    };

    return close;
  }, [id, enabled]);

  return { execution, connected };
}
