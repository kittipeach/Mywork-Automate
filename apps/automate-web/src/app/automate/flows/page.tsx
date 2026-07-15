'use client';

import { useState } from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import {
  Plus,
  Search,
  FolderOpen,
  X,
  Pause,
  Play,
  Square,
  History,
  RotateCcw,
  ChevronDown,
  ChevronRight,
  Trash2,
  Share2,
  RotateCw,
  Users,
  User,
} from 'lucide-react';
import { TopBar } from '@/components/shell/TopBar';
import { Card } from '@/components/ui/Card';
import { Button } from '@/components/ui/Button';
import { StatusBadge, Badge } from '@/components/ui/Badge';
import { Modal } from '@/components/ui/Modal';
import { useToast } from '@/components/ui/Toast';
import type { FlowSummary } from '@/lib/mock/store';
import { useFlows, useVersions } from '@/api/hooks';
import {
  useCreateFlow,
  usePauseFlow,
  useResumeFlow,
  useStopFlow,
  useRollback,
  useDeleteFlow,
  useRestoreFlow,
  ApiError,
} from '@/api/mutations';
import {
  useGrants,
  useAddGrant,
  useDeleteGrant,
  type GrantSubjectType,
  type GrantAccess,
} from '@/api/grants';
import { useMe } from '@/api/auth';
import { availableLifecycleActions } from '@/lib/lifecycle';
import { fmtDateTime } from '@/api/format';

function lifecycleError(error: unknown): string | null {
  if (!error) return null;
  if (error instanceof ApiError && error.status === 409) {
    return 'That action isn’t allowed for this flow’s current status.';
  }
  return 'Something went wrong. Please try again.';
}

const ACTION_META = {
  pause: { label: 'Pause', Icon: Pause, variant: 'secondary' as const },
  resume: { label: 'Resume', Icon: Play, variant: 'secondary' as const },
  stop: { label: 'Stop', Icon: Square, variant: 'danger' as const },
};

function VersionsPanel({ flowId }: { flowId: string }) {
  const { data, isLoading, isError } = useVersions(flowId);
  const rollback = useRollback();
  const versions = data?.versions ?? [];

  const onRollback = (toVersion: number) => {
    const changeNote =
      typeof window !== 'undefined'
        ? window.prompt(`Change note for rolling back to v${toVersion}`, `Rollback to v${toVersion}`)
        : null;
    if (changeNote == null) return;
    rollback.mutate({ flowId, toVersion, changeNote });
  };

  const err = lifecycleError(rollback.error);

  return (
    <div className="bg-surface-sunken px-5 py-4">
      <div className="mb-2 flex items-center gap-2 text-xs font-medium uppercase tracking-wide text-ink-subtle">
        <History className="h-3.5 w-3.5" aria-hidden /> Version history
      </div>
      {isLoading && <p className="text-sm text-ink-muted">Loading versions…</p>}
      {isError && <p className="text-sm text-ink-muted">Couldn’t load versions.</p>}
      {!isLoading && !isError && versions.length === 0 && (
        <p className="text-sm text-ink-muted">No published versions yet.</p>
      )}
      {versions.length > 0 && (
        <ul className="divide-y divide-border rounded-md border border-border bg-surface">
          {versions.map((v) => (
            <li key={v.versionNo} className="flex items-center justify-between gap-4 px-4 py-2.5 text-sm">
              <div className="min-w-0">
                <span className="font-medium text-ink">v{v.versionNo}</span>{' '}
                <span className="text-ink-muted">{v.changeNote}</span>
                <div className="text-xs text-ink-subtle">
                  {v.publishedBy} · {fmtDateTime(v.publishedAt)}
                </div>
              </div>
              <Button
                size="sm"
                variant="ghost"
                onClick={() => onRollback(v.versionNo)}
                disabled={rollback.isPending}
                aria-label={`Roll back to v${v.versionNo}`}
              >
                <RotateCcw className="h-3.5 w-3.5" aria-hidden /> Rollback
              </Button>
            </li>
          ))}
        </ul>
      )}
      {err && <p className="mt-2 text-xs text-danger">{err}</p>}
    </div>
  );
}

const SUBJECT_TYPES: GrantSubjectType[] = ['role', 'user'];
const ACCESS_LEVELS: GrantAccess[] = ['viewer', 'editor', 'owner'];

