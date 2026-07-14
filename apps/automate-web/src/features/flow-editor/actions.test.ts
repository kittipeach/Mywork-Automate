import { describe, it, expect } from 'vitest';
import { validateFlow, testRun, publishFlow } from './actions';
import { seedFlow } from './seed';
import { createNode } from './graph';
import type { FlowNode } from './types';

const { snapshot, config } = seedFlow();
const nodes = snapshot.nodes as FlowNode[];

describe('validateFlow', () => {
  it('warns when there are no nodes', () => {
    expect(validateFlow([], {}).tone).toBe('warning');
  });

  it('flags nodes missing required config', () => {
    // seed's MFT node is missing connectionId
    const b = validateFlow(nodes, config);
    expect(b.tone).toBe('danger');
    expect(b.detail).toContain('MFT to Bank');
  });

  it('passes when every node is fully configured', () => {
    const fixed = { ...config, n_mft: { ...config.n_mft, connectionId: 'conn_bankmft' } };
    const b = validateFlow(nodes, fixed);
    expect(b.tone).toBe('success');
  });
});

describe('testRun', () => {
  it('refuses to run when config is invalid', () => {
    expect(testRun(nodes, config).tone).toBe('warning');
  });

  it('reports a full success sweep when valid', () => {
    const fixed = { ...config, n_mft: { ...config.n_mft, connectionId: 'conn_bankmft' } };
    const b = testRun(nodes, fixed);
    expect(b.tone).toBe('success');
    expect(b.detail).toContain(`${nodes.length}/${nodes.length}`);
  });
});

describe('publishFlow', () => {
  it('blocks publish when invalid', () => {
    expect(publishFlow(nodes, config).tone).toBe('danger');
  });

  it('publishes when valid', () => {
    const single = [createNode('trigger.manual', { x: 0, y: 0 }, 'm')];
    expect(publishFlow(single, {}).tone).toBe('success');
  });
});
