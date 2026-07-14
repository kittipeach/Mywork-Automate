'use client';

import { useState } from 'react';
import { TopBar } from '@/components/shell/TopBar';
import { Card } from '@/components/ui/Card';
import { StatusBadge, type RunStatus } from '@/components/ui/Badge';
import { useExecutions } from '@/api/hooks';

function fmtDuration(ms: number): string {
  if (ms === 0) return '—';
  if (ms < 1000) return `${ms}ms`;
  const s = Math.round(ms / 1000);
  return s < 60 ? `${s}s` : `${Math.floor(s / 60)}m ${s % 60}s`;
}

export default function RunsPage() {
  const [status, setStatus] = useState('');
  const { data } = useExecutions({ status });
  const runs = data?.executions ?? [];

  return (
    <>
      <TopBar title="Run History" />
      <main className="flex-1 overflow-auto p-6">
        <div className="mb-4 flex gap-2">
          {['', 'success', 'failed', 'running'].map((s) => (
            <button
              key={s || 'all'}
              onClick={() => setStatus(s)}
              className={`rounded-full px-3 py-1 text-sm capitalize ${status === s ? 'bg-brand text-brand-fg' : 'bg-surface text-ink-muted border border-border hover:bg-surface-sunken'}`}
            >
              {s || 'all'}
            </button>
          ))}
        </div>
        <Card className="overflow-hidden">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border bg-surface-sunken text-left text-xs uppercase tracking-wide text-ink-subtle">
                <th className="px-5 py-3 font-medium">Flow</th>
                <th className="px-5 py-3 font-medium">Trigger</th>
                <th className="px-5 py-3 font-medium">Status</th>
                <th className="px-5 py-3 font-medium">Steps</th>
                <th className="px-5 py-3 font-medium">Duration</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {runs.map((e) => (
                <tr key={e.id} className="hover:bg-surface-sunken">
                  <td className="px-5 py-3">
                    <div className="font-medium text-ink">{e.flowName}</div>
                    <div className="text-xs text-ink-subtle">{e.id} · v{e.version}</div>
                  </td>
                  <td className="px-5 py-3 capitalize text-ink-muted">{e.trigger}</td>
                  <td className="px-5 py-3"><StatusBadge status={e.status} /></td>
                  <td className="px-5 py-3">
                    <div className="flex items-center gap-1">
                      {e.steps.map((s) => (
                        <span
                          key={s.nodeId}
                          title={`${s.nodeName} · ${s.status}`}
                          className={`h-2 w-6 rounded-full ${dotColor(s.status)}`}
                        />
                      ))}
                    </div>
                  </td>
                  <td className="px-5 py-3 text-ink-muted">{fmtDuration(e.durationMs)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </Card>
      </main>
    </>
  );
}

function dotColor(s: RunStatus): string {
  if (s === 'success') return 'bg-success';
  if (s === 'failed') return 'bg-danger';
  if (s === 'running') return 'bg-info animate-pulse';
  return 'bg-border';
}
