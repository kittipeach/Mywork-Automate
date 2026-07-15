'use client';

import { useQuery } from '@tanstack/react-query';
import type { NodeType } from '@/lib/nodeRegistry';
import type { FlowSummary, Execution, Connection } from '@/lib/mock/store';
import { authHeader } from '@/lib/token';

// Points at the real Go control-plane API. NEXT_PUBLIC_API_BASE is set at build
// time (e.g. http://127.0.0.1:8090/api/automate/v1 in dev, the Kong gateway in
// prod). Falls back to the same-origin path for pure-frontend/mock dev.
export const BASE = process.env.NEXT_PUBLIC_API_BASE ?? '/api/automate/v1';

export async function getJSON<T>(path: string): Promise<T> {
  // authHeader() adds `Authorization: Bearer <token>` only when a token exists;
  // otherwise it spreads to nothing and the request goes out anonymously.
  const res = await fetch(`${BASE}${path}`, { headers: { ...authHeader() } });
  if (!res.ok) throw new Error(`${path} → ${res.status}`);
  return res.json() as Promise<T>;
}

export function useNodes() {
  return useQuery({
    queryKey: ['nodes'],
    queryFn: () => getJSON<{ nodes: NodeType[] }>('/nodes'),
    staleTime: Infinity,
  });
}

export function useFlows(
  params: { q?: string; status?: string; folder?: string; includeDeleted?: boolean } = {},
) {
  const { includeDeleted, ...rest } = params;
  const search = new URLSearchParams(
    Object.entries(rest).filter(([, v]) => v) as [string, string][],
  );
  // includeDeleted is a boolean flag: only emit it when truthy (a `false` would
  // otherwise be dropped by the falsy filter above, but we never want `=false`
  // on the wire — its absence means "live flows only").
  if (includeDeleted) search.set('includeDeleted', 'true');
  const qs = search.toString();
  return useQuery({
    queryKey: ['flows', params],
    queryFn: () => getJSON<{ flows: FlowSummary[] }>(`/flows${qs ? `?${qs}` : ''}`),
  });
}

export function useExecutions(params: { status?: string; flowId?: string } = {}) {
  const qs = new URLSearchParams(
    Object.entries(params).filter(([, v]) => v) as [string, string][],
  ).toString();
  return useQuery({
    queryKey: ['executions', params],
    queryFn: () => getJSON<{ executions: Execution[] }>(`/executions${qs ? `?${qs}` : ''}`),
  });
}

export function useConnections() {
  return useQuery({
    queryKey: ['connections'],
    queryFn: () => getJSON<{ connections: Connection[] }>('/connections'),
  });
}

/** Statuses that mean the run is still in flight (keep polling). */
export const RUNNING_STATUSES = ['queued', 'running'] as const;

/** True while an execution should keep being polled (non-terminal). */
export function isRunning(status?: string): boolean {
  return status === 'queued' || status === 'running';
}

/**
 * Single execution with its per-step drill-down. Disabled until an id is given.
 *
 * When `poll` is set, the query auto-refetches on `intervalMs` (default 1500ms)
 * *only while the run is still queued/running*; once it reaches a terminal state
 * `refetchInterval` returns false and polling stops. This drives the test-run
 * overlay in the flow editor (E3-S4).
 */
export function useExecution(id: string, opts: { poll?: boolean; intervalMs?: number } = {}) {
  const { poll = false, intervalMs = 1500 } = opts;
  return useQuery({
    queryKey: ['execution', id],
    queryFn: () => getJSON<Execution>(`/executions/${id}`),
    enabled: !!id,
    refetchInterval: (query) => {
      if (!poll) return false;
      const status = query.state.data?.status;
      // keep polling until the run reaches a terminal state
      return isRunning(status) ? intervalMs : false;
    },
  });
}

export type FlowVersion = {
  versionNo: number;
  changeNote: string;
  publishedBy: string;
  publishedAt: string;
};

/**
 * GET /flows/{id}/versions → {versions:[...]}. The published-version history for
 * a flow, used by the versions panel + rollback. Disabled until a flow id is
 * given so it only fires when a row is expanded / a panel opens.
 */
export function useVersions(flowId: string) {
  return useQuery({
    queryKey: ['versions', flowId],
    queryFn: () => getJSON<{ versions: FlowVersion[] }>(`/flows/${flowId}/versions`),
    enabled: !!flowId,
  });
}
