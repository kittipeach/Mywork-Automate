'use client';

import { useMutation, useQueryClient } from '@tanstack/react-query';
import type { FlowSummary } from '@/lib/mock/store';
import { BASE } from './hooks';

/**
 * Small POST helper mirroring getJSON in hooks.ts. Sends `body` as JSON (default
 * `{}`) and parses the JSON response. 202/201/200 all parse the body; callers
 * type the result. Throws on any non-2xx so TanStack Query surfaces isError.
 */
export async function poster<T>(path: string, body: unknown = {}): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify(body),
  });
  if (!res.ok) throw new Error(`${path} → ${res.status}`);
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

/** POST /flows/{id}/publish → 200 FlowSummary. Invalidates the flows list. */
export function usePublishFlow() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (flowId: string) => poster<FlowSummary>(`/flows/${flowId}/publish`, {}),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['flows'] });
    },
  });
}
