import { describe, it, expect } from 'vitest';
import { parseAuthConfig, shouldShowLocalLogin, hasEntraSso } from './authProviders';

describe('parseAuthConfig', () => {
  it('accepts entra-only config', () => {
    const cfg = parseAuthConfig({ providers: ['entra'] });
    expect(cfg.providers).toEqual(['entra']);
  });

  it('accepts entra+local config', () => {
    const cfg = parseAuthConfig({ providers: ['entra', 'local'] });
    expect(cfg.providers).toEqual(['entra', 'local']);
  });

  it.each([
    ['empty providers', { providers: [] }],
    ['unknown provider', { providers: ['github'] }],
    ['missing providers', {}],
    ['wrong type', { providers: 'entra' }],
    ['null', null],
  ])('rejects %s', (_name, bad) => {
    expect(() => parseAuthConfig(bad)).toThrow();
  });
});

describe('shouldShowLocalLogin', () => {
  it('is true only when local is present', () => {
    expect(shouldShowLocalLogin({ providers: ['entra', 'local'] })).toBe(true);
    expect(shouldShowLocalLogin({ providers: ['entra'] })).toBe(false);
  });
});

describe('hasEntraSso', () => {
  it('detects entra', () => {
    expect(hasEntraSso({ providers: ['entra'] })).toBe(true);
    expect(hasEntraSso({ providers: ['local'] })).toBe(false);
  });
});
