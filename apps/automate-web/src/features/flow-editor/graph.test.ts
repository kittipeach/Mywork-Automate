import { describe, it, expect, beforeEach } from 'vitest';
import type { Connection } from '@xyflow/react';
import {
  createNode,
  canConnect,
  buildEdge,
  isNodeValid,
  invalidNodes,
  effectiveConfig,
  makeNodeId,
  resetIdSeq,
} from './graph';
import type { FlowNode, FlowEdge } from './types';

beforeEach(() => resetIdSeq(0));

const trigger = createNode('trigger.schedule', { x: 0, y: 0 }, 'trg');
const query = createNode('db.query', { x: 100, y: 0 }, 'qry');
const ifNode = createNode('logic.if', { x: 200, y: 0 }, 'iff');
const file = createNode('file.generate', { x: 300, y: 0 }, 'fil');
const nodes: FlowNode[] = [trigger, query, ifNode, file];

describe('createNode', () => {
  it('builds a node from a registry type with the label as default name', () => {
    const n = createNode('db.query', { x: 5, y: 6 }, 'x');
    expect(n.data.nodeType).toBe('db.query');
    expect(n.data.name).toBe('DB Query');
    expect(n.type).toBe('automateNode');
    expect(n.position).toEqual({ x: 5, y: 6 });
  });

  it('throws for an unknown type', () => {
    expect(() => createNode('nope.nope', { x: 0, y: 0 })).toThrow(/unknown/);
  });

  it('generates unique ids via makeNodeId', () => {
    const a = makeNodeId('db.query');
    const b = makeNodeId('db.query');
    expect(a).not.toBe(b);
  });
});

describe('canConnect', () => {
  it('accepts a valid output→input connection', () => {
    const c: Connection = { source: 'trg', sourceHandle: 'out', target: 'qry', targetHandle: 'in' };
    expect(canConnect(c, nodes, [])).toBe(true);
  });

  it('rejects connecting into a trigger (no input port)', () => {
    const c: Connection = { source: 'qry', sourceHandle: 'out', target: 'trg', targetHandle: null };
    expect(canConnect(c, nodes, [])).toBe(false);
  });

  it('rejects a self-connection', () => {
    const c: Connection = { source: 'qry', sourceHandle: 'out', target: 'qry', targetHandle: 'in' };
    expect(canConnect(c, nodes, [])).toBe(false);
  });

  it('rejects when source or target is missing', () => {
    expect(canConnect({ source: '', sourceHandle: null, target: 'qry', targetHandle: 'in' }, nodes, [])).toBe(false);
    expect(canConnect({ source: 'trg', sourceHandle: 'out', target: '', targetHandle: null }, nodes, [])).toBe(false);
  });

  it('rejects an unknown target node', () => {
    const c: Connection = { source: 'trg', sourceHandle: 'out', target: 'ghost', targetHandle: 'in' };
    expect(canConnect(c, nodes, [])).toBe(false);
  });

  it('rejects a duplicate edge (same handles)', () => {
    const existing: FlowEdge[] = [
      { id: 'e', source: 'trg', sourceHandle: 'out', target: 'qry', targetHandle: 'in' },
    ];
    const c: Connection = { source: 'trg', sourceHandle: 'out', target: 'qry', targetHandle: 'in' };
    expect(canConnect(c, nodes, existing)).toBe(false);
  });

  it('allows a second edge from a different source handle', () => {
    const existing: FlowEdge[] = [
      { id: 'e', source: 'iff', sourceHandle: 'true', target: 'fil', targetHandle: 'in' },
    ];
    const c: Connection = { source: 'iff', sourceHandle: 'false', target: 'fil', targetHandle: 'in' };
    expect(canConnect(c, nodes, existing)).toBe(true);
  });
});

describe('buildEdge', () => {
  it('adds a smoothstep edge with no label for single-output sources', () => {
    const e = buildEdge({ source: 'trg', sourceHandle: 'out', target: 'qry', targetHandle: 'in' }, nodes);
    expect(e.type).toBe('smoothstep');
    expect(e.label).toBeUndefined();
    expect(e.source).toBe('trg');
    expect(e.target).toBe('qry');
  });

  it('labels branch edges from logic.if with the handle label', () => {
    const eTrue = buildEdge({ source: 'iff', sourceHandle: 'true', target: 'fil', targetHandle: 'in' }, nodes);
    expect(eTrue.label).toBe('True');
    const eFalse = buildEdge({ source: 'iff', sourceHandle: 'false', target: 'fil', targetHandle: 'in' }, nodes);
    expect(eFalse.label).toBe('False');
  });
});

describe('isNodeValid / effectiveConfig', () => {
  it('merges defaults so a schedule node is valid out of the box', () => {
    expect(isNodeValid('trigger.schedule')).toBe(true);
  });

  it('is invalid when a required field is missing (db.query needs connectionId)', () => {
    expect(isNodeValid('db.query', { mode: 'sql' })).toBe(false);
    expect(isNodeValid('db.query', { connectionId: 'c', mode: 'sql' })).toBe(true);
  });

  it('effectiveConfig layers stored values over defaults', () => {
    const cfg = effectiveConfig('trigger.schedule', { timezone: 'UTC' });
    expect(cfg.timezone).toBe('UTC');
    expect(cfg.mode).toBe('simple'); // from default
  });

  it('unknown node type is invalid', () => {
    expect(isNodeValid('nope')).toBe(false);
  });
});

describe('invalidNodes', () => {
  it('returns the nodes whose config is incomplete', () => {
    const list = invalidNodes(
      [query, file],
      { qry: { mode: 'sql' }, fil: { format: 'xlsx', filename: 'a.xlsx' } },
    );
    // Wait: ids are from createNode helpers above (qry/fil)
    const ids = list.map((n) => n.id);
    expect(ids).toContain('qry'); // missing connectionId
    expect(ids).not.toContain('fil'); // fully configured
  });
});
