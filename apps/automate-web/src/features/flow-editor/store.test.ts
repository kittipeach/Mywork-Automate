import { describe, it, expect, beforeEach } from 'vitest';
import { useFlowStore, HISTORY_LIMIT } from './store';
import { resetIdSeq } from './graph';
import { seedFlow } from './seed';

const store = () => useFlowStore.getState();

function reset() {
  resetIdSeq(0);
  useFlowStore.setState({
    nodes: [],
    edges: [],
    selectedNodeId: null,
    configByNode: {},
    validityByNode: {},
    past: [],
    future: [],
  });
}

beforeEach(reset);

describe('addNode', () => {
  it('adds a node and returns its id', () => {
    const id = store().addNode('db.query', { x: 0, y: 0 });
    expect(id).toBeTruthy();
    expect(store().nodes).toHaveLength(1);
    expect(store().nodes[0].data.nodeType).toBe('db.query');
  });

  it('computes validity for the new node (db.query invalid by default)', () => {
    const id = store().addNode('db.query', { x: 0, y: 0 })!;
    expect(store().validityByNode[id]).toBe(false);
    // schedule is valid out of the box
    const sid = store().addNode('trigger.schedule', { x: 10, y: 0 })!;
    expect(store().validityByNode[sid]).toBe(true);
  });

  it('rejects an unknown node type', () => {
    const id = store().addNode('nope.nope', { x: 0, y: 0 });
    expect(id).toBeNull();
    expect(store().nodes).toHaveLength(0);
  });

  it('pushes history and clears future', () => {
    store().addNode('db.query', { x: 0, y: 0 });
    expect(store().past).toHaveLength(1);
    expect(store().future).toHaveLength(0);
  });
});

describe('connect', () => {
  it('adds a valid edge and returns true', () => {
    const a = store().addNode('trigger.schedule', { x: 0, y: 0 })!;
    const b = store().addNode('db.query', { x: 100, y: 0 })!;
    const ok = store().connect({ source: a, sourceHandle: 'out', target: b, targetHandle: 'in' });
    expect(ok).toBe(true);
    expect(store().edges).toHaveLength(1);
  });

  it('rejects connecting into a trigger', () => {
    const a = store().addNode('db.query', { x: 0, y: 0 })!;
    const t = store().addNode('trigger.schedule', { x: 100, y: 0 })!;
    const ok = store().connect({ source: a, sourceHandle: 'out', target: t, targetHandle: null });
    expect(ok).toBe(false);
    expect(store().edges).toHaveLength(0);
  });

  it('carries the source handle label for logic.if branches', () => {
    const iff = store().addNode('logic.if', { x: 0, y: 0 })!;
    const file = store().addNode('file.generate', { x: 100, y: 0 })!;
    store().connect({ source: iff, sourceHandle: 'true', target: file, targetHandle: 'in' });
    expect(store().edges[0].label).toBe('True');
    expect(store().edges[0].sourceHandle).toBe('true');
  });
});

describe('undo / redo', () => {
  it('undoes an addNode', () => {
    store().addNode('db.query', { x: 0, y: 0 });
    expect(store().nodes).toHaveLength(1);
    store().undo();
    expect(store().nodes).toHaveLength(0);
    expect(store().canRedo()).toBe(true);
  });

  it('redoes after undo', () => {
    store().addNode('db.query', { x: 0, y: 0 });
    store().undo();
    store().redo();
    expect(store().nodes).toHaveLength(1);
  });

  it('is a no-op when there is nothing to undo/redo', () => {
    expect(store().canUndo()).toBe(false);
    store().undo();
    expect(store().nodes).toHaveLength(0);
    expect(store().canRedo()).toBe(false);
    store().redo();
    expect(store().nodes).toHaveLength(0);
  });

  it('undo then a new mutation clears the redo stack', () => {
    store().addNode('db.query', { x: 0, y: 0 });
    store().addNode('logic.if', { x: 50, y: 0 });
    store().undo();
    expect(store().canRedo()).toBe(true);
    store().addNode('file.generate', { x: 90, y: 0 });
    expect(store().canRedo()).toBe(false);
  });

  it('keeps at least 20 (caps at HISTORY_LIMIT) history entries', () => {
    for (let i = 0; i < HISTORY_LIMIT + 10; i++) {
      store().addNode('db.query', { x: i, y: 0 });
    }
    expect(store().past.length).toBe(HISTORY_LIMIT);
    expect(HISTORY_LIMIT).toBeGreaterThanOrEqual(20);
    // can still undo many times
    let undos = 0;
    while (store().canUndo()) {
      store().undo();
      undos++;
    }
    expect(undos).toBe(HISTORY_LIMIT);
  });

  it('round-trips edges through undo/redo', () => {
    const a = store().addNode('trigger.schedule', { x: 0, y: 0 })!;
    const b = store().addNode('db.query', { x: 100, y: 0 })!;
    store().connect({ source: a, sourceHandle: 'out', target: b, targetHandle: 'in' });
    expect(store().edges).toHaveLength(1);
    store().undo();
    expect(store().edges).toHaveLength(0);
    store().redo();
    expect(store().edges).toHaveLength(1);
  });
});

