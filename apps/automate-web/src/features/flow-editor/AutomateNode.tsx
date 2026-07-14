'use client';

// Custom React Flow node: shows the registry icon + label + user name, one
// handle per input/output port (logic.if exposes true/false outputs; triggers
// have no input), an incomplete/invalid badge, and a brand ring when selected.
import { memo } from 'react';
import { Handle, Position, type NodeProps } from '@xyflow/react';
import { AlertTriangle } from 'lucide-react';
import { NODE_BY_TYPE } from '@/lib/nodeRegistry';
import { cn } from '@/lib/cn';
import { nodeIcon } from './nodeIcons';
import { useFlowStore } from './store';
import type { FlowNodeData } from './types';

function AutomateNodeInner({ id, data, selected }: NodeProps) {
  const d = data as FlowNodeData;
  const def = NODE_BY_TYPE[d.nodeType];
  const valid = useFlowStore((s) => s.validityByNode[id] ?? true);
  const Icon = nodeIcon(def?.icon ?? '');

  if (!def) {
    return (
      <div className="rounded-md border border-danger bg-surface px-3 py-2 text-xs text-danger">
        Unknown node: {d.nodeType}
      </div>
    );
  }

  const nInputs = def.inputs.length;
  const nOutputs = def.outputs.length;

  return (
    <div
      className={cn(
        'relative w-52 rounded-lg border bg-surface shadow-panel transition-shadow',
        selected ? 'border-brand ring-2 ring-brand' : 'border-border',
        !valid && !selected && 'border-danger/60',
      )}
      data-testid={`node-${id}`}
    >
      {/* input handles (left) */}
      {def.inputs.map((port, i) => (
        <Handle
          key={port.id}
          id={port.id}
          type="target"
          position={Position.Left}
          style={{ top: `${((i + 1) / (nInputs + 1)) * 100}%` }}
          className="!h-2.5 !w-2.5 !border-2 !border-surface !bg-ink-subtle"
          aria-label={`${d.name} input ${port.label}`}
        />
      ))}

      <div className="flex items-center gap-2 px-3 py-2">
        <span className={cn('grid h-7 w-7 shrink-0 place-items-center rounded-md bg-surface-sunken', def.accent)}>
          <Icon className="h-4 w-4" />
        </span>
        <div className="min-w-0 flex-1">
          <div className="truncate text-sm font-medium text-ink">{d.name}</div>
          <div className="truncate text-[11px] text-ink-subtle">{def.label}</div>
        </div>
        {!valid && (
          <span
            className="grid h-5 w-5 shrink-0 place-items-center rounded-full bg-danger/10 text-danger"
            title="Incomplete configuration"
            aria-label="Incomplete configuration"
          >
            <AlertTriangle className="h-3 w-3" />
          </span>
        )}
      </div>

      {/* output handles (right), with labels for multi-output nodes */}
      {def.outputs.map((port, i) => {
        const top = `${((i + 1) / (nOutputs + 1)) * 100}%`;
        return (
          <div key={port.id}>
            <Handle
              id={port.id}
              type="source"
              position={Position.Right}
              style={{ top }}
              className="!h-2.5 !w-2.5 !border-2 !border-surface !bg-brand"
              aria-label={`${d.name} output ${port.label}`}
            />
            {nOutputs > 1 && (
              <span
                className="pointer-events-none absolute right-3 -translate-y-1/2 text-[10px] font-medium text-ink-subtle"
                style={{ top }}
              >
                {port.label}
              </span>
            )}
          </div>
        );
      })}
    </div>
  );
}

export const AutomateNode = memo(AutomateNodeInner);
