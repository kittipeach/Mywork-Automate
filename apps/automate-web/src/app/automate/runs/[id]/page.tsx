'use client';

// Run detail drill-down (E7). Fetches a single execution and renders a header
// summary plus a per-step timeline table: node name, type, status, duration,
// input→output row counts, and any error (red). Loading + not-found states.
import { useParams } from 'next/navigation';
import Link from 'next/link';
import { ArrowLeft, AlertTriangle } from 'lucide-react';
import { TopBar } from '@/components/shell/TopBar';
import { Card } from '@/components/ui/Card';
import { StatusBadge } from '@/components/ui/Badge';
import { useExecution } from '@/api/hooks';
import { fmtDuration, fmtDateTime, stepDotColor } from '@/api/format';

export default function RunDetailPage() {
  const params = useParams<{ id: string }>();
  const id = params?.id ?? '';
  const { data: run, isLoading, isError } = useExecution(id);

  return (
    <>
      <TopBar title="Run detail" />
      <main className="flex-1 overflow-auto p-6">
        <Link
          href="/automate/runs"
          className="mb-4 inline-flex items-center gap-1.5 text-sm text-ink-muted hover:text-ink"
        >
          <ArrowLeft className="h-4 w-4" /> Back to runs
        </Link>

        {isLoading && (
          <Card className="p-8 text-center text-sm text-ink-muted">Loading run…</Card>
        )}

        {!isLoading && (isError || !run) && (
          <Card className="p-8 text-center">
            <div className="text-sm font-medium text-ink">Run not found</div>
            <p className="mt-1 text-sm text-ink-muted">
              No execution with id <span className="font-mono">{id}</span>.
            </p>
          </Card>
        )}

        {run && (
          <>
            <Card className="mb-6 p-5">
              <div className="flex items-start justify-between gap-4">
                <div>
                  <h2 className="text-lg font-semibold text-ink">{run.flowName}</h2>
                  <div className="mt-1 text-xs text-ink-subtle">
                    {run.id} · started {fmtDateTime(run.startedAt)}
                  </div>
                </div>
                <StatusBadge status={run.status} />
              </div>
              <dl className="mt-4 grid grid-cols-2 gap-x-6 gap-y-3 text-sm sm:grid-cols-4">
                <Meta label="Trigger" value={<span className="capitalize">{run.trigger}</span>} />
                <Meta label="Version" value={`v${run.version}`} />
                <Meta label="Steps" value={String(run.steps.length)} />
                <Meta label="Total duration" value={fmtDuration(run.durationMs)} />
              </dl>
            </Card>

            <Card className="overflow-hidden">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-border bg-surface-sunken text-left text-xs uppercase tracking-wide text-ink-subtle">
                    <th className="px-5 py-3 font-medium">Step</th>
                    <th className="px-5 py-3 font-medium">Type</th>
                    <th className="px-5 py-3 font-medium">Status</th>
                    <th className="px-5 py-3 font-medium">Duration</th>
                    <th className="px-5 py-3 font-medium">In → Out</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-border">
                  {run.steps.map((s, i) => (
                    <tr key={s.nodeId} className="align-top hover:bg-surface-sunken">
                      <td className="px-5 py-3">
                        <div className="flex items-center gap-2.5">
                          <span
                            aria-hidden
                            className={`h-2 w-2 shrink-0 rounded-full ${stepDotColor(s.status)}`}
                          />
                          <div>
                            <div className="font-medium text-ink">
                              <span className="text-ink-subtle">{i + 1}.</span> {s.nodeName}
                            </div>
                            {s.error && (
                              <div className="mt-1 flex items-start gap-1.5 text-xs text-danger">
                                <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                                <span>{s.error}</span>
                              </div>
                            )}
                          </div>
                        </div>
                      </td>
                      <td className="px-5 py-3">
                        <span className="font-mono text-xs text-ink-muted">{s.nodeType}</span>
                      </td>
                      <td className="px-5 py-3"><StatusBadge status={s.status} /></td>
                      <td className="px-5 py-3 text-ink-muted">{fmtDuration(s.durationMs)}</td>
                      <td className="px-5 py-3 text-ink-muted tabular-nums">
                        {s.inputCount.toLocaleString()} → {s.outputCount.toLocaleString()}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </Card>
          </>
        )}
      </main>
    </>
  );
}

function Meta({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div>
      <dt className="text-xs uppercase tracking-wide text-ink-subtle">{label}</dt>
      <dd className="mt-0.5 font-medium text-ink">{value}</dd>
    </div>
  );
}
