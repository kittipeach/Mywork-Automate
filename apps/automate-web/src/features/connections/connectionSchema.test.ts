import { describe, it, expect } from 'vitest';
import {
  connectionFormSchema,
  emptyConnectionForm,
  CONNECTION_TYPES,
  ROLES,
} from './connectionSchema';

describe('connectionFormSchema', () => {
  it('accepts a fully-filled form', () => {
    const r = connectionFormSchema.safeParse({
      name: 'HR DB',
      type: 'postgres',
      host: 'hr-db:5432',
      allowedRoles: ['admin'],
    });
    expect(r.success).toBe(true);
  });

  it('trims and rejects a blank name', () => {
    const r = connectionFormSchema.safeParse({
      name: '   ',
      type: 'postgres',
      host: 'h',
      allowedRoles: ['admin'],
    });
    expect(r.success).toBe(false);
    if (!r.success) expect(r.error.issues[0].message).toBe('Name is required');
  });

  it('rejects a blank host', () => {
    const r = connectionFormSchema.safeParse({
      name: 'X',
      type: 'sftp',
      host: '',
      allowedRoles: ['admin'],
    });
    expect(r.success).toBe(false);
  });

  it('rejects an unknown type', () => {
    const r = connectionFormSchema.safeParse({
      name: 'X',
      type: 'mysql',
      host: 'h',
      allowedRoles: ['admin'],
    });
    expect(r.success).toBe(false);
  });

  it('requires at least one allowed role', () => {
    const r = connectionFormSchema.safeParse({
      name: 'X',
      type: 'smtp',
      host: 'h',
      allowedRoles: [],
    });
    expect(r.success).toBe(false);
    if (!r.success) expect(r.error.issues[0].message).toBe('Pick at least one role');
  });

  it('seeds an empty form pre-populated only with a role, and exposes catalogs', () => {
    // The seed is intentionally invalid (blank name/host to be filled in) but
    // carries a sane type + default role so the pickers render selected.
    expect(emptyConnectionForm).toEqual({ name: '', type: 'postgres', host: '', allowedRoles: ['admin'] });
    expect(connectionFormSchema.safeParse(emptyConnectionForm).success).toBe(false);
    expect(CONNECTION_TYPES.map((t) => t.value)).toEqual(['postgres', 'sftp', 'smtp', 'graph']);
    expect(ROLES).toContain('admin');
  });
});
