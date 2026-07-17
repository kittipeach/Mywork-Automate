'use client';

// Flow validation (E3-S4). POST /flows/{id}/validate returns a structured
// verdict the editor renders in a results panel. Kept beside the other API hooks
// so the whole data layer stays in src/api.
import { useMutation } from '@tanstack/react-query';
import type { RunStatus } from '@/components/ui/Badge';
import { poster } from './mutations';

export type IssueSeverity = 'error' | 'warning';

export type ValidationIssue = {
  /** node the issue is attached to (absent for flow-level issues). */
  nodeId?: string;
  severity: IssueSeverity;
  message: string;
};

export type ValidateResult = {
  valid: boolean;
  issues: ValidationIssue[];
};

/**
 * POST /flows/{id}/validate → {valid, issues}. Server-side validation of the
 * whole flow (missing config, unreachable nodes, bad expressions, etc). No cache
 * invalidation — the result is transient UI shown in the validate panel.
 */
export function useValidateFlow() {
  return useMutation({
    mutationFn: (flowId: string) => poster<ValidateResult>(`/flows/${flowId}/validate`, {}),
  });
}

/**
 * Map a step's run status to the ring color class the canvas node wears during a
 * test run. Only in-flight/terminal step statuses produce a ring; anything else
 * (idle/unknown) yields no ring so the node keeps its normal border.
 */
export function statusToRing(status?: RunStatus | string): string | null {
  switch (status) {
    case 'running':
    case 'queued':
      return 'ring-2 ring-info animate-pulse';
    case 'success':
      return 'ring-2 ring-success';
    case 'failed':
      return 'ring-2 ring-danger';
    case 'cancelled':
    case 'skipped':
      return 'ring-2 ring-ink-subtle';
    default:
      return null;
  }
}
