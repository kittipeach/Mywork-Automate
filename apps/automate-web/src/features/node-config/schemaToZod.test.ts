import { describe, it, expect } from 'vitest';
import { NODE_BY_TYPE, type JSONSchema } from '@/lib/nodeRegistry';
import {
  buildZodSchema,
  validate,
  fieldsFor,
  resolveDependents,
  defaultsFor,
} from './schemaToZod';

const scheduleSchema = NODE_BY_TYPE['trigger.schedule'].schema;
const fileSchema = NODE_BY_TYPE['file.generate'].schema;
const ifSchema = NODE_BY_TYPE['logic.if'].schema;
const dbSchema = NODE_BY_TYPE['db.query'].schema;

describe('defaultsFor', () => {
  it('seeds defaults from schema, including active dependent branch', () => {
    const d = defaultsFor(scheduleSchema);
    expect(d.mode).toBe('simple');
    expect(d.timezone).toBe('Asia/Bangkok');
    // simple branch dependent is seeded
    expect(d.everyMinutes).toBe(1440);
    // cron branch is NOT active for the default 'simple'
    expect(d.cron).toBeUndefined();
  });

  it('seeds boolean fields to false and unset numbers to undefined', () => {
    const schema: JSONSchema = {
      type: 'object',
      properties: {
        notify: { type: 'boolean', title: 'Notify' },
        count: { type: 'number', title: 'Count' },
      },
    };
    const d = defaultsFor(schema);
    expect(d.notify).toBe(false);
    expect(d.count).toBeUndefined();
  });
});

describe('resolveDependents', () => {
  it('returns nothing when no dependentSchemas', () => {
    const { props, required } = resolveDependents(dbSchema, { mode: 'sql' });
    expect(Object.keys(props)).toHaveLength(0);
    expect(required.size).toBe(0);
  });

  it('activates the branch matching the controlling value', () => {
    const simple = resolveDependents(scheduleSchema, { mode: 'simple' });
    expect(Object.keys(simple.props)).toEqual(['everyMinutes']);
    expect(simple.required.has('everyMinutes')).toBe(true);

    const cron = resolveDependents(scheduleSchema, { mode: 'cron' });
    expect(Object.keys(cron.props)).toEqual(['cron']);
  });

  it('activates no branch when controlling value is unset', () => {
    const { props } = resolveDependents(scheduleSchema, {});
    expect(Object.keys(props)).toHaveLength(0);
  });
});

describe('fieldsFor', () => {
  it('lists base fields then active dependent fields', () => {
    const fields = fieldsFor(scheduleSchema, { mode: 'cron', timezone: 'UTC' });
    const names = fields.map((f) => f.name);
    expect(names).toContain('mode');
    expect(names).toContain('timezone');
    expect(names).toContain('cron');
    expect(names).not.toContain('everyMinutes');
    expect(fields.find((f) => f.name === 'cron')?.dependent).toBe(true);
    expect(fields.find((f) => f.name === 'mode')?.dependent).toBe(false);
  });

  it('marks required base fields from schema.required', () => {
    const fields = fieldsFor(dbSchema, { mode: 'sql' });
    expect(fields.find((f) => f.name === 'connectionId')?.required).toBe(true);
    expect(fields.find((f) => f.name === 'maxRows')?.required).toBe(false);
  });
});

describe('buildZodSchema + validate — strings & required', () => {
  it('fails when a required string is empty', () => {
    const r = validate(dbSchema, { connectionId: '', mode: 'sql', sql: 'SELECT 1', maxRows: 10 });
    expect(r.valid).toBe(false);
    expect(r.errors.connectionId).toBe('Required');
  });

  it('passes when all required strings present', () => {
    const r = validate(dbSchema, {
      connectionId: 'conn_hr',
      mode: 'sql',
      sql: 'SELECT 1',
      maxRows: 10,
    });
    expect(r.valid).toBe(true);
    expect(r.errors).toEqual({});
  });

  it('allows optional string to be empty', () => {
    const r = validate(dbSchema, { connectionId: 'c', mode: 'sql', sql: '', maxRows: 10 });
    expect(r.valid).toBe(true);
  });
});

