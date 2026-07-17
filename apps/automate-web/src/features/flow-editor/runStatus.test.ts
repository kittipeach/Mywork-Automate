import { describe, it, expect } from 'vitest';
import { stepStatusByNode, summarise, isTerminal } from './runStatus';
import type { Execution } from '@/lib/mock/store';
import type { RunStatus } from '@/components/ui/Badge';

function step(nodeId: string, status: RunStatus): Execution['steps'][number] {
  return { nodeId, nodeName: nodeId, nodeType: 'x', status, durationMs: 0, inputCount: 0, outputCount: 0 };
}

function exec(status: RunStatus, steps: Execution['steps']): Execution {
  return {
    id: 'exe_1', flowId: 'flw_1', flowName: 'F', status, trigger: 'manual',
    startedAt: '', durationMs: 0, version: 1, steps,
  };
}

describe('stepStatusByNode', () => {
  it('projects each step status onto its node id', () => {
    const map = stepStatusByNode([step('a', 'success'), step('b', 'running'), step('c', 'failed')]);
    expect(map).toEqual({ a: 'success', b: 'running', c: 'failed' });
  });

  it('returns an empty map for undefined/empty steps', () => {
    expect(stepStatusByNode(undefined)).toEqual({});
    expect(stepStatusByNode([])).toEqual({});
  });

  it('later step wins for a duplicate node id', () => {
    const map = stepStatusByNode([step('a', 'running'), step('a', 'success')]);
    expect(map).toEqual({ a: 'success' });
  });
});

describe('isTerminal', () => {
  it('is true for success/failed/cancelled', () => {
    expect(isTerminal('success')).toBe(true);
    expect(isTerminal('failed')).toBe(true);
    expect(isTerminal('cancelled')).toBe(true);
  });

  it('is false for in-flight and unknown statuses', () => {
    expect(isTerminal('running')).toBe(false);
    expect(isTerminal('queued')).toBe(false);
    expect(isTerminal(undefined)).toBe(false);
    expect(isTerminal('whatever')).toBe(false);
  });
});

describe('summarise', () => {
  it('returns null when there is no execution', () => {
    expect(summarise(undefined)).toBeNull();
  });

  it('counts done/failed/running and marks terminal', () => {
    const e = exec('running', [
      step('a', 'success'),
      step('b', 'success'),
      step('c', 'running'),
      step('d', 'queued'),
      step('e', 'failed'),
    ]);
    expect(summarise(e)).toEqual({
      status: 'running',
      total: 5,
      done: 2,
      failed: 1,
      running: 2, // running + queued
      terminal: false,
    });
  });

  it('marks a finished run terminal', () => {
    const e = exec('success', [step('a', 'success')]);
    expect(summarise(e)).toMatchObject({ status: 'success', done: 1, terminal: true });
  });

  it('handles an execution with no steps', () => {
    const e = exec('queued', []);
    expect(summarise(e)).toEqual({
      status: 'queued', total: 0, done: 0, failed: 0, running: 0, terminal: false,
    });
  });
});
