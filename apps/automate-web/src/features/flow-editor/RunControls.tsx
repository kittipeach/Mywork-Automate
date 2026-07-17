'use client';

// RunControls (E3-S4) — the live Validate + Test-run controls for the flow
// editor toolbar. Replaces the earlier mock banners:
//   - Validate  → POST /flows/{id}/validate, renders an issues panel; clicking
//                 an issue selects that node on the canvas.
//   - Test run  → POST /flows/{id}/run, then polls GET /executions/{id} while the
//                 run is queued/running, projecting each step's status onto the
//                 canvas nodes (rings) and showing a compact run-status strip.
import { useEffect, useState } from 'react';
import { Play, CheckCircle2, AlertTriangle, X, Loader2 } from 'lucide-react';
import { Button } from '@/components/ui/Button';
import { StatusBadge } from '@/components/ui/Badge';
import { cn } from '@/lib/cn';
import { useValidateFlow, type ValidationIssue } from '@/api/validation';
import { useExecution } from '@/api/hooks';
import { useRunFlow } from '@/api/mutations';
import { useFlowStore } from './store';
import { stepStatusByNode, summarise, isTerminal } from './runStatus';

export function RunControls({ flowId }: { flowId: string }) {
  const nodes = useFlowStore((s) => s.nodes);
  const selectNode = useFlowStore((s) => s.selectNode);
  const setStatusByNode = useFlowStore((s) => s.setStatusByNode);
  const clearRunStatus = useFlowStore((s) => s.clearRunStatus);

  const validate = useValidateFlow();
  const runFlow = useRunFlow();

  const [showIssues, setShowIssues] = useState(false);
  const [execId, setExecId] = useState('');

  // Poll the execution while it is queued/running; stop once terminal.
  const execution = useExecution(execId, { poll: true });
  const exec = execution.data;
  const summary = summarise(exec);

  // Project step statuses onto the canvas nodes as the run progresses.
  useEffect(() => {
    if (exec) setStatusByNode(stepStatusByNode(exec.steps));
  }, [exec, setStatusByNode]);

  const nameById = new Map(nodes.map((n) => [n.id, n.data.name]));

  const handleValidate = () => {
    if (!flowId || validate.isPending) return;
    setShowIssues(true);
    validate.mutate(flowId);
  };

  const handleRun = () => {
    if (!flowId || runFlow.isPending) return;
    clearRunStatus();
    runFlow.mutate(flowId, {
      onSuccess: ({ executionId }) => setExecId(executionId),
    });
  };

  const closeRun = () => {
    setExecId('');
    clearRunStatus();
  };

  const runInFlight = runFlow.isPending || (!!exec && !isTerminal(exec.status));
  const result = validate.data;

  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-center gap-2">
        <Button
          size="sm"
          variant="secondary"
          onClick={handleValidate}
          disabled={validate.isPending}
        >
          <CheckCircle2 className="h-4 w-4" />
          {validate.isPending ? 'Validating…' : 'Validate'}
        </Button>
        <Button size="sm" variant="secondary" onClick={handleRun} disabled={runInFlight}>
          {runInFlight ? <Loader2 className="h-4 w-4 animate-spin" /> : <Play className="h-4 w-4" />}
          {runInFlight ? 'Running…' : 'Test run'}
        </Button>
      </div>

      {/* Validation results panel */}
      {showIssues && (
        <ValidatePanel
          loading={validate.isPending}
          error={validate.isError}
          valid={result?.valid}
          issues={result?.issues ?? []}
          nameById={nameById}
          onSelect={(nodeId) => selectNode(nodeId)}
          onClose={() => setShowIssues(false)}
        />
      )}

      {/* Compact run-status strip */}
      {summary && (
        <div
          role="status"
          aria-label="Test run status"
          className="flex items-center gap-3 rounded-md border border-border bg-surface px-3 py-2 text-sm shadow-panel"
        >
          <StatusBadge status={summary.status} />
          <span className="text-ink-muted">
            {summary.done}/{summary.total} done
            {summary.failed > 0 && <span className="text-danger"> · {summary.failed} failed</span>}
            {summary.running > 0 && <span className="text-info"> · {summary.running} running</span>}
          </span>
          <button
            type="button"
            aria-label="Dismiss run"
            onClick={closeRun}
            className="ml-auto shrink-0 text-ink-subtle hover:text-ink"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
      )}
    </div>
  );
}

function ValidatePanel({
  loading,
  error,
  valid,
  issues,
  nameById,
  onSelect,
  onClose,
}: {
  loading: boolean;
  error: boolean;
  valid?: boolean;
  issues: ValidationIssue[];
  nameById: Map<string, string>;
  onSelect: (nodeId: string) => void;
  onClose: () => void;
}) {
  return (
    <div
      role="region"
      aria-label="Validation results"
      className="w-[28rem] max-w-[90vw] rounded-md border border-border bg-surface p-3 shadow-pop"
    >
      <div className="mb-2 flex items-center justify-between">
        <div className="text-sm font-semibold text-ink">Validation</div>
        <button type="button" aria-label="Close validation" onClick={onClose} className="text-ink-subtle hover:text-ink">
          <X className="h-4 w-4" />
        </button>
      </div>

      {loading && <p className="text-sm text-ink-muted">Validating…</p>}
      {error && <p className="text-sm text-danger">Validation request failed. Try again.</p>}

      {!loading && !error && valid && issues.length === 0 && (
        <p className="flex items-center gap-1.5 text-sm font-medium text-success">
          <CheckCircle2 className="h-4 w-4" /> Valid
        </p>
      )}

      {!loading && !error && issues.length > 0 && (
        <ul className="space-y-1">
          {issues.map((issue, i) => {
            const isError = issue.severity === 'error';
            const nodeName = issue.nodeId ? nameById.get(issue.nodeId) ?? issue.nodeId : null;
            return (
              <li key={`${issue.nodeId ?? 'flow'}-${i}`}>
                <button
                  type="button"
                  disabled={!issue.nodeId}
                  onClick={() => issue.nodeId && onSelect(issue.nodeId)}
                  className={cn(
                    'flex w-full items-start gap-2 rounded px-2 py-1.5 text-left text-sm',
                    issue.nodeId && 'hover:bg-surface-sunken',
                    !issue.nodeId && 'cursor-default',
                  )}
                >
                  <AlertTriangle
                    className={cn('mt-0.5 h-4 w-4 shrink-0', isError ? 'text-danger' : 'text-warning')}
                    aria-hidden="true"
                  />
                  <span className="min-w-0 flex-1">
                    {nodeName && <span className="font-medium text-ink">{nodeName}: </span>}
                    <span className={isError ? 'text-danger' : 'text-warning'}>{issue.message}</span>
                  </span>
                </button>
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}
