import { describe, it, expect } from 'vitest';
import { deriveFiles, isExpired, type GeneratedFile } from './files';
import type { Execution } from '@/lib/mock/store';

function exe(over: Partial<Execution> = {}): Execution {
  return {
    id: 'exe_1',
    flowId: 'flw_1',
    flowName: 'Payroll → Bank MFT',
    status: 'success',
    trigger: 'schedule',
    startedAt: '2026-07-14T06:00:00.000Z',
    durationMs: 1000,
    version: 3,
    steps: [],
    ...over,
  };
}

describe('deriveFiles', () => {
  it('produces a row per successful file.generate step', () => {
    const files = deriveFiles([
      exe({
        steps: [
          { nodeId: 't1', nodeName: 'Schedule', nodeType: 'trigger.schedule', status: 'success', durationMs: 1, inputCount: 0, outputCount: 1 },
          { nodeId: 'f1', nodeName: 'Gen XLSX', nodeType: 'file.generate', status: 'success', durationMs: 10, inputCount: 45012, outputCount: 1 },
        ],
      }),
    ]);
    expect(files).toHaveLength(1);
    expect(files[0]).toMatchObject({
      id: 'exe_1_f1',
      ext: 'xlsx',
      filename: 'payroll_bank_mft.xlsx',
      flowName: 'Payroll → Bank MFT',
      executionId: 'exe_1',
    });
    expect(files[0].sizeBytes).toBe(45012 * 180);
  });

  it('ignores non-file and failed/other-status steps', () => {
    const files = deriveFiles([
      exe({
        steps: [
          { nodeId: 'q1', nodeName: 'Query', nodeType: 'db.query', status: 'success', durationMs: 1, inputCount: 1, outputCount: 5 },
          { nodeId: 'f1', nodeName: 'Gen CSV', nodeType: 'file.generate', status: 'failed', durationMs: 1, inputCount: 1, outputCount: 0 },
        ],
      }),
    ]);
    expect(files).toHaveLength(0);
  });

  it('detects the extension from the node name (default xlsx)', () => {
    const [csv, def] = deriveFiles([
      exe({
        id: 'exe_2',
        steps: [
          { nodeId: 'a', nodeName: 'Export CSV report', nodeType: 'file.generate', status: 'success', durationMs: 1, inputCount: 2, outputCount: 1 },
          { nodeId: 'b', nodeName: 'Generate output', nodeType: 'file.generate', status: 'success', durationMs: 1, inputCount: 2, outputCount: 1 },
        ],
      }),
    ]);
    expect(csv.ext).toBe('csv');
    expect(def.ext).toBe('xlsx');
  });

  it('sets expiry 7 days after creation', () => {
    const [f] = deriveFiles([
      exe({ steps: [{ nodeId: 'f1', nodeName: 'Gen', nodeType: 'file.generate', status: 'success', durationMs: 1, inputCount: 1, outputCount: 1 }] }),
    ]);
    expect(f.createdAt).toBe('2026-07-14T06:00:00.000Z');
    expect(f.expiresAt).toBe('2026-07-21T06:00:00.000Z');
  });

  it('sorts newest first across executions', () => {
    const older = exe({ id: 'exe_old', startedAt: '2026-07-10T06:00:00.000Z', steps: [{ nodeId: 'f', nodeName: 'g', nodeType: 'file.generate', status: 'success', durationMs: 1, inputCount: 1, outputCount: 1 }] });
    const newer = exe({ id: 'exe_new', startedAt: '2026-07-14T06:00:00.000Z', steps: [{ nodeId: 'f', nodeName: 'g', nodeType: 'file.generate', status: 'success', durationMs: 1, inputCount: 1, outputCount: 1 }] });
    const files = deriveFiles([older, newer]);
    expect(files.map((f) => f.executionId)).toEqual(['exe_new', 'exe_old']);
  });

  it('returns empty for no executions', () => {
    expect(deriveFiles([])).toEqual([]);
  });
});

describe('isExpired', () => {
  const file: GeneratedFile = {
    id: 'x', filename: 'x.xlsx', ext: 'xlsx', sizeBytes: 1, createdAt: '2026-07-14T06:00:00.000Z',
    expiresAt: '2026-07-21T06:00:00.000Z', flowName: 'F', executionId: 'exe_1',
  };
  it('false before expiry', () => {
    expect(isExpired(file, new Date('2026-07-20T00:00:00.000Z'))).toBe(false);
  });
  it('true after expiry', () => {
    expect(isExpired(file, new Date('2026-07-22T00:00:00.000Z'))).toBe(true);
  });
  it('false when expiry is unparseable', () => {
    expect(isExpired({ ...file, expiresAt: 'bad' }, new Date('2030-01-01T00:00:00.000Z'))).toBe(false);
  });
});
