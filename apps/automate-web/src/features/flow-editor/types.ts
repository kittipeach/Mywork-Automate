import type { Node, Edge } from '@xyflow/react';
import type { FormValues } from '@/features/node-config';

/** Per-node data carried on the React Flow node. */
export type FlowNodeData = {
  /** registry node type key, e.g. 'db.query' */
  nodeType: string;
  /** user-facing name shown on the node (defaults to the registry label) */
  name: string;
};

export type FlowNode = Node<FlowNodeData>;
export type FlowEdge = Edge;

/** A serialisable snapshot of the canvas graph. */
export type GraphSnapshot = {
  nodes: FlowNode[];
  edges: FlowEdge[];
};

export type ConfigByNode = Record<string, FormValues>;
export type ValidityByNode = Record<string, boolean>;
