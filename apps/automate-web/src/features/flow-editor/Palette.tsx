'use client';

// Left rail: the node catalog, grouped by NODE_CATEGORIES, with a search box.
// Items are draggable onto the canvas (sets the node type on dataTransfer) and
// also clickable to add the node at a default position (accessible fallback).
import { useMemo, useState } from 'react';
import { Search } from 'lucide-react';
import { NODE_TYPES, NODE_CATEGORIES, type NodeType } from '@/lib/nodeRegistry';
import { cn } from '@/lib/cn';
import { nodeIcon } from './nodeIcons';

export const DND_MIME = 'application/x-automate-node';

export function Palette({ onAdd }: { onAdd: (nodeType: string) => void }) {
  const [q, setQ] = useState('');

  const grouped = useMemo(() => {
    const query = q.trim().toLowerCase();
    const match = (n: NodeType) =>
      !query ||
      n.label.toLowerCase().includes(query) ||
      n.description.toLowerCase().includes(query) ||
      n.type.toLowerCase().includes(query);
    return NODE_CATEGORIES.map((cat) => ({
      category: cat,
      items: NODE_TYPES.filter((n) => n.category === cat && match(n)),
    })).filter((g) => g.items.length > 0);
  }, [q]);

  return (
    <aside className="flex w-64 shrink-0 flex-col border-r border-border bg-surface" aria-label="Node palette">
      <div className="border-b border-border p-3">
        <div className="relative">
          <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-ink-subtle" />
          <input
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder="Search nodes…"
            aria-label="Search nodes"
            className="h-9 w-full rounded-md border border-border bg-surface pl-9 pr-3 text-sm outline-none focus:ring-2 focus:ring-brand"
          />
        </div>
      </div>

      <div className="flex-1 overflow-auto p-3">
        {grouped.length === 0 && (
          <p className="px-1 py-6 text-center text-sm text-ink-muted">No nodes match.</p>
        )}
        {grouped.map((group) => (
          <div key={group.category} className="mb-4 last:mb-0">
            <h3 className="mb-1.5 px-1 text-[11px] font-semibold uppercase tracking-wide text-ink-subtle">
              {group.category}
            </h3>
            <div className="space-y-1">
              {group.items.map((node) => (
                <PaletteItem key={node.type} node={node} onAdd={onAdd} />
              ))}
            </div>
          </div>
        ))}
      </div>
    </aside>
  );
}

function PaletteItem({ node, onAdd }: { node: NodeType; onAdd: (t: string) => void }) {
  const Icon = nodeIcon(node.icon);
  return (
    <button
      type="button"
      draggable
      onDragStart={(e) => {
        e.dataTransfer.setData(DND_MIME, node.type);
        e.dataTransfer.effectAllowed = 'move';
      }}
      onClick={() => onAdd(node.type)}
      title={node.description}
      aria-label={`Add ${node.label} node`}
      className={cn(
        'flex w-full items-center gap-2.5 rounded-md border border-transparent px-2 py-2 text-left',
        'hover:border-border hover:bg-surface-sunken focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand',
      )}
    >
      <span className={cn('grid h-8 w-8 shrink-0 place-items-center rounded-md bg-surface-sunken', node.accent)}>
        <Icon className="h-4 w-4" />
      </span>
      <span className="min-w-0">
        <span className="block truncate text-sm font-medium text-ink">{node.label}</span>
        <span className="block truncate text-[11px] text-ink-subtle">{node.description}</span>
      </span>
    </button>
  );
}