/** Share dialog: list a flow's grants + add/remove access (E2-S3). */
function ShareDialog({ flow, onClose }: { flow: FlowSummary; onClose: () => void }) {
  const { toast } = useToast();
  const { data, isLoading, isError } = useGrants(flow.id);
  const addGrant = useAddGrant(flow.id);
  const deleteGrant = useDeleteGrant(flow.id);
  const grants = data?.grants ?? [];

  const [subjectType, setSubjectType] = useState<GrantSubjectType>('role');
  const [subjectId, setSubjectId] = useState('');
  const [access, setAccess] = useState<GrantAccess>('viewer');

  const submit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!subjectId.trim() || addGrant.isPending) return;
    addGrant.mutate(
      { subjectType, subjectId: subjectId.trim(), access },
      {
        onSuccess: () => {
          setSubjectId('');
          toast('success', 'Access granted');
        },
        onError: () => toast('error', 'Could not add grant'),
      },
    );
  };

  const remove = (grantId: string) =>
    deleteGrant.mutate(grantId, {
      onSuccess: () => toast('success', 'Access revoked'),
      onError: () => toast('error', 'Could not revoke grant'),
    });

  return (
    <Modal open onClose={onClose} title={`Share “${flow.name}”`}>
      <div className="space-y-4">
        {isLoading && <p className="text-sm text-ink-muted">Loading grants…</p>}
        {isError && <p className="text-sm text-ink-muted">Couldn’t load grants.</p>}
        {!isLoading && !isError && grants.length === 0 && (
          <p className="text-sm text-ink-muted">No one else has access yet.</p>
        )}
        {grants.length > 0 && (
          <ul className="divide-y divide-border rounded-md border border-border">
            {grants.map((g) => (
              <li key={g.id} className="flex items-center justify-between gap-3 px-3 py-2 text-sm">
                <span className="inline-flex items-center gap-1.5">
                  {g.subjectType === 'role' ? (
                    <Users className="h-3.5 w-3.5 text-ink-subtle" aria-hidden />
                  ) : (
                    <User className="h-3.5 w-3.5 text-ink-subtle" aria-hidden />
                  )}
                  <span className="font-medium text-ink">{g.subjectId}</span>
                  <Badge className="capitalize">{g.access}</Badge>
                </span>
                <Button
                  size="sm"
                  variant="ghost"
                  onClick={() => remove(g.id)}
                  disabled={deleteGrant.isPending}
                  aria-label={`Revoke ${g.subjectId}`}
                >
                  <X className="h-3.5 w-3.5" aria-hidden />
                </Button>
              </li>
            ))}
          </ul>
        )}

        <form onSubmit={submit} className="flex flex-wrap items-end gap-2 border-t border-border pt-4">
          <div>
            <label htmlFor="grant-subject-type" className="mb-1 block text-xs font-medium text-ink-muted">
              Type
            </label>
            <select
              id="grant-subject-type"
              value={subjectType}
              onChange={(e) => setSubjectType(e.target.value as GrantSubjectType)}
              className="h-9 rounded-md border border-border bg-surface px-2 text-sm capitalize outline-none focus:ring-2 focus:ring-brand"
            >
              {SUBJECT_TYPES.map((t) => (
                <option key={t} value={t}>{t}</option>
              ))}
            </select>
          </div>
          <div className="flex-1 min-w-[8rem]">
            <label htmlFor="grant-subject-id" className="mb-1 block text-xs font-medium text-ink-muted">
              {subjectType === 'role' ? 'Role' : 'User'}
            </label>
            <input
              id="grant-subject-id"
              value={subjectId}
              onChange={(e) => setSubjectId(e.target.value)}
              placeholder={subjectType === 'role' ? 'e.g. operator' : 'e.g. jane@corp'}
              className="h-9 w-full rounded-md border border-border bg-surface px-3 text-sm outline-none focus:ring-2 focus:ring-brand"
            />
          </div>
          <div>
            <label htmlFor="grant-access" className="mb-1 block text-xs font-medium text-ink-muted">
              Access
            </label>
            <select
              id="grant-access"
              value={access}
              onChange={(e) => setAccess(e.target.value as GrantAccess)}
              className="h-9 rounded-md border border-border bg-surface px-2 text-sm capitalize outline-none focus:ring-2 focus:ring-brand"
            >
              {ACCESS_LEVELS.map((a) => (
                <option key={a} value={a}>{a}</option>
              ))}
            </select>
          </div>
          <Button type="submit" size="sm" disabled={!subjectId.trim() || addGrant.isPending}>
            Add
          </Button>
        </form>
      </div>
    </Modal>
  );
}

