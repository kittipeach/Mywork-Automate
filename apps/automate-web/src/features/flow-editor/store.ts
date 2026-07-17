'use client';

// Zustand store that owns all canvas state: nodes, edges, selection, per-node
// config + validity, and an undo/redo history (past/future snapshot stacks).
// Graph mutations that should be undoable push the *previous* graph onto `past`
// and clear `future` (commit()). Node changes from React Flow (drag/select) are
// applied without a history commit so dragging doesn't spam the stack.
import { create } from 'zustand';
import {
  applyNodeChanges,
  applyEdgeChanges,
  type NodeChange,
  type EdgeChange,
  type Connection,
} from '@xyflow/react';
import { NODE_BY_TYPE } from '@/lib/nodeRegistry';
import type { FormValues } from '@/features/node-config';
import type {
  FlowNode,
  FlowEdge,
  GraphSnapshot,
  ConfigByNode,
  ValidityByNode,
  StatusByNode,
} from './types';
import {
  createNode,
  canConnect,
  buildEdge,
  isNodeValid,
} from './graph';

export const HISTORY_LIMIT = 30;

export type FlowState = {
  nodes: FlowNode[];
  edges: FlowEdge[];
  selectedNodeId: string | null;
  configByNode: ConfigByNode;
  validityByNode: ValidityByNode;
  /** per-node run status during a test run; empty when not running. */
  statusByNode: StatusByNode;
  past: GraphSnapshot[];
  future: GraphSnapshot[];

  // React Flow passthrough (not history-committing)
  onNodesChange: (changes: NodeChange[]) => void;
  onEdgesChange: (changes: EdgeChange[]) => void;

  // history-committing mutations
  addNode: (nodeType: string, position: { x: number; y: number }) => string | null;
  connect: (connection: Connection) => boolean;
  deleteSelected: () => void;
  deleteNode: (id: string) => void;

  // selection + config
  selectNode: (id: string | null) => void;
  setNodeConfig: (id: string, values: FormValues) => void;
  renameNode: (id: string, name: string) => void;

  // test-run status overlay (E3-S4)
  setStatusByNode: (statusByNode: StatusByNode) => void;
  clearRunStatus: () => void;

  // history
  undo: () => void;
  redo: () => void;
  canUndo: () => boolean;
  canRedo: () => boolean;

  // bootstrap
  setGraph: (snapshot: GraphSnapshot, config?: ConfigByNode) => void;
};

function snapshot(state: Pick<FlowState, 'nodes' | 'edges'>): GraphSnapshot {
  return { nodes: state.nodes, edges: state.edges };
}

function recomputeValidity(nodes: FlowNode[], config: ConfigByNode): ValidityByNode {
  const out: ValidityByNode = {};
  for (const n of nodes) out[n.id] = isNodeValid(n.data.nodeType, config[n.id]);
  return out;
}

