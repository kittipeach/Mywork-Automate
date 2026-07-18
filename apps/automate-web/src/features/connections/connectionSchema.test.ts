import { describe, it, expect } from 'vitest';
import {
  connectionFormSchema,
  emptyConnectionForm,
  CONNECTION_TYPES,
  ROLES,
} from './connectionSchema';

describe('connectionFormSchema', () => {
  it('accepts a fully-filled Postgres form', () => {
    const r = connectionFormSchema.safeParse({
      name: 'HR DB',
      type: 'postgres',
      host: 'hr-db',
      port: 5432,
      database: 'hr',
      username: 'reader',
      sslMode: 'disable',
      allowedRoles: ['admin'],
    });
    expect(r.success).toBe(true);
  });

  it('requires database + username for a Postgres connection', () => {
    const r = connectionFormSchema.safeParse({
      name: 'HR DB',
      type: 'postgres',
      host: 'hr-db',
      allowedRoles: ['admin'],
    });
    expect(r.success).toBe(false);
    if (!r.success) {
      const paths = r.error.issues.map((i) => i.path[0]);
      expect(paths).toContain('database');
      expect(paths).toContain('username');
    }
  });

  it('does not require database/username for a non-Postgres connection', () => {
    const r = connectionFormSchema.safeParse({
      name: 'MFT',
      type: 'sftp',
      host: 'mft:22',
      allowedRoles: ['admin'],
    });
    expect(r.success).toBe(true);
  });

  it('coerces the port and rejects an out-of-range one', () => {
    const ok = connectionFormSchema.safeParse({
      name: 'X', type: 'postgres', host: 'h', database: 'd', username: 'u', port: '6000', allowedRoles: ['admin'],
    });
    expect(ok.success).toBe(true);
    if (ok.success) expect(ok.data.port).toBe(6000);
    const bad = connectionFormSchema.safeParse({
      name: 'X', type: 'postgres', host: 'h', database: 'd', username: 'u', port: 70000, allowedRoles: ['admin'],
    });
    expect(bad.success).toBe(false);
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

  it('seeds an empty form with sane defaults, and exposes catalogs', () => {
    // The seed is intentionally invalid (blank name/host/db to be filled in) but
    // carries a sane type + default role + port so the pickers render selected.
    expect(emptyConnectionForm).toEqual({
      name: '', type: 'postgres', host: '', port: 5432, database: '', username: '',
      sslMode: 'disable', password: '', secretRef: '', allowedRoles: ['admin'],
    });
    expect(connectionFormSchema.safeParse(emptyConnectionForm).success).toBe(false);
    expect(CONNECTION_TYPES.map((t) => t.value)).toEqual(['postgres', 'sftp', 'smtp', 'graph']);
    expect(ROLES).toContain('admin');
  });
});
