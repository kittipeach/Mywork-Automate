'use client';

// Flow-sharing / access-grant hooks (E2-S3). List the grants on a flow, add a
// grant (share with a role or a user at a given access level), and revoke one.
// All go through the shared getJSON/poster/request helpers (auth + ApiError
// contract) and invalidate the per-flow ['grants', flowId] list on write.
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { getJSON } from './hooks';
import { poster, request } from './mutations';

/** Whether a grant targets a role (e.g. "operator") or an individual user. */
export type GrantSubjectType = 'role' | 'user';
/** Access level a grant confers, ascending. */
export type GrantAccess = 'viewer' | 'editor' | 'owner';

export type Grant = {
  id: string;
  subjectType: GrantSubjectType;
  subjectId: string;
  access: GrantAccess;
};

/** Body for POST /flows/{id}/grants. */
export type AddGrantInput = {
  subjectType: GrantSubjectType;
  subjectId: string;
  access: GrantAccess;
};

/**
 * GET /flows/{id}/grants → {grants:[...]}. The share list for a flow. Disabled
 * until a flow id is given so it only fires when the Share dialog opens.
 */
export function useGrants(flowId: string) {
  return useQuery({
    queryKey: ['grants', flowId],
    queryFn: () => getJSON<{ grants: Grant[] }>(`/flows/${flowId}/grants`),
    enabled: !!flowId,
  });
}

/**
 * POST /flows/{id}/grants {subjectType, subjectId, access} → 201 Grant. Adds a
 * share. Invalidates that flow's grant list.
 */
export function useAddGrant(flowId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: AddGrantInput) => poster<Grant>(`/flows/${flowId}/grants`, input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['grants', flowId] });
    },
  });
}

/**
 * DELETE /flows/{id}/grants/{grantId} → 204. Revokes a share. Invalidates that
 * flow's grant list.
 */
export function useDeleteGrant(flowId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (grantId: string) =>
      request<void>('DELETE', `/flows/${flowId}/grants/${grantId}`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['grants', flowId] });
    },
  });
}
