import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { ReactNode } from 'react';
import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import {
  useRunFlow,
  useCreateFlow,
  usePublishFlow,
  usePauseFlow,
  useResumeFlow,
  useStopFlow,
  useRollback,
  poster,
  ApiError,
} from './mutations';
import { setToken, clearToken } from '@/lib/token';

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

  it('throws an ApiError carrying the status on a non-2xx response', async () => {
    fetchMock.mockReturnValueOnce(respond(409, {}));
    await expect(poster('/thing')).rejects.toThrow('/thing → 409');
    fetchMock.mockReturnValueOnce(respond(409, {}));
    const err = (await poster('/thing').catch((e) => e)) as ApiError;
    expect(err).toBeInstanceOf(ApiError);
    expect(err.status).toBe(409);
    expect(err.path).toBe('/thing');
  });

  it('omits Authorization when no token is set', async () => {
    fetchMock.mockReturnValueOnce(respond(200, {}));
    await poster('/thing');
    expect(fetchMock.mock.calls[0][1].headers).not.toHaveProperty('authorization');
  });

  it('attaches Authorization: Bearer <token> when a token is set', async () => {
    setToken('tok_xyz');
    fetchMock.mockReturnValueOnce(respond(200, {}));
    await poster('/thing');
    const headers = fetchMock.mock.calls[0][1].headers;
    expect(headers.authorization).toBe('Bearer tok_xyz');
    expect(headers['content-type']).toBe('application/json');
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
  it('POSTs /flows/{id}/publish with {changeNote} and invalidates flows + versions', async () => {
    fetchMock.mockReturnValueOnce(respond(200, { id: 'flw_1', name: 'X', folder: 'HR', version: 4 }));
    const client = makeClient();
    const spy = vi.spyOn(client, 'invalidateQueries');
    const { result } = renderHook(() => usePublishFlow(), { wrapper: wrapperFor(client) });

    result.current.mutate({ flowId: 'flw_1', changeNote: 'fix mapping' });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe('/api/automate/v1/flows/flw_1/publish');
    expect(init.method).toBe('POST');
    expect(JSON.parse(init.body)).toEqual({ changeNote: 'fix mapping' });
    expect(result.current.data).toMatchObject({ version: 4 });
    expect(spy).toHaveBeenCalledWith({ queryKey: ['flows'] });
    expect(spy).toHaveBeenCalledWith({ queryKey: ['versions', 'flw_1'] });
  });
});

describe.each([
  ['usePauseFlow', usePauseFlow, 'pause'],
  ['useResumeFlow', useResumeFlow, 'resume'],
  ['useStopFlow', useStopFlow, 'stop'],
] as const)('%s', (_name, useHook, action) => {
  it(`POSTs /flows/{id}/${action} with {} and invalidates flows`, async () => {
    fetchMock.mockReturnValueOnce(respond(200, { status: action === 'pause' ? 'paused' : 'published' }));
    const client = makeClient();
    const spy = vi.spyOn(client, 'invalidateQueries');
    const { result } = renderHook(() => useHook(), { wrapper: wrapperFor(client) });

    result.current.mutate('flw_1');
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe(`/api/automate/v1/flows/flw_1/${action}`);
    expect(init.method).toBe('POST');
    expect(init.body).toBe('{}');
    expect(spy).toHaveBeenCalledWith({ queryKey: ['flows'] });
  });

  it('surfaces a 409 invalid transition as an ApiError with status 409', async () => {
    fetchMock.mockReturnValueOnce(respond(409, {}));
    const { result } = renderHook(() => useHook(), { wrapper: wrapperFor(makeClient()) });
    result.current.mutate('flw_1');
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error).toBeInstanceOf(ApiError);
    expect((result.current.error as ApiError).status).toBe(409);
  });
});

describe('useRollback', () => {
  it('POSTs /flows/{id}/rollback with {toVersion, changeNote} and invalidates flows + versions', async () => {
    fetchMock.mockReturnValueOnce(respond(200, { id: 'flw_1', name: 'X', folder: 'HR', version: 5 }));
    const client = makeClient();
    const spy = vi.spyOn(client, 'invalidateQueries');
    const { result } = renderHook(() => useRollback(), { wrapper: wrapperFor(client) });

    result.current.mutate({ flowId: 'flw_1', toVersion: 2, changeNote: 'revert bad change' });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe('/api/automate/v1/flows/flw_1/rollback');
    expect(init.method).toBe('POST');
    expect(JSON.parse(init.body)).toEqual({ toVersion: 2, changeNote: 'revert bad change' });
    expect(spy).toHaveBeenCalledWith({ queryKey: ['flows'] });
    expect(spy).toHaveBeenCalledWith({ queryKey: ['versions', 'flw_1'] });
  });
});
