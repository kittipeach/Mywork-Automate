import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { ReactNode } from 'react';
import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { useNodes, useFlows, useExecutions, useConnections, useExecution, useVersions } from './hooks';
import { setToken, clearToken } from '@/lib/token';

function wrapper() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
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

/** Read the headers object fetch was called with on a given call. */
function headersOf(call = 0): Record<string, string> {
  return (fetchMock.mock.calls[call][1]?.headers ?? {}) as Record<string, string>;
}

function ok(body: unknown) {
  return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve(body) } as Response);
}

describe('api hooks', () => {
  it('useNodes fetches /nodes', async () => {
    fetchMock.mockReturnValueOnce(ok({ nodes: [] }));
    const { result } = renderHook(() => useNodes(), { wrapper: wrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(fetchMock.mock.calls[0][0]).toBe('/api/automate/v1/nodes');
  });

  it('useFlows builds a querystring from params', async () => {
    fetchMock.mockReturnValueOnce(ok({ flows: [] }));
    const { result } = renderHook(() => useFlows({ q: 'pay', status: 'published' }), { wrapper: wrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    const url = fetchMock.mock.calls[0][0] as string;
    expect(url).toContain('/api/automate/v1/flows?');
    expect(url).toContain('q=pay');
    expect(url).toContain('status=published');
  });

  it('useFlows with no params hits the bare path', async () => {
    fetchMock.mockReturnValueOnce(ok({ flows: [] }));
    const { result } = renderHook(() => useFlows(), { wrapper: wrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(fetchMock.mock.calls[0][0]).toBe('/api/automate/v1/flows');
  });

  it('useExecutions passes flowId', async () => {
    fetchMock.mockReturnValueOnce(ok({ executions: [] }));
    const { result } = renderHook(() => useExecutions({ flowId: 'flw_1' }), { wrapper: wrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(fetchMock.mock.calls[0][0]).toContain('flowId=flw_1');
  });

  it('useConnections fetches /connections', async () => {
    fetchMock.mockReturnValueOnce(ok({ connections: [] }));
    const { result } = renderHook(() => useConnections(), { wrapper: wrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(fetchMock.mock.calls[0][0]).toBe('/api/automate/v1/connections');
  });

  it('surfaces a non-ok response as an error', async () => {
    fetchMock.mockReturnValueOnce(Promise.resolve({ ok: false, status: 500 } as Response));
    const { result } = renderHook(() => useConnections(), { wrapper: wrapper() });
    await waitFor(() => expect(result.current.isError).toBe(true));
  });

  it('useExecution fetches /executions/{id}', async () => {
    fetchMock.mockReturnValueOnce(ok({ id: 'exe_1', steps: [] }));
    const { result } = renderHook(() => useExecution('exe_1'), { wrapper: wrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(fetchMock.mock.calls[0][0]).toBe('/api/automate/v1/executions/exe_1');
  });

  it('useExecution is disabled with no id (no fetch)', async () => {
    const { result } = renderHook(() => useExecution(''), { wrapper: wrapper() });
    // enabled:false → stays in pending/idle, never fetches
    expect(result.current.fetchStatus).toBe('idle');
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('useVersions fetches /flows/{id}/versions', async () => {
    fetchMock.mockReturnValueOnce(ok({ versions: [] }));
    const { result } = renderHook(() => useVersions('flw_1'), { wrapper: wrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(fetchMock.mock.calls[0][0]).toBe('/api/automate/v1/flows/flw_1/versions');
  });

  it('useVersions is disabled with no flowId (no fetch)', () => {
    const { result } = renderHook(() => useVersions(''), { wrapper: wrapper() });
    expect(result.current.fetchStatus).toBe('idle');
    expect(fetchMock).not.toHaveBeenCalled();
  });
});

describe('getJSON auth header', () => {
  it('omits Authorization when no token is set', async () => {
    fetchMock.mockReturnValueOnce(ok({ nodes: [] }));
    const { result } = renderHook(() => useNodes(), { wrapper: wrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(headersOf()).not.toHaveProperty('authorization');
  });

  it('attaches Authorization: Bearer <token> when a token is set', async () => {
    setToken('tok_abc');
    fetchMock.mockReturnValueOnce(ok({ nodes: [] }));
    const { result } = renderHook(() => useNodes(), { wrapper: wrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(headersOf().authorization).toBe('Bearer tok_abc');
  });
});
