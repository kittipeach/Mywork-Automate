import { describe, it, expect } from 'vitest';
import { buildSelect, quoteIdent, WHERE_OPS } from './buildSelect';

describe('quoteIdent', () => {
  it('wraps identifiers in double quotes', () => {
    expect(quoteIdent('users')).toBe('"users"');
  });

  it('doubles embedded double quotes to neutralise injection', () => {
    expect(quoteIdent('we"ird')).toBe('"we""ird"');
  });

  it('trims surrounding whitespace', () => {
    expect(quoteIdent('  users  ')).toBe('"users"');
  });

  it('throws on an empty identifier', () => {
    expect(() => quoteIdent('')).toThrow();
    expect(() => quoteIdent('   ')).toThrow();
  });
});

describe('buildSelect', () => {
  it('selects all columns when none are given', () => {
    expect(buildSelect({ table: 'users' })).toBe('SELECT * FROM "users"');
    expect(buildSelect({ table: 'users', columns: [] })).toBe('SELECT * FROM "users"');
  });

  it('quotes and comma-joins the chosen columns', () => {
    expect(buildSelect({ table: 'users', columns: ['id', 'name'] })).toBe(
      'SELECT "id", "name" FROM "users"',
    );
  });

  it('appends a quoted-identifier WHERE with a quoted string literal', () => {
    expect(
      buildSelect({ table: 'users', columns: ['id'], where: { column: 'name', op: '=', value: 'ann' } }),
    ).toBe('SELECT "id" FROM "users" WHERE "name" = \'ann\'');
  });

  it('leaves numeric WHERE values unquoted so ranges work', () => {
    expect(
      buildSelect({ table: 't', where: { column: 'age', op: '>=', value: '18' } }),
    ).toBe('SELECT * FROM "t" WHERE "age" >= 18');
    expect(
      buildSelect({ table: 't', where: { column: 'score', op: '<', value: '-2.5' } }),
    ).toBe('SELECT * FROM "t" WHERE "score" < -2.5');
  });

  it('always quotes the RHS for LIKE even when numeric-looking', () => {
    expect(
      buildSelect({ table: 't', where: { column: 'code', op: 'LIKE', value: '100%' } }),
    ).toBe('SELECT * FROM "t" WHERE "code" LIKE \'100%\'');
  });

  it('escapes single quotes in string literals', () => {
    expect(
      buildSelect({ table: 't', where: { column: 'name', op: '=', value: "O'Brien" } }),
    ).toBe('SELECT * FROM "t" WHERE "name" = \'O\'\'Brien\'');
  });

  it('omits WHERE when the column is blank', () => {
    expect(
      buildSelect({ table: 't', where: { column: '', op: '=', value: 'x' } }),
    ).toBe('SELECT * FROM "t"');
  });

  it('appends a positive integer LIMIT and floors it', () => {
    expect(buildSelect({ table: 't', limit: 100 })).toBe('SELECT * FROM "t" LIMIT 100');
    expect(buildSelect({ table: 't', limit: 10.9 })).toBe('SELECT * FROM "t" LIMIT 10');
  });

  it('ignores a non-positive or non-finite LIMIT', () => {
    expect(buildSelect({ table: 't', limit: 0 })).toBe('SELECT * FROM "t"');
    expect(buildSelect({ table: 't', limit: -5 })).toBe('SELECT * FROM "t"');
    expect(buildSelect({ table: 't', limit: Number.NaN })).toBe('SELECT * FROM "t"');
  });

  it('combines columns, where and limit', () => {
    expect(
      buildSelect({
        table: 'orders',
        columns: ['id', 'total'],
        where: { column: 'status', op: '=', value: 'paid' },
        limit: 50,
      }),
    ).toBe('SELECT "id", "total" FROM "orders" WHERE "status" = \'paid\' LIMIT 50');
  });

  it('throws on an unsupported operator', () => {
    expect(() =>
      // @ts-expect-error deliberately invalid op
      buildSelect({ table: 't', where: { column: 'a', op: 'DROP', value: '1' } }),
    ).toThrow(/unsupported operator/);
  });

  it('exposes the supported operator set', () => {
    expect(WHERE_OPS).toContain('=');
    expect(WHERE_OPS).toContain('LIKE');
  });
});