describe('selection + config + validity', () => {
  it('selectNode marks the node selected and sets selectedNodeId', () => {
    const id = store().addNode('db.query', { x: 0, y: 0 })!;
    store().selectNode(id);
    expect(store().selectedNodeId).toBe(id);
    expect(store().nodes.find((n) => n.id === id)?.selected).toBe(true);
  });

  it('setNodeConfig updates validity from invalid → valid', () => {
    const id = store().addNode('db.query', { x: 0, y: 0 })!;
    expect(store().validityByNode[id]).toBe(false);
    store().setNodeConfig(id, { connectionId: 'conn_hr', mode: 'sql', sql: 'SELECT 1', maxRows: 10 });
    expect(store().validityByNode[id]).toBe(true);
  });

  it('renameNode changes the display name', () => {
    const id = store().addNode('db.query', { x: 0, y: 0 })!;
    store().renameNode(id, 'My Query');
    expect(store().nodes.find((n) => n.id === id)?.data.name).toBe('My Query');
  });
});

describe('delete', () => {
  it('deleteNode removes the node, its edges and config', () => {
    const a = store().addNode('trigger.schedule', { x: 0, y: 0 })!;
    const b = store().addNode('db.query', { x: 100, y: 0 })!;
    store().connect({ source: a, sourceHandle: 'out', target: b, targetHandle: 'in' });
    store().setNodeConfig(b, { connectionId: 'c', mode: 'sql' });
    store().deleteNode(b);
    expect(store().nodes.find((n) => n.id === b)).toBeUndefined();
    expect(store().edges).toHaveLength(0);
    expect(store().configByNode[b]).toBeUndefined();
  });

  it('deleteSelected removes the selected node and clears selection', () => {
    const a = store().addNode('db.query', { x: 0, y: 0 })!;
    store().selectNode(a);
    store().deleteSelected();
    expect(store().nodes).toHaveLength(0);
    expect(store().selectedNodeId).toBeNull();
  });

  it('deleteSelected is a no-op with nothing selected', () => {
    store().addNode('db.query', { x: 0, y: 0 });
    const before = store().nodes.length;
    store().deleteSelected();
    expect(store().nodes.length).toBe(before);
  });
});

describe('React Flow passthrough changes', () => {
  it('onNodesChange applies a position change without touching history', () => {
    const id = store().addNode('db.query', { x: 0, y: 0 })!;
    const before = store().past.length;
    store().onNodesChange([
      { id, type: 'position', position: { x: 50, y: 60 }, dragging: false },
    ]);
    const moved = store().nodes.find((n) => n.id === id);
    expect(moved?.position).toEqual({ x: 50, y: 60 });
    expect(store().past.length).toBe(before); // drags don't commit history
  });

  it('onNodesChange reflects a React-Flow selection into selectedNodeId', () => {
    const id = store().addNode('db.query', { x: 0, y: 0 })!;
    store().onNodesChange([{ id, type: 'select', selected: true }]);
    expect(store().selectedNodeId).toBe(id);
  });

  it('onEdgesChange applies edge changes', () => {
    const a = store().addNode('trigger.schedule', { x: 0, y: 0 })!;
    const b = store().addNode('db.query', { x: 100, y: 0 })!;
    store().connect({ source: a, sourceHandle: 'out', target: b, targetHandle: 'in' });
    const edgeId = store().edges[0].id;
    store().onEdgesChange([{ id: edgeId, type: 'select', selected: true }]);
    expect(store().edges[0].selected).toBe(true);
  });

  it('deleteSelected removes a selected edge', () => {
    const a = store().addNode('trigger.schedule', { x: 0, y: 0 })!;
    const b = store().addNode('db.query', { x: 100, y: 0 })!;
    store().connect({ source: a, sourceHandle: 'out', target: b, targetHandle: 'in' });
    const edgeId = store().edges[0].id;
    store().onEdgesChange([{ id: edgeId, type: 'select', selected: true }]);
    store().deleteSelected();
    expect(store().edges).toHaveLength(0);
    expect(store().nodes).toHaveLength(2); // nodes untouched
  });

  it('undo preserves selection when the node still exists', () => {
    const a = store().addNode('db.query', { x: 0, y: 0 })!;
    // add a second node (this is the undoable step we will revert)
    store().addNode('logic.if', { x: 60, y: 0 });
    store().selectNode(a);
    store().undo(); // removes the logic.if; 'a' still present
    expect(store().selectedNodeId).toBe(a);
  });

  it('undo clears selection when the selected node no longer exists', () => {
    const a = store().addNode('db.query', { x: 0, y: 0 })!;
    store().selectNode(a);
    store().undo(); // reverts the addNode of 'a'
    expect(store().selectedNodeId).toBeNull();
  });
});

describe('setGraph + seedFlow', () => {
  it('loads a snapshot with config and computes validity', () => {
    const { snapshot, config } = seedFlow();
    store().setGraph(snapshot, config);
    expect(store().nodes.length).toBe(snapshot.nodes.length);
    expect(store().edges.length).toBe(snapshot.edges.length);
    // MFT is intentionally missing connectionId in the seed → invalid
    expect(store().validityByNode['n_mft']).toBe(false);
    // query is fully configured → valid
    expect(store().validityByNode['n_query']).toBe(true);
    // history is reset on load
    expect(store().past).toHaveLength(0);
    expect(store().future).toHaveLength(0);
  });
});
