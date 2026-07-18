import { describe, it, expect } from 'vitest';
import {
  emptySpec,
  tablesInPlay,
  specComplete,
  previewSQL,
  FILTER_OPS,
  JOIN_TYPES,
  type QuerySpec,
} from './queryBuilderSpec';

describe('queryBuilderSpec', () => {
  it('emptySpec has sane defaults + catalogs', () => {
    const s = emptySpec();
    expect(s.table).toBe('');
    expect(s.columns).toEqual([]);
    expect(s.combinator).toBe('and');
    expect(s.limit).toBe(100);
    expect(FILTER_OPS).toContain('like');
    expect(JOIN_TYPES).toEqual(['inner', 'left']);
  });

  it('tablesInPlay lists base then joined tables, de-duplicated', () => {
    const spec: QuerySpec = {
      table: 'employees',
      joins: [
        { type: 'left', table: 'departments', on: [] },
        { type: 'inner', table: 'departments', on: [] }, // dup ignored
        { type: 'inner', table: '', on: [] }, // empty ignored
      ],
      columns: [],
    };
    expect(tablesInPlay(spec)).toEqual(['employees', 'departments']);
    expect(tablesInPlay(emptySpec())).toEqual([]);
  });

  describe('specComplete', () => {
    it('needs a table + at least one named column', () => {
      expect(specComplete({ table: '', columns: [{ name: 'a' }] })).toBe(false);
      expect(specComplete({ table: 't', columns: [] })).toBe(false);
      expect(specComplete({ table: 't', columns: [{ name: '' }] })).toBe(false);
      expect(specComplete({ table: 't', columns: [{ name: 'a' }] })).toBe(true);
    });

    it('needs every join fully specified', () => {
      const base = { table: 't', columns: [{ name: 'a' }] };
      expect(specComplete({ ...base, joins: [{ type: 'inner', table: '', on: [] }] })).toBe(false);
      expect(specComplete({ ...base, joins: [{ type: 'inner', table: 'b', on: [] }] })).toBe(false);
      expect(
        specComplete({ ...base, joins: [{ type: 'inner', table: 'b', on: [{ leftTable: 't', leftCol: 'id', rightTable: 'b', rightCol: '' }] }] }),
      ).toBe(false);
      expect(
        specComplete({ ...base, joins: [{ type: 'inner', table: 'b', on: [{ leftTable: 't', leftCol: 'id', rightTable: 'b', rightCol: 'tid' }] }] }),
      ).toBe(true);
    });

    it('rejects a filter with no column', () => {
      expect(specComplete({ table: 't', columns: [{ name: 'a' }], where: [{ column: '', op: '=', value: '1' }] })).toBe(false);
    });
  });

  describe('previewSQL', () => {
    it('renders a single-table select with quoted idents + LIMIT', () => {
      const sql = previewSQL({ table: 'employees', columns: [{ name: 'name' }, { name: 'salary', alias: 'pay' }], limit: 50 }, 1000);
      expect(sql).toBe('SELECT "name", "salary" AS "pay" FROM "employees"\nLIMIT 50');
    });

    it('renders joins + qualified columns + where with combinator', () => {
      const spec: QuerySpec = {
        table: 'employees',
        joins: [{ type: 'left', table: 'departments', on: [{ leftTable: 'employees', leftCol: 'dept_id', rightTable: 'departments', rightCol: 'id' }] }],
        columns: [{ table: 'employees', name: 'name' }, { table: 'departments', name: 'name', alias: 'dept' }],
        where: [
          { table: 'employees', column: 'salary', op: '>=', value: '50000' },
          { table: 'departments', column: 'name', op: 'like', value: 'Fin%' },
        ],
        combinator: 'or',
        limit: 100,
      };
      const sql = previewSQL(spec, 1000);
      expect(sql).toContain('LEFT JOIN "departments" ON "employees"."dept_id" = "departments"."id"');
      expect(sql).toContain('"employees"."salary" >= 50000'); // numeric unquoted
      expect(sql).toContain(`"departments"."name" LIKE 'Fin%'`); // text quoted
      expect(sql).toContain(' OR ');
      expect(sql.endsWith('LIMIT 100')).toBe(true);
    });

    it('clamps the limit to maxRows and falls back to maxRows when unset', () => {
      expect(previewSQL({ table: 't', columns: [{ name: 'c' }], limit: 999999 }, 500).endsWith('LIMIT 500')).toBe(true);
      expect(previewSQL({ table: 't', columns: [{ name: 'c' }] }, 500).endsWith('LIMIT 500')).toBe(true);
    });

    it('escapes embedded quotes in identifiers and literals', () => {
      const sql = previewSQL({ table: 't', columns: [{ name: 'c' }], where: [{ column: 'c', op: '=', value: "O'Brien" }] }, 100);
      expect(sql).toContain(`= 'O''Brien'`);
    });

    it('empty table → empty preview; empty columns → SELECT *', () => {
      expect(previewSQL(emptySpec(), 100)).toBe('');
      expect(previewSQL({ table: 't', columns: [] }, 100)).toContain('SELECT * FROM "t"');
    });
  });
});