describe('buildZodSchema + validate — enums', () => {
  it('rejects a value not in the enum', () => {
    const r = validate(dbSchema, { connectionId: 'c', mode: 'nonsense' });
    expect(r.valid).toBe(false);
    expect(r.errors.mode).toBeTruthy();
  });

  it('accepts a valid enum value', () => {
    const r = validate(dbSchema, { connectionId: 'c', mode: 'builder' });
    expect(r.valid).toBe(true);
  });

  it('fails a required enum left blank', () => {
    const r = validate(dbSchema, { connectionId: 'c', mode: '' });
    expect(r.valid).toBe(false);
    expect(r.errors.mode).toBeTruthy();
  });
});

describe('buildZodSchema + validate — number bounds', () => {
  it('enforces minimum', () => {
    const r = validate(dbSchema, { connectionId: 'c', mode: 'sql', maxRows: 0 });
    expect(r.valid).toBe(false);
    expect(r.errors.maxRows).toContain('≥');
  });

  it('enforces maximum', () => {
    const r = validate(dbSchema, { connectionId: 'c', mode: 'sql', maxRows: 999999 });
    expect(r.valid).toBe(false);
    expect(r.errors.maxRows).toContain('≤');
  });

  it('accepts a number within bounds', () => {
    const r = validate(dbSchema, { connectionId: 'c', mode: 'sql', maxRows: 1000 });
    expect(r.valid).toBe(true);
  });

  it('coerces numeric strings', () => {
    const r = validate(dbSchema, { connectionId: 'c', mode: 'sql', maxRows: '500' });
    expect(r.valid).toBe(true);
  });

  it('flags a required number left undefined (logic.if right)', () => {
    const r = validate(ifSchema, { left: '{{x}}', op: '>', right: undefined });
    expect(r.valid).toBe(false);
    expect(r.errors.right).toBeTruthy();
  });

  it('rejects a non-numeric string for a number field', () => {
    const r = validate(ifSchema, { left: '{{x}}', op: '>', right: 'abc' });
    expect(r.valid).toBe(false);
    expect(r.errors.right).toBeTruthy();
  });
});

describe('dependentSchemas validation', () => {
  it('requires everyMinutes when mode=simple', () => {
    const bad = validate(scheduleSchema, { mode: 'simple', timezone: 'UTC', everyMinutes: undefined });
    expect(bad.valid).toBe(false);
    expect(bad.errors.everyMinutes).toBeTruthy();

    const good = validate(scheduleSchema, { mode: 'simple', timezone: 'UTC', everyMinutes: 30 });
    expect(good.valid).toBe(true);
  });

  it('switches required field from everyMinutes to cron when mode changes', () => {
    // cron mode: everyMinutes no longer validated, cron required
    const missingCron = validate(scheduleSchema, { mode: 'cron', timezone: 'UTC' });
    expect(missingCron.valid).toBe(false);
    expect(missingCron.errors.cron).toBeTruthy();
    expect(missingCron.errors.everyMinutes).toBeUndefined();

    const withCron = validate(scheduleSchema, { mode: 'cron', timezone: 'UTC', cron: '0 6 * * *' });
    expect(withCron.valid).toBe(true);
  });

  it('file.generate shows+requires delimiter only when format=txt', () => {
    const txtFields = fieldsFor(fileSchema, { format: 'txt', filename: 'a.txt' }).map((f) => f.name);
    expect(txtFields).toContain('delimiter');

    const xlsxFields = fieldsFor(fileSchema, { format: 'xlsx', filename: 'a.xlsx' }).map((f) => f.name);
    expect(xlsxFields).not.toContain('delimiter');

    const badTxt = validate(fileSchema, { format: 'txt', filename: 'a.txt', delimiter: '' });
    expect(badTxt.valid).toBe(false);
    expect(badTxt.errors.delimiter).toBeTruthy();

    const goodTxt = validate(fileSchema, { format: 'txt', filename: 'a.txt', delimiter: '|' });
    expect(goodTxt.valid).toBe(true);
  });
});

describe('buildZodSchema shape', () => {
  it('produces a zod object that safeParses', () => {
    const zodSchema = buildZodSchema(fileSchema, { format: 'csv', filename: 'r.csv' });
    expect(zodSchema.safeParse({ format: 'csv', filename: 'r.csv' }).success).toBe(true);
  });

  it('handles boolean fields', () => {
    const schema: JSONSchema = {
      type: 'object',
      properties: { on: { type: 'boolean', title: 'On' } },
      required: [],
    };
    expect(validate(schema, { on: true }).valid).toBe(true);
    expect(validate(schema, { on: false }).valid).toBe(true);
  });
});
