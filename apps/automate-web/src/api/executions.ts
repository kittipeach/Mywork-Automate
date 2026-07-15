'use client';

// Execution control hooks (E5-S5): cancel a running/queued run, or retry any
// run to spawn a fresh execution. Both invalidate the executions list and the
// single-execution query so the runs page + detail refresh.
import { useMutation, useQueryClient } from '@tanstack/react-query';
import type { RunStatus } from '@/components/ui/Badge';
import { poster } from './mutations';

export type CancelResult = { status: 'cancelled' };
export type RetryResult = { executionId: string };

/**
 * POST /executions/{id}/cancel → {status:"cancelled"}. Terminal runs return 409
 * (surfaced as an ApiError with status 409 so the caller can toast "already
 * finished"). Invalidates the executions list + this execution's detail.
 */
export function useCancelExecution() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => poster<CancelResult>(`/executions/${id}/cancel`, {}),
    onSuccess: (_data, id) => {
      qc.invalidateQueries({ queryKey: ['executions'] });
      qc.invalidateQueries({ queryKey: ['execution', id] });
    },
  });
}

/**
 * POST /executions/{id}/retry → {executionId}. Starts a new run from the same
 * flow version; callers navigate to the returned id. Invalidates the executions
 * list + this execution's detail.
 */
export function useRetryExecution() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => poster<RetryResult>(`/executions/${id}/retry`, {}),
    onSuccess: (_data, id) => {
      qc.invalidateQueries({ queryKey: ['executions'] });
      qc.invalidateQueries({ queryKey: ['execution', id] });
    },
  });
}

/** A run can be cancelled only while it is still in flight. */
export function canCancel(status: RunStatus): boolean {
  return status === 'running' || status === 'queued';
}

/**
 * Re-run is offered only for runs that have reached a terminal state (there's
 * nothing to re-run while one is still going).
 */
export function canRetry(status: RunStatus): boolean {
  return status === 'success' || status === 'failed' || status === 'cancelled';
}
