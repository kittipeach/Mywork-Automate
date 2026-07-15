// Tiny bearer-token store backed by localStorage. SSR-safe: every access is
// guarded so it degrades to a no-op on the server (no `window`). The token is
// the session credential returned by POST /auth/local/login (or, later, the
// Entra SSO callback) and is attached as `Authorization: Bearer <token>` by the
// shared fetch helpers in src/api.
export const TOKEN_KEY = 'automate.token';

/** True only in a browser with a working localStorage (guards SSR + private mode). */
function hasStorage(): boolean {
  return typeof window !== 'undefined' && typeof window.localStorage !== 'undefined';
}

/** The current bearer token, or null when unauthenticated / on the server. */
export function getToken(): string | null {
  if (!hasStorage()) return null;
  try {
    return window.localStorage.getItem(TOKEN_KEY);
  } catch {
    return null;
  }
}

/** Persist the bearer token. No-op on the server. */
export function setToken(token: string): void {
  if (!hasStorage()) return;
  try {
    window.localStorage.setItem(TOKEN_KEY, token);
  } catch {
    /* storage full / disabled — treat as unauthenticated */
  }
}

/** Drop the bearer token (logout). No-op on the server. */
export function clearToken(): void {
  if (!hasStorage()) return;
  try {
    window.localStorage.removeItem(TOKEN_KEY);
  } catch {
    /* ignore */
  }
}

/**
 * Build the Authorization header when a token exists, else an empty object so
 * it can be spread into any fetch `headers` unconditionally.
 */
export function authHeader(): Record<string, string> {
  const t = getToken();
  return t ? { authorization: `Bearer ${t}` } : {};
}
