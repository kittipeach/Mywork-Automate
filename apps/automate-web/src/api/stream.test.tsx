import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook, act, waitFor } from '@testing-library/react';
import {
  parseExecutionEvent,
  isTerminal,
  TERMINAL_STATUSES,
  useExecutionStream,
} from './stream';
import type { Execution } from '@/lib/mock/store';

const sample: Execution = {
  id: 'exe_1',
  flowId: 'flw_1',
  flowName: 'X',
  status: 'running',
  trigger: 'manual',
  startedAt: '2026-07-14T00:00:00.000Z',
  durationMs: 0,
  version: 1,
  steps: [],
};

describe('isTerminal', () => {
  it('is true for every terminal status', () => {
    for (const s of TERMINAL_STATUSES) expect(isTerminal(s)).toBe(true);
  });

  it('is false for in-flight and unknown statuses', () => {
    expect(isTerminal('running')).toBe(false);
    expect(isTerminal('queued')).toBe(false);
    expect(isTerminal(undefined)).toBe(false);
    expect(isTerminal('bogus')).toBe(false);
  });
});

describe('parseExecutionEvent', () => {
  it('parses a well-formed execution payload', () => {
    const out = parseExecutionEvent(JSON.stringify(sample));
    expect(out).toEqual(sample);
  });

  it('returns null for invalid JSON', () => {
    expect(parseExecutionEvent('not json')).toBeNull();
    expect(parseExecutionEvent('')).toBeNull();
  });

  it('returns null for JSON that is not an execution object', () => {
    expect(parseExecutionEvent('null')).toBeNull();
    expect(parseExecutionEvent('42')).toBeNull();
    expect(parseExecutionEvent('"str"')).toBeNull();
    expect(parseExecutionEvent('[1,2]')).toBeNull();
  });

  it('returns null when id or status is missing / wrong type', () => {
    expect(parseExecutionEvent(JSON.stringify({ status: 'running' }))).toBeNull();
    expect(parseExecutionEvent(JSON.stringify({ id: 'x' }))).toBeNull();
    expect(parseExecutionEvent(JSON.stringify({ id: 1, status: 'running' }))).toBeNull();
    expect(parseExecutionEvent(JSON.stringify({ id: 'x', status: 5 }))).toBeNull();
  });
});

// ---- useExecutionStream with a fake EventSource ---------------------------

type Handler = ((ev: MessageEvent) => void) | (() => void) | null;

class FakeEventSource {
  static instances: FakeEventSource[] = [];
  url: string;
  onopen: Handler = null;
  onmessage: ((ev: MessageEvent) => void) | null = null;
  onerror: Handler = null;
  closed = false;

  constructor(url: string) {
    this.url = url;
    FakeEventSource.instances.push(this);
  }

  close() {
    this.closed = true;
  }

  emitOpen() {
    (this.onopen as () => void)?.();
  }
  emitMessage(data: string) {
    this.onmessage?.({ data } as MessageEvent);
  }
  emitError() {
    (this.onerror as () => void)?.();
  }
}

const OriginalEventSource = globalThis.EventSource;

beforeEach(() => {
  FakeEventSource.instances = [];
  (globalThis as unknown as { EventSource: unknown }).EventSource =
    FakeEventSource as unknown as typeof EventSource;
});

afterEach(() => {
  (globalThis as unknown as { EventSource: unknown }).EventSource = OriginalEventSource;
  vi.restoreAllMocks();
});

describe('useExecutionStream', () => {
  it('does not open a connection when disabled', () => {
    const { result } = renderHook(() => useExecutionStream('exe_1', false));
    expect(FakeEventSource.instances).toHaveLength(0);
    expect(result.current.connected).toBe(false);
    expect(result.current.execution).toBeNull();
  });

  it('does not open a connection without an id', () => {
    renderHook(() => useExecutionStream('', true));
    expect(FakeEventSource.instances).toHaveLength(0);
  });

  it('opens against the stream URL and reports connected on open', async () => {
    const { result } = renderHook(() => useExecutionStream('exe_1', true));
    const es = FakeEventSource.instances[0];
    expect(es.url).toBe('/api/automate/v1/executions/exe_1/stream');

    act(() => es.emitOpen());
    await waitFor(() => expect(result.current.connected).toBe(true));
  });

  it('updates execution state on each message', async () => {
    const { result } = renderHook(() => useExecutionStream('exe_1', true));
    const es = FakeEventSource.instances[0];
    act(() => es.emitOpen());
    act(() => es.emitMessage(JSON.stringify({ ...sample, durationMs: 1200 })));
    await waitFor(() => expect(result.current.execution?.durationMs).toBe(1200));
  });

  it('ignores malformed messages without crashing', async () => {
    const { result } = renderHook(() => useExecutionStream('exe_1', true));
    const es = FakeEventSource.instances[0];
    act(() => es.emitMessage('garbage'));
    expect(result.current.execution).toBeNull();
  });

  it('closes the connection and stops when a terminal snapshot arrives', async () => {
    const { result } = renderHook(() => useExecutionStream('exe_1', true));
    const es = FakeEventSource.instances[0];
    act(() => es.emitOpen());
    act(() => es.emitMessage(JSON.stringify({ ...sample, status: 'success' })));

    await waitFor(() => expect(result.current.execution?.status).toBe('success'));
    expect(es.closed).toBe(true);
    expect(result.current.connected).toBe(false);
  });

  it('reports disconnected on error', async () => {
    const { result } = renderHook(() => useExecutionStream('exe_1', true));
    const es = FakeEventSource.instances[0];
    act(() => es.emitOpen());
    await waitFor(() => expect(result.current.connected).toBe(true));
    act(() => es.emitError());
    await waitFor(() => expect(result.current.connected).toBe(false));
  });

  it('closes the connection on unmount', () => {
    const { unmount } = renderHook(() => useExecutionStream('exe_1', true));
    const es = FakeEventSource.instances[0];
    unmount();
    expect(es.closed).toBe(true);
  });

  it('no-ops when EventSource is unavailable (SSR)', () => {
    (globalThis as unknown as { EventSource: unknown }).EventSource = undefined;
    const { result } = renderHook(() => useExecutionStream('exe_1', true));
    expect(FakeEventSource.instances).toHaveLength(0);
    expect(result.current.connected).toBe(false);
  });
});
