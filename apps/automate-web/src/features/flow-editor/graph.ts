// Pure, framework-free graph logic shared by the zustand store. Keeping these
// out of the store makes them trivially unit-testable (React Flow rendering is
// hard in jsdom; the store/logic carry the behaviour).
import type { Connection } from '@xyflow/react';
import { NODE_BY_TYPE, type JSONSchema } from '@/lib/nodeRegistry';
import { validate, defaultsFor, type FormValues } from '@/features/node-config';
import type { FlowNode, FlowEdge } from './types';

let seq = 0;
/** Deterministic-ish id; tests can reset via resetIdSeq(). */
export function makeNodeId(nodeType: string): string {
  seq += 1;
  return `${nodeType.replace(/\W+/g, '_')}_${seq}`;
}
export function resetIdSeq(n = 0) {
  seq = n;
}

/** Create a new canvas node for a registry type at a position. */
export function createNode(
  nodeType: string,
  position: { x: number; y: number },
  id?: string,
): FlowNode {
  const def = NODE_BY_TYPE[nodeType];
  if (!def) throw new Error(`unknown node type: ${nodeType}`);
  return {
    id: id ?? makeNodeId(nodeType),
    type: 'automateNode',
    position,
    data: { nodeType, name: def.label },
  };
}

/**
 * Is this connection allowed? Rules:
 *  - target must have an input port (triggers have none → reject).
 *  - can't connect a node to itself.
 *  - no duplicate edge between the same source-handle/target-handle pair.
 */
export function canConnect(
  connection: Connection,
  nodes: FlowNode[],
  edges: FlowEdge[],
): boolean {
  if (!connection.source || !connection.target) return false;
  if (connection.source === connection.target) return false;

  const targetNode = nodes.find((n) => n.id === connection.target);
  if (!targetNode) return false;
  const targetDef = NODE_BY_TYPE[targetNode.data.nodeType];
  if (!targetDef || targetDef.inputs.length === 0) return false; // e.g. triggers

  const duplicate = edges.some(
    (e) =>
      e.source === connection.source &&
      e.target === connection.target &&
      (e.sourceHandle ?? null) === (connection.sourceHandle ?? null) &&
      (e.targetHandle ?? null) === (connection.targetHandle ?? null),
  );
  return !duplicate;
}

/**
 * Build the edge to add for an accepted connection. Edges from logic.if carry a
 * label (True/False) derived from the source handle so branches are readable.
 */
export function buildEdge(connection: Connection, nodes: FlowNode[]): FlowEdge {
  const sourceNode = nodes.find((n) => n.id === connection.source);
  const def = sourceNode ? NODE_BY_TYPE[sourceNode.data.nodeType] : undefined;
  const port = def?.outputs.find((o) => o.id === connection.sourceHandle);
  const label = def && def.outputs.length > 1 ? port?.label : undefined;

  return {
    id: `e_${connection.source}:${connection.sourceHandle ?? 'out'}->${connection.target}`,
    source: connection.source!,
    target: connection.target!,
    sourceHandle: connection.sourceHandle ?? undefined,
    targetHandle: connection.targetHandle ?? undefined,
    type: 'smoothstep',
    label,
  };
}

/** Resolve the effective config for a node (stored values over schema defaults). */
export function effectiveConfig(nodeType: string, stored?: FormValues): FormValues {
  const def = NODE_BY_TYPE[nodeType];
  const schema: JSONSchema = def?.schema ?? {};
  return { ...defaultsFor(schema), ...(stored ?? {}) };
}

/** Is a node's config valid/complete against its schema? */
export function isNodeValid(nodeType: string, stored?: FormValues): boolean {
  const def = NODE_BY_TYPE[nodeType];
  if (!def) return false;
  const values = effectiveConfig(nodeType, stored);
  return validate(def.schema, values).valid;
}

/**
 * Names of the nodes upstream of `nodeId` (all transitive ancestors following
 * edges backward). Used to populate the expression data picker so a field can
 * only reference nodes that actually run before it. Returns names in a stable
 * order (breadth-first from the target) with duplicates removed.
 */
export function upstreamNodeNames(
  nodeId: string,
  nodes: FlowNode[],
  edges: FlowEdge[],
): string[] {
  const nameById = new Map(nodes.map((n) => [n.id, n.data.name]));
  const seen = new Set<string>();
  const order: string[] = [];
  const queue = edges.filter((e) => e.target === nodeId).map((e) => e.source);
  while (queue.length > 0) {
    const src = queue.shift()!;
    if (seen.has(src)) continue;
    seen.add(src);
    const name = nameById.get(src);
    if (name) order.push(name);
    for (const e of edges) if (e.target === src) queue.push(e.source);
  }
  return order;
}

/** Names of nodes whose config is incomplete (used by the Validate action). */
export function invalidNodes(
  nodes: FlowNode[],
  configByNode: Record<string, FormValues>,
): { id: string; name: string }[] {
  return nodes
    .filter((n) => !isNodeValid(n.data.nodeType, configByNode[n.id]))
    .map((n) => ({ id: n.id, name: n.data.name }));
}
