import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { ReactNode } from 'react';
import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import {
  useCreateConnection,
  useUpdateConnection,
  useDeleteConnection,
  useTestConnection,
  useQueryPreview,
  useConnectionSchema,
} from './connections';
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

describe('useCreateConnection', () => {
  it('POSTs /connections with the input and invalidates connections', async () => {
    fetchMock.mockReturnValueOnce(respond(201, { id: 'conn_new', name: 'HR' }));
    const client = makeClient();
    const spy = vi.spyOn(client, 'invalidateQueries');
    const { result } = renderHook(() => useCreateConnection(), { wrapper: wrapperFor(client) });

    result.current.mutate({ name: 'HR', type: 'postgres', host: 'db:5432', allowedRoles: ['admin'] });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe('/api/automate/v1/connections');
    expect(init.method).toBe('POST');
    expect(JSON.parse(init.body)).toEqual({
      name: 'HR',
      type: 'postgres',
      host: 'db:5432',
      allowedRoles: ['admin'],
    });
    expect(result.current.data).toMatchObject({ id: 'conn_new' });
    expect(spy).toHaveBeenCalledWith({ queryKey: ['connections'] });
  });

  it('surfaces a failure as an ApiError', async () => {
    fetchMock.mockReturnValueOnce(respond(422, {}));
    const { result } = renderHook(() => useCreateConnection(), { wrapper: wrapperFor(makeClient()) });
    result.current.mutate({ name: 'X', type: 'sftp', host: 'h' });
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error).toBeInstanceOf(ApiError);
  });
});

describe('useUpdateConnection', () => {
  it('PUTs /connections/{id} with body sans id and invalidates connections', async () => {
    fetchMock.mockReturnValueOnce(respond(200, { id: 'conn_1', name: 'HR2' }));
    const client = makeClient();
    const spy = vi.spyOn(client, 'invalidateQueries');
    const { result } = renderHook(() => useUpdateConnection(), { wrapper: wrapperFor(client) });

    result.current.mutate({ id: 'conn_1', name: 'HR2', type: 'postgres', host: 'db:5432' });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe('/api/automate/v1/connections/conn_1');
    expect(init.method).toBe('PUT');
    expect(JSON.parse(init.body)).toEqual({ name: 'HR2', type: 'postgres', host: 'db:5432' });
    expect(spy).toHaveBeenCalledWith({ queryKey: ['connections'] });
  });
});

describe('useDeleteConnection', () => {
  it('DELETEs /connections/{id}, tolerates a 204 no-body, and invalidates connections', async () => {
    fetchMock.mockReturnValueOnce(
      Promise.resolve({ ok: true, status: 204, json: () => Promise.reject(new Error('no body')) } as unknown as Response),
    );
    const client = makeClient();
    const spy = vi.spyOn(client, 'invalidateQueries');
    const { result } = renderHook(() => useDeleteConnection(), { wrapper: wrapperFor(client) });

    result.current.mutate('conn_1');
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe('/api/automate/v1/connections/conn_1');
    expect(init.method).toBe('DELETE');
    expect(init.body).toBeUndefined();
    expect(result.current.data).toBeUndefined();
    expect(spy).toHaveBeenCalledWith({ queryKey: ['connections'] });
  });
});

describe('useTestConnection', () => {
  it('POSTs /connections/{id}/test and returns {status:"ok"}', async () => {
    fetchMock.mockReturnValueOnce(respond(200, { status: 'ok' }));
    const { result } = renderHook(() => useTestConnection(), { wrapper: wrapperFor(makeClient()) });
    result.current.mutate('conn_1');
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe('/api/automate/v1/connections/conn_1/test');
    expect(init.method).toBe('POST');
    expect(result.current.data).toEqual({ status: 'ok' });
  });

  it('surfaces a failing probe as an error', async () => {
    fetchMock.mockReturnValueOnce(respond(502, {}));
    const { result } = renderHook(() => useTestConnection(), { wrapper: wrapperFor(makeClient()) });
    result.current.mutate('conn_1');
    await waitFor(() => expect(result.current.isError).toBe(true));
  });
});

describe('useQueryPreview', () => {
  it('POSTs query-preview with {sql, maxRows} and returns items + meta', async () => {
    const payload = {
      items: [{ id: 1, name: 'a' }],
      meta: { columns: ['id', 'name'], rowCount: 1, truncated: false },
    };
    fetchMock.mockReturnValueOnce(respond(200, payload));
    const { result } = renderHook(() => useQueryPreview(), { wrapper: wrapperFor(makeClient()) });

    result.current.mutate({ connectionId: 'conn_1', sql: 'SELECT 1', maxRows: 50 });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe('/api/automate/v1/connections/conn_1/query-preview');
    expect(init.method).toBe('POST');
    expect(JSON.parse(init.body)).toEqual({ sql: 'SELECT 1', maxRows: 50 });
    expect(result.current.data).toEqual(payload);
  });

  it('surfaces invalid_sql (400) as an ApiError carrying status 400', async () => {
    fetchMock.mockReturnValueOnce(respond(400, { error: 'invalid_sql' }));
    const { result } = renderHook(() => useQueryPreview(), { wrapper: wrapperFor(makeClient()) });
    result.current.mutate({ connectionId: 'conn_1', sql: 'DROP TABLE x' });
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error).toBeInstanceOf(ApiError);
    expect((result.current.error as ApiError).status).toBe(400);
  });
});

describe('useConnectionSchema', () => {
  it('GETs /connections/{id}/schema when an id is given', async () => {
    fetchMock.mockReturnValueOnce(respond(200, { tables: [] }));
    const { result } = renderHook(() => useConnectionSchema('conn_1'), { wrapper: wrapperFor(makeClient()) });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(fetchMock.mock.calls[0][0]).toBe('/api/automate/v1/connections/conn_1/schema');
  });

  it('is disabled with no id (never fetches)', () => {
    const { result } = renderHook(() => useConnectionSchema(''), { wrapper: wrapperFor(makeClient()) });
    expect(result.current.fetchStatus).toBe('idle');
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
