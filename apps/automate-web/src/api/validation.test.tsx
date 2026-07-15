import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { ReactNode } from 'react';
import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { useValidateFlow, statusToRing } from './validation';
import { ApiError } from './mutations';
import { setToken, clearToken } from '@/lib/token';

function makeClient() {
  return new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
}

function wrapperFor(client: QueryClient) {
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
}

const fetchMock = vi.fn();

beforeEach(() => {
  fetchMock.mockReset();
  clearToken();
  globalThis.fetch = fetchMock as unknown as typeof fetch;
});

function respond(status: number, body: unknown) {
  return Promise.resolve({
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
  } as Response);
}

describe('useValidateFlow', () => {
  it('POSTs /flows/{id}/validate with {} and returns the verdict', async () => {
    const verdict = {
      valid: false,
      issues: [
        { nodeId: 'q1', severity: 'error', message: 'Missing connection' },
        { severity: 'warning', message: 'No delivery node' },
      ],
    };
    fetchMock.mockReturnValueOnce(respond(200, verdict));
    const { result } = renderHook(() => useValidateFlow(), { wrapper: wrapperFor(makeClient()) });

    result.current.mutate('flw_1');
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe('/api/automate/v1/flows/flw_1/validate');
    expect(init.method).toBe('POST');
    expect(init.headers['content-type']).toBe('application/json');
    expect(init.body).toBe('{}');
    expect(result.current.data).toEqual(verdict);
  });

  it('returns valid:true with no issues for a clean flow', async () => {
    fetchMock.mockReturnValueOnce(respond(200, { valid: true, issues: [] }));
    const { result } = renderHook(() => useValidateFlow(), { wrapper: wrapperFor(makeClient()) });
    result.current.mutate('flw_ok');
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual({ valid: true, issues: [] });
  });

  it('attaches the bearer token when present', async () => {
    setToken('tok_val');
    fetchMock.mockReturnValueOnce(respond(200, { valid: true, issues: [] }));
    const { result } = renderHook(() => useValidateFlow(), { wrapper: wrapperFor(makeClient()) });
    result.current.mutate('flw_1');
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(fetchMock.mock.calls[0][1].headers.authorization).toBe('Bearer tok_val');
  });

  it('surfaces a non-2xx as an ApiError carrying the status', async () => {
    fetchMock.mockReturnValueOnce(respond(500, {}));
    const { result } = renderHook(() => useValidateFlow(), { wrapper: wrapperFor(makeClient()) });
    result.current.mutate('flw_1');
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error).toBeInstanceOf(ApiError);
    expect((result.current.error as ApiError).status).toBe(500);
  });
});

describe('statusToRing', () => {
  it('gives an info pulsing ring for in-flight steps', () => {
    expect(statusToRing('running')).toContain('ring-info');
    expect(statusToRing('running')).toContain('animate-pulse');
    expect(statusToRing('queued')).toContain('ring-info');
  });

  it('gives success/danger rings for terminal steps', () => {
    expect(statusToRing('success')).toContain('ring-success');
    expect(statusToRing('failed')).toContain('ring-danger');
  });

  it('gives a muted ring for cancelled/skipped', () => {
    expect(statusToRing('cancelled')).toContain('ring-ink-subtle');
    expect(statusToRing('skipped')).toContain('ring-ink-subtle');
  });

  it('returns null for idle/unknown/undefined so the node keeps its border', () => {
    expect(statusToRing(undefined)).toBeNull();
    expect(statusToRing('draft')).toBeNull();
    expect(statusToRing('whatever')).toBeNull();
  });
});