export const useFlowStore = create<FlowState>((set, get) => ({
  nodes: [],
  edges: [],
  selectedNodeId: null,
  configByNode: {},
  validityByNode: {},
  statusByNode: {},
  past: [],
  future: [],

  onNodesChange: (changes) =>
    set((s) => {
      const nodes = applyNodeChanges(changes, s.nodes) as FlowNode[];
      // reflect a selection change coming from React Flow into selectedNodeId
      const selected = nodes.find((n) => n.selected);
      return {
        nodes,
        selectedNodeId: selected ? selected.id : s.selectedNodeId,
      };
    }),

  onEdgesChange: (changes) =>
    set((s) => ({ edges: applyEdgeChanges(changes, s.edges) as FlowEdge[] })),

  addNode: (nodeType, position) => {
    if (!NODE_BY_TYPE[nodeType]) return null;
    const node = createNode(nodeType, position);
    set((s) => {
      const past = [...s.past, snapshot(s)].slice(-HISTORY_LIMIT);
      const nodes = [...s.nodes, node];
      return {
        nodes,
        past,
        future: [],
        validityByNode: recomputeValidity(nodes, s.configByNode),
      };
    });
    return node.id;
  },

  connect: (connection) => {
    const s = get();
    if (!canConnect(connection, s.nodes, s.edges)) return false;
    const edge = buildEdge(connection, s.nodes);
    set((cur) => {
      const past = [...cur.past, snapshot(cur)].slice(-HISTORY_LIMIT);
      return { edges: [...cur.edges, edge], past, future: [] };
    });
    return true;
  },

  deleteNode: (id) =>
    set((s) => {
      const past = [...s.past, snapshot(s)].slice(-HISTORY_LIMIT);
      const nodes = s.nodes.filter((n) => n.id !== id);
      const edges = s.edges.filter((e) => e.source !== id && e.target !== id);
      const configByNode = { ...s.configByNode };
      delete configByNode[id];
      return {
        nodes,
        edges,
        configByNode,
        past,
        future: [],
        selectedNodeId: s.selectedNodeId === id ? null : s.selectedNodeId,
        validityByNode: recomputeValidity(nodes, configByNode),
      };
    }),

  deleteSelected: () =>
    set((s) => {
      const selectedNodeIds = new Set(s.nodes.filter((n) => n.selected).map((n) => n.id));
      if (s.selectedNodeId) selectedNodeIds.add(s.selectedNodeId);
      const selectedEdgeIds = new Set(s.edges.filter((e) => e.selected).map((e) => e.id));
      if (selectedNodeIds.size === 0 && selectedEdgeIds.size === 0) return s;

      const past = [...s.past, snapshot(s)].slice(-HISTORY_LIMIT);
      const nodes = s.nodes.filter((n) => !selectedNodeIds.has(n.id));
      const edges = s.edges.filter(
        (e) =>
          !selectedEdgeIds.has(e.id) &&
          !selectedNodeIds.has(e.source) &&
          !selectedNodeIds.has(e.target),
      );
      const configByNode = { ...s.configByNode };
      selectedNodeIds.forEach((id) => delete configByNode[id]);
      return {
        nodes,
        edges,
        configByNode,
        past,
        future: [],
        selectedNodeId: null,
        validityByNode: recomputeValidity(nodes, configByNode),
      };
    }),

  selectNode: (id) =>
    set((s) => ({
      selectedNodeId: id,
      nodes: s.nodes.map((n) => ({ ...n, selected: n.id === id })),
    })),

  setNodeConfig: (id, values) =>
    set((s) => {
      const configByNode = { ...s.configByNode, [id]: values };
      const node = s.nodes.find((n) => n.id === id);
      const validityByNode = { ...s.validityByNode };
      if (node) validityByNode[id] = isNodeValid(node.data.nodeType, values);
      return { configByNode, validityByNode };
    }),

  renameNode: (id, name) =>
    set((s) => ({
      nodes: s.nodes.map((n) => (n.id === id ? { ...n, data: { ...n.data, name } } : n)),
    })),

  setStatusByNode: (statusByNode) => set(() => ({ statusByNode })),
  clearRunStatus: () => set(() => ({ statusByNode: {} })),

  undo: () =>
    set((s) => {
      if (s.past.length === 0) return s;
      const previous = s.past[s.past.length - 1];
      const past = s.past.slice(0, -1);
      const future = [snapshot(s), ...s.future].slice(0, HISTORY_LIMIT);
      const selectedNodeId =
        s.selectedNodeId && previous.nodes.some((n) => n.id === s.selectedNodeId)
          ? s.selectedNodeId
          : null;
      return {
        nodes: previous.nodes,
        edges: previous.edges,
        past,
        future,
        selectedNodeId,
        validityByNode: recomputeValidity(previous.nodes, s.configByNode),
      };
    }),

  redo: () =>
    set((s) => {
      if (s.future.length === 0) return s;
      const next = s.future[0];
      const future = s.future.slice(1);
      const past = [...s.past, snapshot(s)].slice(-HISTORY_LIMIT);
      return {
        nodes: next.nodes,
        edges: next.edges,
        past,
        future,
        validityByNode: recomputeValidity(next.nodes, s.configByNode),
      };
    }),

  canUndo: () => get().past.length > 0,
  canRedo: () => get().future.length > 0,

  setGraph: (snap, config = {}) =>
    set(() => ({
      nodes: snap.nodes,
      edges: snap.edges,
      configByNode: config,
      validityByNode: recomputeValidity(snap.nodes, config),
      statusByNode: {},
      selectedNodeId: null,
      past: [],
      future: [],
    })),
}));
