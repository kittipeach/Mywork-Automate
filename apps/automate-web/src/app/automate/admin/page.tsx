'use client';

import { useState } from 'react';
import { useRouter } from 'next/navigation';
import { useQueryClient } from '@tanstack/react-query';
import { ShieldCheck, UserCircle, LogOut } from 'lucide-react';
import { TopBar } from '@/components/shell/TopBar';
import { Card } from '@/components/ui/Card';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import { useAuditLogs } from '@/api/audit';
import { useMe, logout } from '@/api/auth';
import { fmtDateTime } from '@/api/format';

const ACTIONS = ['', 'auth.login', 'flow.create', 'flow.publish', 'execution.manual_run', 'connection.create'];

function MeCard() {
  const router = useRouter();
  const qc = useQueryClient();
  const { data, isLoading, isError } = useMe();

  const onLogout = () => {
    logout(qc);
    router.push('/login');
  };

  return (
    <Card className="mb-6 p-5">
      <div className="flex items-start justify-between gap-4">
        <div className="flex items-start gap-3">
          <UserCircle className="h-8 w-8 text-ink-subtle" aria-hidden />
          <div>
            <div className="flex items-center gap-2">
              <h2 className="text-sm font-semibold text-ink">Signed in</h2>
              {isLoading && <span className="text-xs text-ink-subtle">loading…</span>}
              {!isLoading && (isError || !data) && (
                <span className="text-xs text-ink-subtle">not signed in</span>
              )}
              {data && (
                <Badge className="bg-brand-100 capitalize text-brand-700">{data.role}</Badge>
              )}
            </div>
            {data && data.permissions.length > 0 && (
              <div className="mt-2 flex flex-wrap gap-1.5">
                {data.permissions.map((p) => (
                  <Badge key={p}>{p}</Badge>
                ))}
              </div>
            )}
            {data && data.permissions.length === 0 && (
              <p className="mt-1 text-xs text-ink-subtle">No permissions granted.</p>
            )}
          </div>
        </div>
        <Button size="sm" variant="secondary" onClick={onLogout}>
          <LogOut className="h-4 w-4" aria-hidden /> Log out
        </Button>
      </div>
    </Card>
  );
}

export default function AdminPage() {
  const [action, setAction] = useState('');
  const { data, isLoading, isError } = useAuditLogs({ action, limit: 200 });
  const logs = data?.auditLogs ?? [];

  return (
    <>
      <TopBar title="Admin & RBAC" />
      <main className="flex-1 overflow-auto p-6">
        <MeCard />
        <div className="mb-4 flex items-center gap-2">
          <ShieldCheck className="h-4 w-4 text-ink-subtle" />
          <h2 className="text-sm font-semibold text-ink">Audit log</h2>
          <span className="text-xs text-ink-subtle">append-only · admin only</span>
          <select
            value={action}
            onChange={(e) => setAction(e.target.value)}
            aria-label="Filter by action"
            className="ml-auto h-8 rounded-md border border-border bg-surface px-2 text-sm outline-none focus:ring-2 focus:ring-brand"
          >
            {ACTIONS.map((a) => (
              <option key={a || 'all'} value={a}>{a || 'all actions'}</option>
            ))}
          </select>
        </div>

        {isError ? (
          <Card className="p-6 text-sm text-ink-muted">
            You need the <span className="font-medium text-ink">Admin</span> role to view the audit log.
          </Card>
        ) : (
          <Card className="overflow-hidden">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-border bg-surface-sunken text-left text-xs uppercase tracking-wide text-ink-subtle">
                  <th className="px-5 py-3 font-medium">Time</th>
                  <th className="px-5 py-3 font-medium">Action</th>
                  <th className="px-5 py-3 font-medium">Actor</th>
                  <th className="px-5 py-3 font-medium">Resource</th>
                  <th className="px-5 py-3 font-medium">IP</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border">
                {isLoading && (
                  <tr><td colSpan={5} className="px-5 py-8 text-center text-ink-muted">Loading…</td></tr>
                )}
                {!isLoading && logs.length === 0 && (
                  <tr><td colSpan={5} className="px-5 py-8 text-center text-ink-muted">No audit entries.</td></tr>
                )}
                {logs.map((e) => (
                  <tr key={e.id} className="hover:bg-surface-sunken">
                    <td className="px-5 py-3 text-ink-muted tabular-nums">{fmtDateTime(e.createdAt)}</td>
                    <td className="px-5 py-3"><Badge>{e.action}</Badge></td>
                    <td className="px-5 py-3 text-ink-muted">{e.userId || e.role}</td>
                    <td className="px-5 py-3 text-ink-muted">
                      {e.resourceType ? `${e.resourceType}${e.resourceId ? ` · ${e.resourceId}` : ''}` : '—'}
                    </td>
                    <td className="px-5 py-3 font-mono text-xs text-ink-subtle">{e.ip || '—'}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </Card>
        )}
      </main>
    </>
  );
}