function FlowRow({ flow, isAdmin }: { flow: FlowSummary; isAdmin: boolean }) {
  const [expanded, setExpanded] = useState(false);
  const [sharing, setSharing] = useState(false);
  const { toast } = useToast();
  const pause = usePauseFlow();
  const resume = useResumeFlow();
  const stop = useStopFlow();
  const del = useDeleteFlow();
  const restore = useRestoreFlow();

  const runners = { pause, resume, stop };
  const actions = availableLifecycleActions(flow.status);
  const deleted = !!flow.deletedAt;
  const pending = pause.isPending || resume.isPending || stop.isPending;
  const err =
    lifecycleError(pause.error) ?? lifecycleError(resume.error) ?? lifecycleError(stop.error);

  const onDelete = () => {
    if (typeof window !== 'undefined' && !window.confirm(`Delete “${flow.name}”?`)) return;
    del.mutate(flow.id, {
      onSuccess: () => toast('success', 'Flow deleted'),
      onError: () => toast('error', 'Could not delete flow'),
    });
  };

  const onRestore = () =>
    restore.mutate(flow.id, {
      onSuccess: () => toast('success', 'Flow restored'),
      onError: () => toast('error', 'Could not restore flow'),
    });

  return (
    <>
      <tr className={`group hover:bg-surface-sunken ${deleted ? 'opacity-60' : ''}`}>
        <td className="px-5 py-3">
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={() => setExpanded((e) => !e)}
              aria-label={expanded ? `Hide versions of ${flow.name}` : `Show versions of ${flow.name}`}
              aria-expanded={expanded}
              className="text-ink-subtle hover:text-ink"
            >
              {expanded ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
            </button>
            <Link href={`/automate/flows/${flow.id}`} className="font-medium text-ink group-hover:text-brand">
              {flow.name}
            </Link>
            {deleted && <Badge className="bg-danger/10 text-danger">deleted</Badge>}
          </div>
        </td>
        <td className="px-5 py-3">
          <span className="inline-flex items-center gap-1.5 text-ink-muted">
            <FolderOpen className="h-3.5 w-3.5" /> {flow.folder}
          </span>
        </td>
        <td className="px-5 py-3"><StatusBadge status={flow.status} /></td>
        <td className="px-5 py-3 text-ink-muted">{flow.version ? `v${flow.version}` : '—'}</td>
        <td className="px-5 py-3">
          {flow.lastRun ? (
            <StatusBadge status={flow.lastRun.status} />
          ) : (
            <span className="text-ink-subtle">never</span>
          )}
        </td>
        <td className="px-5 py-3">
          <div className="flex items-center justify-end gap-1.5">
            {deleted ? (
              isAdmin && (
                <Button
                  size="sm"
                  variant="secondary"
                  disabled={restore.isPending}
                  onClick={onRestore}
                  aria-label={`Restore ${flow.name}`}
                >
                  <RotateCw className="h-3.5 w-3.5" aria-hidden /> Restore
                </Button>
              )
            ) : (
              <>
                {actions.map((a) => {
                  const { label, Icon, variant } = ACTION_META[a];
                  return (
                    <Button
                      key={a}
                      size="sm"
                      variant={variant}
                      disabled={pending}
                      onClick={() => runners[a].mutate(flow.id)}
                      aria-label={`${label} ${flow.name}`}
                    >
                      <Icon className="h-3.5 w-3.5" aria-hidden /> {label}
                    </Button>
                  );
                })}
                <Button
                  size="sm"
                  variant="ghost"
                  onClick={() => setSharing(true)}
                  aria-label={`Share ${flow.name}`}
                >
                  <Share2 className="h-3.5 w-3.5" aria-hidden />
                </Button>
                <Button
                  size="sm"
                  variant="ghost"
                  disabled={del.isPending}
                  onClick={onDelete}
                  aria-label={`Delete ${flow.name}`}
                >
                  <Trash2 className="h-3.5 w-3.5" aria-hidden />
                </Button>
              </>
            )}
          </div>
          {err && <p className="mt-1 text-right text-xs text-danger">{err}</p>}
        </td>
      </tr>
      {expanded && (
        <tr>
          <td colSpan={6} className="p-0">
            <VersionsPanel flowId={flow.id} />
          </td>
        </tr>
      )}
      {sharing && <ShareDialogRow flow={flow} onClose={() => setSharing(false)} />}
    </>
  );
}

/** Renders the Share modal in a table-safe wrapper row (modal is fixed-position). */
function ShareDialogRow({ flow, onClose }: { flow: FlowSummary; onClose: () => void }) {
  return (
    <tr>
      <td colSpan={6} className="p-0">
        <ShareDialog flow={flow} onClose={onClose} />
      </td>
    </tr>
  );
}

export default function FlowsPage() {
  const router = useRouter();
  const [q, setQ] = useState('');
  const [status, setStatus] = useState('');
  const [showDeleted, setShowDeleted] = useState(false);
  const [creating, setCreating] = useState(false);
  const [name, setName] = useState('');
  const [folder, setFolder] = useState('HR Ops');
  const me = useMe();
  const isAdmin = me.data?.role === 'admin';
  // Only admins may include soft-deleted flows; if the toggle is somehow on for
  // a non-admin (e.g. role changed), fall back to the live-only view.
  const includeDeleted = isAdmin && showDeleted;
  const { data, isLoading } = useFlows({ q, status, includeDeleted });
  const createFlow = useCreateFlow();
  const flows = data?.flows ?? [];

  const submitNewFlow = (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim() || createFlow.isPending) return;
    createFlow.mutate(
      { name: name.trim(), folder: folder.trim() || 'General' },
      {
        onSuccess: (flow) => {
          setCreating(false);
          setName('');
          router.push(`/automate/flows/${flow.id}`);
        },
      },
    );
  };

  return (
    <>
      <TopBar
        title="Flows"
        actions={
          <Button size="sm" onClick={() => setCreating(true)}>
            <Plus className="h-4 w-4" /> New flow
          </Button>
        }
      />
      <main className="flex-1 overflow-auto p-6">
        {creating && (
          <Card className="mb-4 p-4">
            <form onSubmit={submitNewFlow} className="flex flex-wrap items-end gap-3">
              <div className="flex-1 min-w-[12rem]">
                <label htmlFor="new-flow-name" className="mb-1 block text-xs font-medium text-ink-muted">
                  Flow name
                </label>
                <input
                  id="new-flow-name"
                  autoFocus
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="e.g. Monthly Tax Export"
                  className="h-9 w-full rounded-md border border-border bg-surface px-3 text-sm outline-none focus:ring-2 focus:ring-brand"
                />
              </div>
              <div className="min-w-[10rem]">
                <label htmlFor="new-flow-folder" className="mb-1 block text-xs font-medium text-ink-muted">
                  Folder
                </label>
                <input
                  id="new-flow-folder"
                  value={folder}
                  onChange={(e) => setFolder(e.target.value)}
                  placeholder="Folder"
                  className="h-9 w-full rounded-md border border-border bg-surface px-3 text-sm outline-none focus:ring-2 focus:ring-brand"
                />
              </div>
              <Button type="submit" size="sm" disabled={!name.trim() || createFlow.isPending}>
                {createFlow.isPending ? 'Creating…' : 'Create'}
              </Button>
              <Button
                type="button"
                size="sm"
                variant="ghost"
                onClick={() => setCreating(false)}
                aria-label="Cancel new flow"
              >
                <X className="h-4 w-4" />
              </Button>
              {createFlow.isError && (
                <span className="w-full text-xs text-danger">
                  Couldn’t create the flow. Please try again.
                </span>
              )}
            </form>
          </Card>
        )}
        <div className="mb-4 flex items-center gap-3">
          <div className="relative flex-1 max-w-sm">
            <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-ink-subtle" />
            <input
              value={q}
              onChange={(e) => setQ(e.target.value)}
              placeholder="Search flows…"
              aria-label="Search flows"
              className="h-9 w-full rounded-md border border-border bg-surface pl-9 pr-3 text-sm outline-none focus:ring-2 focus:ring-brand"
            />
          </div>
          <select
            value={status}
            onChange={(e) => setStatus(e.target.value)}
            aria-label="Filter by status"
            className="h-9 rounded-md border border-border bg-surface px-3 text-sm outline-none focus:ring-2 focus:ring-brand"
          >
            <option value="">All statuses</option>
            <option value="draft">Draft</option>
            <option value="published">Published</option>
            <option value="paused">Paused</option>
            <option value="stopped">Stopped</option>
          </select>
          {isAdmin && (
            <label className="inline-flex items-center gap-2 text-sm text-ink-muted">
              <input
                type="checkbox"
                checked={showDeleted}
                onChange={(e) => setShowDeleted(e.target.checked)}
                aria-label="Show deleted flows"
                className="h-4 w-4 rounded border-border text-brand focus:ring-2 focus:ring-brand"
              />
              Show deleted
            </label>
          )}
        </div>

        <Card className="overflow-hidden">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border bg-surface-sunken text-left text-xs uppercase tracking-wide text-ink-subtle">
                <th className="px-5 py-3 font-medium">Flow</th>
                <th className="px-5 py-3 font-medium">Folder</th>
                <th className="px-5 py-3 font-medium">Status</th>
                <th className="px-5 py-3 font-medium">Version</th>
                <th className="px-5 py-3 font-medium">Last run</th>
                <th className="px-5 py-3 text-right font-medium">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {isLoading && (
                <tr><td colSpan={6} className="px-5 py-8 text-center text-ink-muted">Loading…</td></tr>
              )}
              {!isLoading && flows.length === 0 && (
                <tr><td colSpan={6} className="px-5 py-8 text-center text-ink-muted">No flows match.</td></tr>
              )}
              {flows.map((f) => (
                <FlowRow key={f.id} flow={f} isAdmin={isAdmin} />
              ))}
            </tbody>
          </table>
        </Card>
      </main>
    </>
  );
}
