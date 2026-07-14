'use client';

import { useState } from 'react';
import Link from 'next/link';
import { Plus, Search, FolderOpen } from 'lucide-react';
import { TopBar } from '@/components/shell/TopBar';
import { Card } from '@/components/ui/Card';
import { Button } from '@/components/ui/Button';
import { StatusBadge } from '@/components/ui/Badge';
import { useFlows } from '@/api/hooks';

export default function FlowsPage() {
  const [q, setQ] = useState('');
  const [status, setStatus] = useState('');
  const { data, isLoading } = useFlows({ q, status });
  const flows = data?.flows ?? [];

  return (
    <>
      <TopBar
        title="Flows"
        actions={
          <Button size="sm">
            <Plus className="h-4 w-4" /> New flow
          </Button>
        }
      />
      <main className="flex-1 overflow-auto p-6">
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
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {isLoading && (
                <tr><td colSpan={5} className="px-5 py-8 text-center text-ink-muted">Loading…</td></tr>
              )}
              {!isLoading && flows.length === 0 && (
                <tr><td colSpan={5} className="px-5 py-8 text-center text-ink-muted">No flows match.</td></tr>
              )}
              {flows.map((f) => (
                <tr key={f.id} className="group hover:bg-surface-sunken">
                  <td className="px-5 py-3">
                    <Link href={`/automate/flows/${f.id}`} className="font-medium text-ink group-hover:text-brand">
                      {f.name}
                    </Link>
                  </td>
                  <td className="px-5 py-3">
                    <span className="inline-flex items-center gap-1.5 text-ink-muted">
                      <FolderOpen className="h-3.5 w-3.5" /> {f.folder}
                    </span>
                  </td>
                  <td className="px-5 py-3"><StatusBadge status={f.status} /></td>
                  <td className="px-5 py-3 text-ink-muted">{f.version ? `v${f.version}` : '—'}</td>
                  <td className="px-5 py-3">
                    {f.lastRun ? <StatusBadge status={f.lastRun.status} /> : <span className="text-ink-subtle">never</span>}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </Card>
      </main>
    </>
  );
}
