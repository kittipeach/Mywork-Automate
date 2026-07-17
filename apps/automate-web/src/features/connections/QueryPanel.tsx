'use client';

// Query preview + visual builder (E6-S2 / E6-S4). Pick a connection, optionally
// build a SELECT visually (table + columns + one WHERE) which generates SQL into
// the editable monospace editor, then Preview → masked results table. Invalid
// SQL (400) renders inline. The SQL-generation is the pure buildSelect() in
// src/lib (unit-tested); this component is the thin presentational shell.
import { useMemo, useState } from 'react';
import { Play, Loader2, AlertTriangle, Wand2 } from 'lucide-react';
import { Button } from '@/components/ui/Button';
import { cn } from '@/lib/cn';
import { ApiError } from '@/api/mutations';
import { useConnectionSchema, useQueryPreview } from '@/api/connections';
import { buildSelect, WHERE_OPS, type WhereOp } from '@/lib/buildSelect';

export type QueryPanelProps = {
  connectionId: string;
  sql: string;
  onSqlChange: (sql: string) => void;
  maxRows?: number;
};

export function QueryPanel({ connectionId, sql, onSqlChange, maxRows = 100 }: QueryPanelProps) {
  const [showBuilder, setShowBuilder] = useState(false);
  const preview = useQueryPreview();

  const runPreview = () => {
    if (!connectionId || !sql.trim()) return;
    preview.mutate({ connectionId, sql, maxRows });
  };

  const errorMessage =
    preview.error instanceof ApiError && preview.error.status === 400
      ? 'Invalid SQL — only single-statement SELECT is allowed.'
      : preview.isError
        ? 'Preview failed. Check the connection and try again.'
        : null;

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <span className="text-sm font-medium text-ink">Query</span>
        <button
          type="button"
          onClick={() => setShowBuilder((v) => !v)}
          className="inline-flex items-center gap-1 text-xs font-medium text-brand hover:text-brand-700"
          aria-expanded={showBuilder}
        >
          <Wand2 className="h-3.5 w-3.5" />
          {showBuilder ? 'Hide builder' : 'Visual builder'}
        </button>
      </div>

      {showBuilder && (
        <VisualBuilder
          connectionId={connectionId}
          onGenerate={(generated) => {
            onSqlChange(generated);
            setShowBuilder(false);
          }}
        />
      )}

      <textarea
        aria-label="SQL"
        value={sql}
        onChange={(e) => onSqlChange(e.target.value)}
        rows={5}
        spellCheck={false}
        placeholder="SELECT * FROM ..."
        className="w-full rounded-md border border-border bg-surface px-3 py-2 font-mono text-sm outline-none focus:ring-2 focus:ring-brand"
      />

      <div className="flex items-center gap-2">
        <Button
          size="sm"
          onClick={runPreview}
          disabled={!connectionId || !sql.trim() || preview.isPending}
        >
          {preview.isPending ? <Loader2 className="h-4 w-4 animate-spin" /> : <Play className="h-4 w-4" />}
          Preview
        </Button>
        {!connectionId && <span className="text-xs text-ink-subtle">Select a connection first.</span>}
      </div>

      {errorMessage && (
        <div
          role="alert"
          className="flex items-start gap-1.5 rounded-md border border-danger/30 bg-danger/5 px-3 py-2 text-xs text-danger"
        >
          <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
          <span>{errorMessage}</span>
        </div>
      )}

      {preview.isSuccess && preview.data && <ResultsTable data={preview.data} />}
    </div>
  );
}

