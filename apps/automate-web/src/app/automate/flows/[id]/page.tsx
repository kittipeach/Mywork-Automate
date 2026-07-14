'use client';

// Flow designer (E3-S1 + E3-S2). Three panes: palette (left), React Flow canvas
// (center), config panel (right). Top toolbar carries the flow name, Undo/Redo,
// and the mock Validate / Test run / Publish actions. Seeded with a realistic
// sample flow so any flow id opens onto a populated canvas.
import { useEffect } from 'react';
import { useParams } from 'next/navigation';
import { flows } from '@/lib/mock/store';
import {
  Canvas,
  Palette,
  ConfigPanel,
  EditorToolbar,
  useFlowStore,
  useUndoRedoHotkeys,
  seedFlow,
} from '@/features/flow-editor';

export default function FlowEditorPage() {
  const params = useParams<{ id: string }>();
  const id = params?.id ?? '';
  const flowName = flows.find((f) => f.id === id)?.name ?? 'Untitled flow';

  const setGraph = useFlowStore((s) => s.setGraph);
  const addNode = useFlowStore((s) => s.addNode);
  const selectNode = useFlowStore((s) => s.selectNode);

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

  return (
    <div className="flex h-full min-h-0 flex-col">
      <EditorToolbar flowName={flowName} />
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
