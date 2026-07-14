'use client';

// The React Flow canvas. Wires the zustand store to <ReactFlow>, handles
// drag-drop from the palette, connection validation, and Backspace/Delete.
// The '@xyflow/react' stylesheet is imported once here.
import { useCallback, useMemo, useRef } from 'react';
import {
  ReactFlow,
  Background,
  Controls,
  MiniMap,
  ReactFlowProvider,
  useReactFlow,
  type Connection,
  type IsValidConnection,
} from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import { AutomateNode } from './AutomateNode';
import { DND_MIME } from './Palette';
import { useFlowStore } from './store';
import { canConnect } from './graph';

const nodeTypes = { automateNode: AutomateNode };

function CanvasInner() {
  const wrapperRef = useRef<HTMLDivElement>(null);
  const rf = useReactFlow();

  const nodes = useFlowStore((s) => s.nodes);
  const edges = useFlowStore((s) => s.edges);
  const onNodesChange = useFlowStore((s) => s.onNodesChange);
  const onEdgesChange = useFlowStore((s) => s.onEdgesChange);
  const storeConnect = useFlowStore((s) => s.connect);
  const addNode = useFlowStore((s) => s.addNode);
  const selectNode = useFlowStore((s) => s.selectNode);
  const deleteSelected = useFlowStore((s) => s.deleteSelected);

  const onConnect = useCallback(
    (c: Connection) => {
      storeConnect(c);
    },
    [storeConnect],
  );

  const isValidConnection = useCallback<IsValidConnection>(
    (c) => canConnect(c as Connection, useFlowStore.getState().nodes, useFlowStore.getState().edges),
    [],
  );

  const onDrop = useCallback(
    (event: React.DragEvent) => {
      event.preventDefault();
      const nodeType = event.dataTransfer.getData(DND_MIME);
      if (!nodeType) return;
      const position = rf.screenToFlowPosition({ x: event.clientX, y: event.clientY });
      addNode(nodeType, position);
    },
    [rf, addNode],
  );

  const onDragOver = useCallback((event: React.DragEvent) => {
    event.preventDefault();
    event.dataTransfer.dropEffect = 'move';
  }, []);

  const onKeyDown = useCallback(
    (event: React.KeyboardEvent) => {
      const target = event.target as HTMLElement;
      // don't hijack Backspace while typing in an input
      if (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA') return;
      if (event.key === 'Backspace' || event.key === 'Delete') {
        deleteSelected();
      }
    },
    [deleteSelected],
  );

  const defaultEdgeOptions = useMemo(() => ({ type: 'smoothstep' as const }), []);

  return (
    <div
      ref={wrapperRef}
      className="h-full w-full"
      onDrop={onDrop}
      onDragOver={onDragOver}
      onKeyDown={onKeyDown}
    >
      <ReactFlow
        nodes={nodes}
        edges={edges}
        nodeTypes={nodeTypes}
        onNodesChange={onNodesChange}
        onEdgesChange={onEdgesChange}
        onConnect={onConnect}
        isValidConnection={isValidConnection}
        onNodeClick={(_, node) => selectNode(node.id)}
        onPaneClick={() => selectNode(null)}
        defaultEdgeOptions={defaultEdgeOptions}
        deleteKeyCode={null}
        fitView
        proOptions={{ hideAttribution: true }}
      >
        <Background gap={16} />
        <Controls />
        <MiniMap pannable zoomable className="!bg-surface" />
      </ReactFlow>
    </div>
  );
}

export function Canvas() {
  return (
    <ReactFlowProvider>
      <CanvasInner />
    </ReactFlowProvider>
  );
}
