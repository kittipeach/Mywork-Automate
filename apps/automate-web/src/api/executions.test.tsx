import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { ReactNode } from 'react';
import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { useCancelExecution, useRetryExecution, canCancel, canRetry } from './executions';
import { ApiError } from './mutations';
import { clearToken } from '@/lib/token';

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

describe('useCancelExecution', () => {
  it('POSTs /executions/{id}/cancel and invalidates executions + that execution', async () => {
    fetchMock.mockReturnValueOnce(respond(200, { status: 'cancelled' }));
    const client = makeClient();
    const spy = vi.spyOn(client, 'invalidateQueries');
    const { result } = renderHook(() => useCancelExecution(), { wrapper: wrapperFor(client) });

    result.current.mutate('exe_1');
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe('/api/automate/v1/executions/exe_1/cancel');
    expect(init.method).toBe('POST');
    expect(init.body).toBe('{}');
    expect(result.current.data).toEqual({ status: 'cancelled' });
    expect(spy).toHaveBeenCalledWith({ queryKey: ['executions'] });
    expect(spy).toHaveBeenCalledWith({ queryKey: ['execution', 'exe_1'] });
  });

  it('surfaces a 409 on a terminal run as an ApiError with status 409', async () => {
    fetchMock.mockReturnValueOnce(respond(409, {}));
    const { result } = renderHook(() => useCancelExecution(), { wrapper: wrapperFor(makeClient()) });
    result.current.mutate('exe_1');
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error).toBeInstanceOf(ApiError);
    expect((result.current.error as ApiError).status).toBe(409);
  });
});

describe('useRetryExecution', () => {
  it('POSTs /executions/{id}/retry, returns the new executionId, and invalidates', async () => {
    fetchMock.mockReturnValueOnce(respond(200, { executionId: 'exe_new' }));
    const client = makeClient();
    const spy = vi.spyOn(client, 'invalidateQueries');
    const { result } = renderHook(() => useRetryExecution(), { wrapper: wrapperFor(client) });

    result.current.mutate('exe_1');
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe('/api/automate/v1/executions/exe_1/retry');
    expect(init.method).toBe('POST');
    expect(result.current.data).toEqual({ executionId: 'exe_new' });
    expect(spy).toHaveBeenCalledWith({ queryKey: ['executions'] });
    expect(spy).toHaveBeenCalledWith({ queryKey: ['execution', 'exe_1'] });
  });
});

describe('canCancel / canRetry', () => {
  it('canCancel is true only for in-flight runs', () => {
    expect(canCancel('running')).toBe(true);
    expect(canCancel('queued')).toBe(true);
    expect(canCancel('success')).toBe(false);
    expect(canCancel('failed')).toBe(false);
    expect(canCancel('cancelled')).toBe(false);
  });

  it('canRetry is true only for terminal runs', () => {
    expect(canRetry('success')).toBe(true);
    expect(canRetry('failed')).toBe(true);
    expect(canRetry('cancelled')).toBe(true);
    expect(canRetry('running')).toBe(false);
    expect(canRetry('queued')).toBe(false);
  });
});
