import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { ReactNode } from 'react';
import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { useLogin, useMe, useUsers, logout, isAuthenticated } from './auth';
import { ApiError } from './mutations';
import { getToken, setToken, clearToken } from '@/lib/token';

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

describe('useLogin', () => {
  it('POSTs /auth/local/login with {email,password}, stores the token, and invalidates queries', async () => {
    fetchMock.mockReturnValueOnce(respond(200, { token: 'tok_9', userId: 'u1', roles: ['admin'] }));
    const client = makeClient();
    const spy = vi.spyOn(client, 'invalidateQueries');
    const { result } = renderHook(() => useLogin(), { wrapper: wrapperFor(client) });

    result.current.mutate({ email: 'a@b.co', password: 'pw' });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe('/api/automate/v1/auth/local/login');
    expect(init.method).toBe('POST');
    expect(JSON.parse(init.body)).toEqual({ email: 'a@b.co', password: 'pw' });
    expect(getToken()).toBe('tok_9');
    // invalidateQueries() called with no args = invalidate everything.
    expect(spy).toHaveBeenCalledWith();
  });

  it('surfaces 401 as an ApiError and does not store a token', async () => {
    fetchMock.mockReturnValueOnce(respond(401, {}));
    const { result } = renderHook(() => useLogin(), { wrapper: wrapperFor(makeClient()) });
    result.current.mutate({ email: 'a@b.co', password: 'bad' });
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect((result.current.error as ApiError).status).toBe(401);
    expect(getToken()).toBeNull();
  });

  it('surfaces 423 locked as an ApiError', async () => {
    fetchMock.mockReturnValueOnce(respond(423, {}));
    const { result } = renderHook(() => useLogin(), { wrapper: wrapperFor(makeClient()) });
    result.current.mutate({ email: 'a@b.co', password: 'pw' });
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect((result.current.error as ApiError).status).toBe(423);
  });
});

describe('useMe', () => {
  it('GETs /me and returns {role, permissions}', async () => {
    fetchMock.mockReturnValueOnce(respond(200, { role: 'designer', permissions: ['flow.read'] }));
    const { result } = renderHook(() => useMe(), { wrapper: wrapperFor(makeClient()) });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(fetchMock.mock.calls[0][0]).toBe('/api/automate/v1/me');
    expect(result.current.data).toEqual({ role: 'designer', permissions: ['flow.read'] });
  });

  it('tolerates 401 without retrying (errors once)', async () => {
    fetchMock.mockReturnValue(respond(401, {}));
    const { result } = renderHook(() => useMe(), { wrapper: wrapperFor(makeClient()) });
    await waitFor(() => expect(result.current.isError).toBe(true));
    // retry:false → exactly one attempt
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it('attaches the bearer token when signed in', async () => {
    setToken('tok_me');
    fetchMock.mockReturnValueOnce(respond(200, { role: 'admin', permissions: [] }));
    const { result } = renderHook(() => useMe(), { wrapper: wrapperFor(makeClient()) });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(fetchMock.mock.calls[0][1].headers.authorization).toBe('Bearer tok_me');
  });
});

describe('useUsers', () => {
  it('GETs /users and returns the directory list', async () => {
    fetchMock.mockReturnValueOnce(
      respond(200, {
        users: [{ id: 'u1', email: 'a@b.co', displayName: 'A', roles: ['admin'] }],
      }),
    );
    const { result } = renderHook(() => useUsers(), { wrapper: wrapperFor(makeClient()) });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(fetchMock.mock.calls[0][0]).toBe('/api/automate/v1/users');
    expect(result.current.data?.users).toHaveLength(1);
  });

  it('tolerates 403 without retrying (errors once)', async () => {
    fetchMock.mockReturnValue(respond(403, {}));
    const { result } = renderHook(() => useUsers(), { wrapper: wrapperFor(makeClient()) });
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it('attaches the bearer token when signed in', async () => {
    setToken('tok_u');
    fetchMock.mockReturnValueOnce(respond(200, { users: [] }));
    const { result } = renderHook(() => useUsers(), { wrapper: wrapperFor(makeClient()) });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(fetchMock.mock.calls[0][1].headers.authorization).toBe('Bearer tok_u');
  });
});

describe('logout / isAuthenticated', () => {
  it('isAuthenticated reflects token presence', () => {
    expect(isAuthenticated()).toBe(false);
    setToken('tok');
    expect(isAuthenticated()).toBe(true);
  });

  it('logout clears the token and the query cache', () => {
    setToken('tok');
    const clear = vi.fn();
    logout({ clear });
    expect(getToken()).toBeNull();
    expect(clear).toHaveBeenCalledOnce();
  });

  it('logout works without a query client', () => {
    setToken('tok');
    expect(() => logout()).not.toThrow();
    expect(getToken()).toBeNull();
  });
});
