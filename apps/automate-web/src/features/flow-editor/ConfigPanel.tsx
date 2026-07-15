'use client';

// Right pane: shows the selected node's SchemaForm (E3-S2) plus a rename field.
// Wires SchemaForm's onChange/onValidityChange back into the store so the node's
// incomplete badge updates live.
import { X } from 'lucide-react';
import { NODE_BY_TYPE } from '@/lib/nodeRegistry';
import { Button } from '@/components/ui/Button';
import { StatusBadge } from '@/components/ui/Badge';
import { cn } from '@/lib/cn';
import { SchemaForm } from '@/features/node-config';
import { QueryPanel } from '@/features/connections/QueryPanel';
import { nodeIcon } from './nodeIcons';
import { useFlowStore } from './store';

export function ConfigPanel() {
  const selectedNodeId = useFlowStore((s) => s.selectedNodeId);
  const node = useFlowStore((s) => s.nodes.find((n) => n.id === s.selectedNodeId));
  const config = useFlowStore((s) => (s.selectedNodeId ? s.configByNode[s.selectedNodeId] : undefined));
  const valid = useFlowStore((s) => (s.selectedNodeId ? s.validityByNode[s.selectedNodeId] ?? true : true));
  const setNodeConfig = useFlowStore((s) => s.setNodeConfig);
  const renameNode = useFlowStore((s) => s.renameNode);
  const selectNode = useFlowStore((s) => s.selectNode);

  if (!selectedNodeId || !node) {
    return (
      <aside className="flex w-80 shrink-0 flex-col border-l border-border bg-surface">
        <div className="grid flex-1 place-items-center p-6 text-center text-sm text-ink-muted">
          Select a node to configure it.
        </div>
      </aside>
    );
  }

  const def = NODE_BY_TYPE[node.data.nodeType];
  const Icon = nodeIcon(def?.icon ?? '');

  return (
    <aside className="flex w-80 shrink-0 flex-col border-l border-border bg-surface" aria-label="Node configuration panel">
      <div className="flex items-center gap-2 border-b border-border px-4 py-3">
        <span className={cn('grid h-8 w-8 shrink-0 place-items-center rounded-md bg-surface-sunken', def?.accent)}>
          <Icon className="h-4 w-4" />
        </span>
        <div className="min-w-0 flex-1">
          <div className="truncate text-sm font-semibold text-ink">{def?.label}</div>
          <div className="truncate text-[11px] text-ink-subtle">{node.data.nodeType}</div>
        </div>
        <StatusBadge status={valid ? 'success' : 'failed'} />
        <Button variant="ghost" size="sm" aria-label="Close panel" onClick={() => selectNode(null)}>
          <X className="h-4 w-4" />
        </Button>
      </div>

      <div className="flex-1 overflow-auto p-4">
        <div className="mb-4 space-y-1">
          <label htmlFor="node-name" className="text-sm font-medium text-ink">
            Node name
          </label>
          <input
            id="node-name"
            value={node.data.name}
            onChange={(e) => renameNode(selectedNodeId, e.target.value)}
            className="h-9 w-full rounded-md border border-border bg-surface px-3 text-sm outline-none focus:ring-2 focus:ring-brand"
          />
        </div>

        {def && (
          <SchemaForm
            key={selectedNodeId}
            formId={selectedNodeId}
            schema={def.schema}
            value={config}
            onChange={(v) => setNodeConfig(selectedNodeId, v)}
          />
        )}

        {node.data.nodeType === 'db.query' && (
          <div className="mt-4 border-t border-border pt-4">
            <QueryPanel
              connectionId={(config?.connectionId as string) ?? ''}
              sql={(config?.sql as string) ?? ''}
              maxRows={(config?.maxRows as number) ?? 100}
              onSqlChange={(sql) => setNodeConfig(selectedNodeId, { ...(config ?? {}), sql })}
            />
          </div>
        )}
      </div>
    </aside>
  );
}
