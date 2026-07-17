'use client';

import { useMutation, useQueryClient } from '@tanstack/react-query';
import type { FlowSummary } from '@/lib/mock/store';
import { authHeader } from '@/lib/token';
import { BASE } from './hooks';

/**
 * Small POST helper mirroring getJSON in hooks.ts. Sends `body` as JSON (default
 * `{}`) and parses the JSON response. 202/201/200 all parse the body; callers
 * type the result. Throws on any non-2xx so TanStack Query surfaces isError.
 *
 * The thrown error carries the HTTP `status` so callers can distinguish, e.g.,
 * 409 invalid-transition or 423 locked from a generic failure.
 */
export class ApiError extends Error {
  constructor(
    public readonly status: number,
    public readonly path: string,
  ) {
    super(`${path} → ${status}`);
    this.name = 'ApiError';
  }
}

export async function poster<T>(path: string, body: unknown = {}): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    method: 'POST',
    // authHeader() attaches the bearer token when present (spreads to nothing
    // otherwise), so every mutation is authenticated without per-call plumbing.
    headers: { 'content-type': 'application/json', ...authHeader() },
    body: JSON.stringify(body),
  });
  if (!res.ok) throw new ApiError(res.status, path);
  return res.json() as Promise<T>;
}

/**
 * Generic verbed request helper for the mutations poster doesn't cover (PUT for
 * updates, DELETE for removals). Same auth + error contract as `poster`. A 204
 * No Content (typical for DELETE) has no body, so it resolves to `undefined`
 * rather than trying to parse empty JSON.
 */
export async function request<T>(
  method: 'PUT' | 'DELETE',
  path: string,
  body?: unknown,
): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    method,
    headers: { 'content-type': 'application/json', ...authHeader() },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (!res.ok) throw new ApiError(res.status, path);
  if (res.status === 204) return undefined as T;
  return res.json() as Promise<T>;
}

export type RunFlowResult = { executionId: string };
export type CreateFlowInput = { name: string; folder: string };

/** POST /flows/{id}/run → 202 { executionId }. Invalidates the executions list. */
export function useRunFlow() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (flowId: string) => poster<RunFlowResult>(`/flows/${flowId}/run`, {}),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['executions'] });
    },
  });
}

/** POST /flows (body {name, folder}) → 201 FlowSummary. Invalidates the flows list. */
export function useCreateFlow() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateFlowInput) => poster<FlowSummary>('/flows', input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['flows'] });
    },
  });
}

/**
 * POST /flows/{id}/publish (body {changeNote}) → 200 FlowSummary. The API now
 * requires a change note per publish (it becomes the version's changeNote), so
 * callers pass both the flow id and the note. Invalidates flows + that flow's
 * version history.
 */
export function usePublishFlow() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ flowId, changeNote }: { flowId: string; changeNote: string }) =>
      poster<FlowSummary>(`/flows/${flowId}/publish`, { changeNote }),
    onSuccess: (_data, { flowId }) => {
      qc.invalidateQueries({ queryKey: ['flows'] });
      qc.invalidateQueries({ queryKey: ['versions', flowId] });
    },
  });
}

export type LifecycleAction = 'pause' | 'resume' | 'stop';
export type LifecycleResult = { status: FlowSummary['status'] };

/**
 * Shared factory for the pause/resume/stop lifecycle mutations. Each POSTs
 * /flows/{id}/{action} → 200 {status} (409 on an invalid transition, surfaced as
 * an ApiError with status 409) and invalidates the flows list on success.
 */
function useLifecycleMutation(action: LifecycleAction) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (flowId: string) => poster<LifecycleResult>(`/flows/${flowId}/${action}`, {}),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['flows'] });
    },
  });
}

/** POST /flows/{id}/pause → 200 {status}. published → paused. */
export function usePauseFlow() {
  return useLifecycleMutation('pause');
}

/** POST /flows/{id}/resume → 200 {status}. paused → published. */
export function useResumeFlow() {
  return useLifecycleMutation('resume');
}

/** POST /flows/{id}/stop → 200 {status}. published|paused → stopped. */
export function useStopFlow() {
  return useLifecycleMutation('stop');
}

/**
 * POST /flows/{id}/rollback (body {toVersion, changeNote}) → 200 FlowSummary.
 * Restores an earlier published version as a new version. Invalidates the flows
 * list and that flow's version history.
 */
export function useRollback() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      flowId,
      toVersion,
      changeNote,
    }: {
      flowId: string;
      toVersion: number;
      changeNote: string;
    }) => poster<FlowSummary>(`/flows/${flowId}/rollback`, { toVersion, changeNote }),
    onSuccess: (_data, { flowId }) => {
      qc.invalidateQueries({ queryKey: ['flows'] });
      qc.invalidateQueries({ queryKey: ['versions', flowId] });
    },
  });
}

/**
 * DELETE /flows/{id} → 204 (soft delete; E3-S6). No request body; `request`
 * resolves to undefined on the 204. Invalidates the flows list so the row drops
 * out of the default (live-only) view and reappears greyed under "Show deleted".
 */
export function useDeleteFlow() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (flowId: string) => request<void>('DELETE', `/flows/${flowId}`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['flows'] });
    },
  });
}

/**
 * POST /flows/{id}/restore → 200 FlowSummary (admin only; E3-S6). Un-deletes a
 * soft-deleted flow. Invalidates the flows list so it moves back into the live
 * view.
 */
export function useRestoreFlow() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (flowId: string) => poster<FlowSummary>(`/flows/${flowId}/restore`, {}),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['flows'] });
    },
  });
}
