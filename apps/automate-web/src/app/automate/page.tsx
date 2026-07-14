'use client';

import Link from 'next/link';
import { CheckCircle2, XCircle, Workflow, Activity } from 'lucide-react';
import { TopBar } from '@/components/shell/TopBar';
import { Card, CardBody, CardHeader } from '@/components/ui/Card';
import { StatusBadge } from '@/components/ui/Badge';
import { useExecutions, useFlows } from '@/api/hooks';

function Stat({ icon: Icon, label, value, tone }: { icon: any; label: string; value: string; tone: string }) {
  return (
    <Card>
      <CardBody className="flex items-center gap-4">
        <div className={`grid h-11 w-11 place-items-center rounded-lg ${tone}`}>
          <Icon className="h-5 w-5" />
        </div>
        <div>
          <div className="text-2xl font-semibold text-ink">{value}</div>
          <div className="text-sm text-ink-muted">{label}</div>
        </div>
      </CardBody>
    </Card>
  );
}

export default function DashboardPage() {
  const { data: flowsData } = useFlows();
  const { data: exData } = useExecutions();
  const flows = flowsData?.flows ?? [];
  const executions = exData?.executions ?? [];
  const published = flows.filter((f) => f.status === 'published').length;
  const failures = executions.filter((e) => e.status === 'failed');
  const success = executions.filter((e) => e.status === 'success').length;
  const rate = executions.length ? Math.round((success / executions.length) * 100) : 100;

  return (
    <>
      <TopBar title="Dashboard" />
      <main className="flex-1 overflow-auto p-6">
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
          <Stat icon={Activity} label="Runs today" value={String(executions.length)} tone="bg-info/10 text-info" />
          <Stat icon={CheckCircle2} label="Success rate" value={`${rate}%`} tone="bg-success/10 text-success" />
          <Stat icon={Workflow} label="Published flows" value={String(published)} tone="bg-brand/10 text-brand" />
          <Stat icon={XCircle} label="Recent failures" value={String(failures.length)} tone="bg-danger/10 text-danger" />
        </div>

        <div className="mt-6 grid grid-cols-1 gap-4 lg:grid-cols-2">
          <Card>
            <CardHeader title="Recent runs" actions={<Link href="/automate/runs" className="text-sm text-brand hover:underline">View all</Link>} />
            <CardBody className="p-0">
              <ul className="divide-y divide-border">
                {executions.map((e) => (
                  <li key={e.id} className="flex items-center justify-between px-5 py-3">
                    <div>
                      <div className="text-sm font-medium text-ink">{e.flowName}</div>
                      <div className="text-xs text-ink-subtle">{e.trigger} · v{e.version}</div>
                    </div>
                    <StatusBadge status={e.status} />
                  </li>
                ))}
              </ul>
            </CardBody>
          </Card>

          <Card>
            <CardHeader title="Attention needed" subtitle="Failed runs to review" />
            <CardBody className="p-0">
              {failures.length === 0 ? (
                <p className="px-5 py-6 text-sm text-ink-muted">No failures. 🎉</p>
              ) : (
                <ul className="divide-y divide-border">
                  {failures.map((e) => (
                    <li key={e.id} className="px-5 py-3">
                      <div className="text-sm font-medium text-ink">{e.flowName}</div>
                      <div className="text-xs text-danger">{e.steps.find((s) => s.error)?.error ?? 'failed'}</div>
                    </li>
                  ))}
                </ul>
              )}
            </CardBody>
          </Card>
        </div>
      </main>
    </>
  );
}
