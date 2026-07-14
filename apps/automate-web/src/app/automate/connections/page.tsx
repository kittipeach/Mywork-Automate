'use client';

import { Database, Server, Mail, Send, Plus, ShieldCheck } from 'lucide-react';
import { TopBar } from '@/components/shell/TopBar';
import { Card, CardBody } from '@/components/ui/Card';
import { Button } from '@/components/ui/Button';
import { Badge } from '@/components/ui/Badge';
import { useConnections } from '@/api/hooks';
import type { Connection } from '@/lib/mock/store';

const ICON: Record<Connection['type'], any> = { postgres: Database, sftp: Send, smtp: Mail, graph: Server };

export default function ConnectionsPage() {
  const { data } = useConnections();
  const connections = data?.connections ?? [];
  return (
    <>
      <TopBar
        title="Connections"
        actions={<Button size="sm"><Plus className="h-4 w-4" /> New connection</Button>}
      />
      <main className="flex-1 overflow-auto p-6">
        <p className="mb-4 max-w-2xl text-sm text-ink-muted">
          Credentials are stored in Azure Key Vault — this list holds only metadata and a
          reference. Access is controlled per role.
        </p>
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
          {connections.map((c) => {
            const Icon = ICON[c.type];
            return (
              <Card key={c.id}>
                <CardBody>
                  <div className="flex items-start justify-between">
                    <div className="flex items-center gap-3">
                      <div className="grid h-10 w-10 place-items-center rounded-lg bg-brand/10 text-brand">
                        <Icon className="h-5 w-5" />
                      </div>
                      <div>
                        <div className="font-medium text-ink">{c.name}</div>
                        <div className="text-xs text-ink-subtle">{c.host}</div>
                      </div>
                    </div>
                    <span
                      className={`text-xs font-medium ${c.status === 'ok' ? 'text-success' : c.status === 'error' ? 'text-danger' : 'text-ink-subtle'}`}
                    >
                      {c.status === 'ok' ? '● connected' : c.status}
                    </span>
                  </div>
                  <div className="mt-4 flex flex-wrap items-center gap-1.5">
                    <ShieldCheck className="h-3.5 w-3.5 text-ink-subtle" />
                    {c.allowedRoles.map((r) => (
                      <Badge key={r}>{r}</Badge>
                    ))}
                  </div>
                  <div className="mt-4 flex gap-2">
                    <Button size="sm" variant="secondary">Test</Button>
                    <Button size="sm" variant="ghost">Edit</Button>
                  </div>
                </CardBody>
              </Card>
            );
          })}
        </div>
      </main>
    </>
  );
}
