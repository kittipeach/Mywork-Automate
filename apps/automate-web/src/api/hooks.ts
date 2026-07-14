'use client';

import { useQuery } from '@tanstack/react-query';
import type { NodeType } from '@/lib/nodeRegistry';
import type { FlowSummary, Execution, Connection } from '@/lib/mock/store';

// Points at the real Go control-plane API. NEXT_PUBLIC_API_BASE is set at build
// time (e.g. http://127.0.0.1:8090/api/automate/v1 in dev, the Kong gateway in
// prod). Falls back to the same-origin path for pure-frontend/mock dev.
const BASE = process.env.NEXT_PUBLIC_API_BASE ?? '/api/automate/v1';

async function getJSON<T>(path: string): Promise<T> {
  const res = await fetch(`${BASE}${path}`);
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

export function useFlows(params: { q?: string; status?: string; folder?: string } = {}) {
  const qs = new URLSearchParams(
    Object.entries(params).filter(([, v]) => v) as [string, string][],
  ).toString();
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
