'use client';

// Flow designer (E3-S1 + E3-S2). Three panes: palette (left), React Flow canvas
// (center), config panel (right). Top toolbar carries the flow name, Undo/Redo,
// and the mock Validate / Test run / Publish actions. Seeded with a realistic
// sample flow so any flow id opens onto a populated canvas.
import { useEffect, useState } from 'react';
import { useParams } from 'next/navigation';
import { UploadCloud, X } from 'lucide-react';
import { flows } from '@/lib/mock/store';
import { Button } from '@/components/ui/Button';
import { cn } from '@/lib/cn';
import { usePublishFlow } from '@/api/mutations';
import {
  Canvas,
  Palette,
  ConfigPanel,
  EditorToolbar,
  RunControls,
  useFlowStore,
  useUndoRedoHotkeys,
  seedFlow,
} from '@/features/flow-editor';

type Banner = { tone: 'success' | 'danger'; title: string };

const toneClasses: Record<Banner['tone'], string> = {
  success: 'bg-success/10 text-success border-success/30',
  danger: 'bg-danger/10 text-danger border-danger/30',
};

export default function FlowEditorPage() {
  const params = useParams<{ id: string }>();
  const id = params?.id ?? '';
  const flowName = flows.find((f) => f.id === id)?.name ?? 'Untitled flow';

  const setGraph = useFlowStore((s) => s.setGraph);
  const addNode = useFlowStore((s) => s.addNode);
  const selectNode = useFlowStore((s) => s.selectNode);

  const publishFlow = usePublishFlow();
  const [banner, setBanner] = useState<Banner | null>(null);

  useUndoRedoHotkeys();

  // Seed the sample flow when the editor mounts (or the flow id changes).
  useEffect(() => {
    const { snapshot, config } = seedFlow();
    setGraph(snapshot, config);
  }, [id, setGraph]);

  // Click-to-add from the palette drops the node near the top-left of the canvas.
  const handleAdd = (nodeType: string) => {
    const newId = addNode(nodeType, { x: 120, y: 120 });
    if (newId) selectNode(newId);
  };

  const handlePublish = () => {
    if (!id || publishFlow.isPending) return;
    // The API now records a change note on every publish. Keep it minimal — a
    // prompt — until a richer publish dialog lands.
    const changeNote =
      typeof window !== 'undefined' ? window.prompt('Change note for this version', '') : '';
    if (changeNote == null) return; // cancelled
    publishFlow.mutate(
      { flowId: id, changeNote },
      {
        onSuccess: (flow) =>
          setBanner({ tone: 'success', title: `Published ${flow.name} (v${flow.version})` }),
        onError: () => setBanner({ tone: 'danger', title: 'Failed to publish flow' }),
      },
    );
  };

  return (
    <div className="flex h-full min-h-0 flex-col">
      <EditorToolbar flowName={flowName} />
      <div className="relative flex items-start justify-end gap-3 border-b border-border bg-surface px-6 py-2">
        <RunControls flowId={id} />
        <Button size="sm" onClick={handlePublish} disabled={publishFlow.isPending}>
          <UploadCloud className="h-4 w-4" /> {publishFlow.isPending ? 'Publishing…' : 'Publish'}
        </Button>

        {banner && (
          <div
            role="status"
            className={cn(
              'absolute right-6 top-14 z-10 flex w-[28rem] max-w-[90%] items-start gap-3 rounded-md border px-4 py-3 shadow-pop',
              toneClasses[banner.tone],
            )}
          >
            <div className="min-w-0 flex-1">
              <div className="text-sm font-semibold">{banner.title}</div>
            </div>
            <button
              type="button"
              aria-label="Dismiss"
              onClick={() => setBanner(null)}
              className="shrink-0 opacity-70 hover:opacity-100"
            >
              <X className="h-4 w-4" />
            </button>
          </div>
        )}
      </div>
      <div className="flex min-h-0 flex-1">
        <Palette onAdd={handleAdd} />
        <div className="min-w-0 flex-1 bg-surface-sunken">
          <Canvas />
        </div>
        <ConfigPanel />
      </div>
    </div>
  );
}