function ResultsTable({
  data,
}: {
  data: { items: Record<string, unknown>[]; meta: { columns: string[]; rowCount: number; truncated: boolean } };
}) {
  const { items, meta } = data;
  if (meta.columns.length === 0) {
    return <p className="text-xs text-ink-muted">Query ran — no columns returned.</p>;
  }
  return (
    <div className="space-y-1.5">
      <div className="overflow-auto rounded-md border border-border">
        <table className="w-full text-left text-xs">
          <thead>
            <tr className="border-b border-border bg-surface-sunken text-ink-subtle">
              {meta.columns.map((col) => (
                <th key={col} className="px-3 py-2 font-medium">
                  {col}
                </th>
              ))}
            </tr>
          </thead>
          <tbody className="divide-y divide-border">
            {items.map((row, i) => (
              <tr key={i} className="hover:bg-surface-sunken">
                {meta.columns.map((col) => (
                  <td key={col} className="px-3 py-1.5 font-mono text-ink-muted">
                    {formatCell(row[col])}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <p className="text-xs text-ink-subtle">
        Showing {items.length} of {meta.rowCount}
        {meta.truncated ? ' (truncated)' : ''} · values are masked per policy
      </p>
    </div>
  );
}

function formatCell(v: unknown): string {
  if (v === null || v === undefined) return '∅';
  if (typeof v === 'object') return JSON.stringify(v);
  return String(v);
}

function VisualBuilder({
  connectionId,
  onGenerate,
}: {
  connectionId: string;
  onGenerate: (sql: string) => void;
}) {
  const { data, isLoading, isError } = useConnectionSchema(connectionId);
  const tables = data?.tables ?? [];

  const [table, setTable] = useState('');
  const [selectedCols, setSelectedCols] = useState<string[]>([]);
  const [whereCol, setWhereCol] = useState('');
  const [whereOp, setWhereOp] = useState<WhereOp>('=');
  const [whereVal, setWhereVal] = useState('');

  const columns = useMemo(
    () => tables.find((t) => t.name === table)?.columns ?? [],
    [tables, table],
  );

  const generated = useMemo(() => {
    if (!table) return '';
    try {
      return buildSelect({
        table,
        columns: selectedCols,
        where: whereCol ? { column: whereCol, op: whereOp, value: whereVal } : undefined,
      });
    } catch {
      return '';
    }
  }, [table, selectedCols, whereCol, whereOp, whereVal]);

  if (!connectionId) {
    return <p className="text-xs text-ink-subtle">Select a connection to browse its schema.</p>;
  }
  if (isLoading) return <p className="text-xs text-ink-muted">Loading schema…</p>;
  if (isError) return <p className="text-xs text-danger">Could not load schema.</p>;

  return (
    <div className="space-y-3 rounded-md border border-border bg-surface-sunken/40 p-3">
      <div className="space-y-1">
        <label htmlFor="qb-table" className="text-xs font-medium text-ink">
          Table
        </label>
        <select
          id="qb-table"
          value={table}
          onChange={(e) => {
            setTable(e.target.value);
            setSelectedCols([]);
            setWhereCol('');
          }}
          className="h-8 w-full rounded-md border border-border bg-surface px-2 text-sm outline-none focus:ring-2 focus:ring-brand"
        >
          <option value="">— Select table —</option>
          {tables.map((t) => (
            <option key={t.name} value={t.name}>
              {t.name}
            </option>
          ))}
        </select>
      </div>

      {table && (
        <>
          <fieldset className="space-y-1">
            <legend className="text-xs font-medium text-ink">Columns (none = all)</legend>
            <div className="flex flex-wrap gap-1.5">
              {columns.map((col) => {
                const checked = selectedCols.includes(col.name);
                return (
                  <label
                    key={col.name}
                    className={cn(
                      'flex cursor-pointer items-center gap-1 rounded border px-2 py-0.5 text-xs',
                      checked ? 'border-brand bg-brand/10 text-brand-700' : 'border-border text-ink-muted',
                    )}
                  >
                    <input
                      type="checkbox"
                      className="sr-only"
                      checked={checked}
                      onChange={(e) =>
                        setSelectedCols((prev) =>
                          e.target.checked ? [...prev, col.name] : prev.filter((c) => c !== col.name),
                        )
                      }
                    />
                    {col.name}
                    <span className="text-ink-subtle">{col.type}</span>
                  </label>
                );
              })}
            </div>
          </fieldset>

          <div className="space-y-1">
            <span className="text-xs font-medium text-ink">Where (optional)</span>
            <div className="flex gap-1.5">
              <select
                aria-label="Where column"
                value={whereCol}
                onChange={(e) => setWhereCol(e.target.value)}
                className="h-8 flex-1 rounded-md border border-border bg-surface px-2 text-sm outline-none focus:ring-2 focus:ring-brand"
              >
                <option value="">—</option>
                {columns.map((col) => (
                  <option key={col.name} value={col.name}>
                    {col.name}
                  </option>
                ))}
              </select>
              <select
                aria-label="Where operator"
                value={whereOp}
                onChange={(e) => setWhereOp(e.target.value as WhereOp)}
                className="h-8 w-20 rounded-md border border-border bg-surface px-2 text-sm outline-none focus:ring-2 focus:ring-brand"
              >
                {WHERE_OPS.map((op) => (
                  <option key={op} value={op}>
                    {op}
                  </option>
                ))}
              </select>
              <input
                aria-label="Where value"
                value={whereVal}
                onChange={(e) => setWhereVal(e.target.value)}
                placeholder="value"
                className="h-8 flex-1 rounded-md border border-border bg-surface px-2 text-sm outline-none focus:ring-2 focus:ring-brand"
              />
            </div>
          </div>

          {generated && (
            <div className="space-y-1">
              <span className="text-xs font-medium text-ink">Generated SQL</span>
              <pre className="overflow-auto rounded border border-border bg-surface px-2 py-1.5 font-mono text-xs text-ink-muted">
                {generated}
              </pre>
              <Button size="sm" variant="secondary" onClick={() => onGenerate(generated)}>
                Use this SQL
              </Button>
            </div>
          )}
        </>
      )}
    </div>
  );
}
