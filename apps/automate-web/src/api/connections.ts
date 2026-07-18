'use client';

// Connection + DB-query hooks (E6). Create/update/delete/test connections,
// preview a SELECT against a connection, and introspect its schema for the
// visual builder. All go through the shared poster/request helpers (auth +
// ApiError contract) and invalidate the ['connections'] list on write.
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import type { Connection } from '@/lib/mock/store';
import { getJSON } from './hooks';
import { poster, request } from './mutations';

export type ConnectionType = Connection['type'];

/** Body for create/update. `id` is only present on the edit path. */
export type ConnectionInput = {
  name: string;
  type: ConnectionType;
  host: string;
  port?: number;
  database?: string;
  username?: string;
  sslMode?: string;
  /** Write-only (dev): saved to the secret store; never returned. */
  password?: string;
  /** Existing secret name (prod path). */
  secretRef?: string;
  allowedRoles?: string[];
};

/** POST /connections {name,type,host,allowedRoles?} → 201. Invalidates the list. */
export function useCreateConnection() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: ConnectionInput) => poster<Connection>('/connections', input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['connections'] });
    },
  });
}

/** PUT /connections/{id} {name,type,host,allowedRoles?} → 200. Invalidates the list. */
export function useUpdateConnection() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...input }: ConnectionInput & { id: string }) =>
      request<Connection>('PUT', `/connections/${id}`, input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['connections'] });
    },
  });
}

/** DELETE /connections/{id} → 204. Invalidates the list. */
export function useDeleteConnection() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => request<void>('DELETE', `/connections/${id}`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['connections'] });
    },
  });
}

/**
 * Live-probe result. The probe itself is a 200 even when the target is
 * unreachable: `status` carries the outcome (`ok` connected, `error` could not
 * connect / bad config, `unknown` testing not configured on the server) and
 * `message` a human reason. The password is never echoed in `message`.
 */
export type TestConnectionResult = { status: 'ok' | 'error' | 'unknown'; message?: string };

/**
 * POST /connections/{id}/test → {status, message}. Read-only probe; does not
 * invalidate. Network/HTTP errors still reject (ApiError); a reachable server
 * reporting an unreachable target resolves with status:"error".
 */
export function useTestConnection() {
  return useMutation({
    mutationFn: (id: string) => poster<TestConnectionResult>(`/connections/${id}/test`, {}),
  });
}

// ---- Query preview + schema (E6-S2 / E6-S4) -------------------------------

export type QueryPreviewInput = { connectionId: string; sql: string; maxRows?: number };
export type QueryPreviewMeta = { columns: string[]; rowCount: number; truncated: boolean };
export type QueryPreviewResult = {
  items: Record<string, unknown>[];
  meta: QueryPreviewMeta;
};

/**
 * POST /connections/{id}/query-preview {sql, maxRows?} → {items, meta}. Invalid
 * SQL is a 400 (invalid_sql), surfaced as an ApiError with status 400 so the
 * editor can show the message inline. Read-only: no cache invalidation.
 */
export function useQueryPreview() {
  return useMutation({
    mutationFn: ({ connectionId, sql, maxRows }: QueryPreviewInput) =>
      poster<QueryPreviewResult>(`/connections/${connectionId}/query-preview`, { sql, maxRows }),
  });
}

export type SchemaColumn = { name: string; type: string };
export type SchemaTable = { name: string; columns: SchemaColumn[] };
export type ConnectionSchema = { tables: SchemaTable[] };

/**
 * GET /connections/{id}/schema → {tables:[{name, columns:[{name,type}]}]}. Feeds
 * the visual builder's table/column pickers. Disabled until an id is given so it
 * only fires when a connection is selected.
 */
export function useConnectionSchema(id: string) {
  return useQuery({
    queryKey: ['connectionSchema', id],
    queryFn: () => getJSON<ConnectionSchema>(`/connections/${id}/schema`),
    enabled: !!id,
  });
}
