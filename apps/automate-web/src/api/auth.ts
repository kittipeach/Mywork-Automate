'use client';

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { getToken, setToken, clearToken } from '@/lib/token';
import { getJSON } from './hooks';
import { poster } from './mutations';

export type LoginInput = { email: string; password: string };
export type LoginResult = { token: string; userId: string; roles: string[] };

/** Current identity from GET /me. */
export type Me = { role: string; permissions: string[] };

/**
 * POST /auth/local/login {email,password} → {token, userId, roles}. On success
 * the bearer token is persisted and *all* queries are invalidated so anything
 * cached anonymously (e.g. /me) refetches with the new credential. Errors surface
 * as ApiError — 401 (invalid credentials) and 423 (account locked) are handled by
 * the caller via `error.status`.
 */
export function useLogin() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: LoginInput) => poster<LoginResult>('/auth/local/login', input),
    onSuccess: (data) => {
      setToken(data.token);
      qc.invalidateQueries();
    },
  });
}

/**
 * GET /me → {role, permissions}. Always enabled so the shell can reflect the
 * signed-in identity, but 401/403 (unauthenticated / no access) are tolerated:
 * we don't retry them and consumers simply treat `data` as absent.
 */
export function useMe() {
  return useQuery({
    queryKey: ['me'],
    queryFn: () => getJSON<Me>('/me'),
    retry: false,
  });
}

/** A directory user with their assigned roles (admin users list; E2-S3). */
export type DirectoryUser = {
  id: string;
  email: string;
  displayName: string;
  roles: string[];
};

/**
 * GET /users → {users:[...]} (admin only; E2-S3). 403 for non-admins is
 * tolerated (no retry) so callers render a friendly "admin only" message from
 * `isError` rather than crashing.
 */
export function useUsers() {
  return useQuery({
    queryKey: ['users'],
    queryFn: () => getJSON<{ users: DirectoryUser[] }>('/users'),
    retry: false,
  });
}

/** Whether a bearer token is currently held (browser-only; false during SSR). */
export function isAuthenticated(): boolean {
  return getToken() !== null;
}

/**
 * Clear the session token and reset cached queries. Pass the QueryClient from a
 * component (via useQueryClient) so cached /me and lists drop immediately.
 */
export function logout(qc?: { clear: () => void }): void {
  clearToken();
  qc?.clear();
}
