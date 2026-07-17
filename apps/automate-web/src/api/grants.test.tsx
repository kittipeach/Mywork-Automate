import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { ReactNode } from 'react';
import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { useGrants, useAddGrant, useDeleteGrant } from './grants';
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

describe('useGrants', () => {
  it('GETs /flows/{id}/grants and returns the list', async () => {
    fetchMock.mockReturnValueOnce(
      respond(200, { grants: [{ id: 'grn_1', subjectType: 'role', subjectId: 'operator', access: 'viewer' }] }),
    );
    const { result } = renderHook(() => useGrants('flw_1'), { wrapper: wrapperFor(makeClient()) });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(fetchMock.mock.calls[0][0]).toBe('/api/automate/v1/flows/flw_1/grants');
    expect(result.current.data?.grants).toHaveLength(1);
  });

  it('is disabled (does not fetch) without a flow id', () => {
    renderHook(() => useGrants(''), { wrapper: wrapperFor(makeClient()) });
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('sends the bearer token when set', async () => {
    setToken('tok_g');
    fetchMock.mockReturnValueOnce(respond(200, { grants: [] }));
    const { result } = renderHook(() => useGrants('flw_1'), { wrapper: wrapperFor(makeClient()) });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    const headers = fetchMock.mock.calls[0][1].headers as Record<string, string>;
    expect(headers.authorization).toBe('Bearer tok_g');
  });
});

describe('useAddGrant', () => {
  it('POSTs /flows/{id}/grants with the body and invalidates the grant list', async () => {
    fetchMock.mockReturnValueOnce(respond(201, { id: 'grn_new' }));
    const client = makeClient();
    const spy = vi.spyOn(client, 'invalidateQueries');
    const { result } = renderHook(() => useAddGrant('flw_1'), { wrapper: wrapperFor(client) });

    result.current.mutate({ subjectType: 'user', subjectId: 'jane@corp', access: 'editor' });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe('/api/automate/v1/flows/flw_1/grants');
    expect(init.method).toBe('POST');
    expect(JSON.parse(init.body)).toEqual({
      subjectType: 'user',
      subjectId: 'jane@corp',
      access: 'editor',
    });
    expect(result.current.data).toEqual({ id: 'grn_new' });
    expect(spy).toHaveBeenCalledWith({ queryKey: ['grants', 'flw_1'] });
  });

  it('surfaces a non-2xx as an ApiError', async () => {
    fetchMock.mockReturnValueOnce(respond(400, {}));
    const { result } = renderHook(() => useAddGrant('flw_1'), { wrapper: wrapperFor(makeClient()) });
    result.current.mutate({ subjectType: 'role', subjectId: 'x', access: 'viewer' });
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error).toBeInstanceOf(ApiError);
  });
});

describe('useDeleteGrant', () => {
  it('DELETEs /flows/{id}/grants/{grantId} with no body and invalidates the grant list', async () => {
    fetchMock.mockReturnValueOnce(
      Promise.resolve({
        ok: true,
        status: 204,
        json: () => Promise.reject(new Error('no body')),
      } as unknown as Response),
    );
    const client = makeClient();
    const spy = vi.spyOn(client, 'invalidateQueries');
    const { result } = renderHook(() => useDeleteGrant('flw_1'), { wrapper: wrapperFor(client) });

    result.current.mutate('grn_9');
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe('/api/automate/v1/flows/flw_1/grants/grn_9');
    expect(init.method).toBe('DELETE');
    expect(init.body).toBeUndefined();
    expect(result.current.data).toBeUndefined();
    expect(spy).toHaveBeenCalledWith({ queryKey: ['grants', 'flw_1'] });
  });
});
