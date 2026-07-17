// Pure helpers for the test-run overlay (E3-S4). Framework-free so they can be
// unit-tested without React. They turn an execution's per-step list into the
// `statusByNode` map the canvas overlays as node rings, and summarise the run for
// the compact status strip.
import type { Execution } from '@/lib/mock/store';
import type { RunStatus } from '@/components/ui/Badge';
import type { StatusByNode } from './types';

/**
 * Map each execution step onto its canvas node id. The API's step.nodeId matches
 * the graph node id, so this is a straight projection. Later steps win if a node
 * appears twice (shouldn't happen, but keeps it deterministic).
 */
export function stepStatusByNode(steps: Execution['steps'] | undefined): StatusByNode {
  const out: StatusByNode = {};
  for (const s of steps ?? []) out[s.nodeId] = s.status;
  return out;
}

export type RunSummary = {
  status: RunStatus;
  total: number;
  done: number;
  failed: number;
  running: number;
  /** true once the run has reached a terminal overall status. */
  terminal: boolean;
};

const TERMINAL: RunStatus[] = ['success', 'failed', 'cancelled'];

/** Is an overall run status terminal (polling can stop)? */
export function isTerminal(status?: RunStatus | string): boolean {
  return TERMINAL.includes(status as RunStatus);
}

/** Summarise an execution for the run-status strip. */
export function summarise(execution: Execution | undefined): RunSummary | null {
  if (!execution) return null;
  const steps = execution.steps ?? [];
  return {
    status: execution.status,
    total: steps.length,
    done: steps.filter((s) => s.status === 'success').length,
    failed: steps.filter((s) => s.status === 'failed').length,
    running: steps.filter((s) => s.status === 'running' || s.status === 'queued').length,
    terminal: isTerminal(execution.status),
  };
}
