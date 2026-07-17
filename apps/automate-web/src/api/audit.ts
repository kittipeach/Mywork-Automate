'use client';

import { useQuery } from '@tanstack/react-query';
import { getJSON } from './hooks';

export type AuditEntry = {
  id: number;
  userId?: string;
  role: string;
  action: string;
  resourceType?: string;
  resourceId?: string;
  detail?: Record<string, unknown>;
  ip?: string;
  createdAt: string;
};

/** Audit log viewer (E2-S5). Admin-only server-side (403 for other roles). */
export function useAuditLogs(params: { action?: string; limit?: number } = {}) {
  const entries = Object.entries(params).filter(([, v]) => v !== undefined && v !== '');
  const qs = new URLSearchParams(entries.map(([k, v]) => [k, String(v)])).toString();
  return useQuery({
    queryKey: ['auditLogs', params],
    queryFn: () => getJSON<{ auditLogs: AuditEntry[] }>(`/audit-logs${qs ? `?${qs}` : ''}`),
  });
}
