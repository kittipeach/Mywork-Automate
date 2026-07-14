import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { ReactNode } from 'react';
import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { useRunFlow, useCreateFlow, usePublishFlow, poster } from './mutations';

function makeClient() {
  return new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
}

function wrapperFor(client: QueryClient) {
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
}

const fetchMock = vi.fn();

beforeEach(() => {
  fetchMock.mockReset();
  globalThis.fetch = fetchMock as unknown as typeof fetch;
});

function respond(status: number, body: unknown) {
  return Promise.resolve({
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
  } as Response);
}

describe('poster', () => {
  it('POSTs JSON and parses the body', async () => {
    fetchMock.mockReturnValueOnce(respond(201, { ok: 1 }));
    const out = await poster<{ ok: number }>('/thing', { a: 1 });
    expect(out).toEqual({ ok: 1 });
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe('/api/automate/v1/thing');
    expect(init.method).toBe('POST');
    expect(init.headers['content-type']).toBe('application/json');
    expect(init.body).toBe(JSON.stringify({ a: 1 }));
  });

  it('defaults the body to {}', async () => {
    fetchMock.mockReturnValueOnce(respond(202, {}));
    await poster('/thing');
    expect(fetchMock.mock.calls[0][1].body).toBe('{}');
  });

  it('throws on a non-2xx response', async () => {
    fetchMock.mockReturnValueOnce(respond(500, {}));
    await expect(poster('/thing')).rejects.toThrow('/thing → 500');
  });
});

describe('useRunFlow', () => {
  it('POSTs /flows/{id}/run with {} and returns executionId', async () => {
    fetchMock.mockReturnValueOnce(respond(202, { executionId: 'exe_99' }));
    const client = makeClient();
    const { result } = renderHook(() => useRunFlow(), { wrapper: wrapperFor(client) });

    result.current.mutate('flw_1');
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data).toEqual({ executionId: 'exe_99' });
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe('/api/automate/v1/flows/flw_1/run');
    expect(init.method).toBe('POST');
    expect(init.body).toBe('{}');
  });

  it('invalidates the executions query on success', async () => {
    fetchMock.mockReturnValueOnce(respond(202, { executionId: 'exe_99' }));
    const client = makeClient();
    const spy = vi.spyOn(client, 'invalidateQueries');
    const { result } = renderHook(() => useRunFlow(), { wrapper: wrapperFor(client) });

    result.current.mutate('flw_1');
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(spy).toHaveBeenCalledWith({ queryKey: ['executions'] });
  });

  it('surfaces a failed run as an error', async () => {
    fetchMock.mockReturnValueOnce(respond(500, {}));
    const { result } = renderHook(() => useRunFlow(), { wrapper: wrapperFor(makeClient()) });
    result.current.mutate('flw_1');
    await waitFor(() => expect(result.current.isError).toBe(true));
  });
});

describe('useCreateFlow', () => {
  it('POSTs /flows with {name, folder} and invalidates flows', async () => {
    fetchMock.mockReturnValueOnce(respond(201, { id: 'flw_new', name: 'X', folder: 'HR' }));
    const client = makeClient();
    const spy = vi.spyOn(client, 'invalidateQueries');
    const { result } = renderHook(() => useCreateFlow(), { wrapper: wrapperFor(client) });

    result.current.mutate({ name: 'X', folder: 'HR' });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe('/api/automate/v1/flows');
    expect(init.method).toBe('POST');
    expect(JSON.parse(init.body)).toEqual({ name: 'X', folder: 'HR' });
    expect(result.current.data).toMatchObject({ id: 'flw_new' });
    expect(spy).toHaveBeenCalledWith({ queryKey: ['flows'] });
  });
});

describe('usePublishFlow', () => {
  it('POSTs /flows/{id}/publish and invalidates flows', async () => {
    fetchMock.mockReturnValueOnce(respond(200, { id: 'flw_1', name: 'X', folder: 'HR', version: 4 }));
    const client = makeClient();
    const spy = vi.spyOn(client, 'invalidateQueries');
    const { result } = renderHook(() => usePublishFlow(), { wrapper: wrapperFor(client) });

    result.current.mutate('flw_1');
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe('/api/automate/v1/flows/flw_1/publish');
    expect(init.method).toBe('POST');
    expect(init.body).toBe('{}');
    expect(result.current.data).toMatchObject({ version: 4 });
    expect(spy).toHaveBeenCalledWith({ queryKey: ['flows'] });
  });
});
