import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest';
import { getToken, setToken, clearToken, authHeader, TOKEN_KEY } from './token';

describe('token store', () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it('returns null when nothing is stored', () => {
    expect(getToken()).toBeNull();
  });

  it('set → get round-trips the token and persists under the shared key', () => {
    setToken('tok_1');
    expect(getToken()).toBe('tok_1');
    expect(window.localStorage.getItem(TOKEN_KEY)).toBe('tok_1');
  });

  it('clear removes the token', () => {
    setToken('tok_1');
    clearToken();
    expect(getToken()).toBeNull();
  });

  describe('authHeader', () => {
    it('is empty when unauthenticated', () => {
      expect(authHeader()).toEqual({});
    });

    it('carries a bearer header when a token is set', () => {
      setToken('tok_2');
      expect(authHeader()).toEqual({ authorization: 'Bearer tok_2' });
    });
  });

  describe('storage failures are swallowed', () => {
    afterEach(() => {
      vi.restoreAllMocks();
    });

    it('getToken returns null when getItem throws', () => {
      setToken('tok_3');
      vi.spyOn(window.localStorage.__proto__, 'getItem').mockImplementation(() => {
        throw new Error('blocked');
      });
      expect(getToken()).toBeNull();
    });

    it('setToken is a no-op when setItem throws', () => {
      vi.spyOn(window.localStorage.__proto__, 'setItem').mockImplementation(() => {
        throw new Error('quota');
      });
      expect(() => setToken('tok_4')).not.toThrow();
    });

    it('clearToken is a no-op when removeItem throws', () => {
      vi.spyOn(window.localStorage.__proto__, 'removeItem').mockImplementation(() => {
        throw new Error('blocked');
      });
      expect(() => clearToken()).not.toThrow();
    });
  });
});

describe('token store SSR guard', () => {
  const realWindow = globalThis.window;
  afterEach(() => {
    // Restore the jsdom window for the rest of the suite.
    (globalThis as { window?: unknown }).window = realWindow;
  });

  it('degrades to no-op / null when window is undefined', () => {
    (globalThis as { window?: unknown }).window = undefined;
    expect(getToken()).toBeNull();
    expect(authHeader()).toEqual({});
    expect(() => setToken('x')).not.toThrow();
    expect(() => clearToken()).not.toThrow();
  });
});
