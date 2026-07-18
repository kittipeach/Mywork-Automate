// Pure model + helpers for the visual query builder (E6-S2). The builder emits a
// STRUCTURED spec (matching the server's pkg/sqlbuilder) — never free-form SQL.
// The server compiles and guards the real query; previewSQL() here only renders
// a human-readable approximation for the panel. Kept separate from the React
// component so the logic is unit-tested (src/features is gated ≥90%).

export type JoinType = 'inner' | 'left';
export const JOIN_TYPES: JoinType[] = ['inner', 'left'];

/** Filter operators the builder exposes (mirrors sqlbuilder's whitelist). */
export const FILTER_OPS = ['=', '!=', '>', '>=', '<', '<=', 'like'] as const;
export type FilterOp = (typeof FILTER_OPS)[number];

export type OnCond = { leftTable: string; leftCol: string; rightTable: string; rightCol: string };
export type Join = { type: JoinType; table: string; on: OnCond[] };
export type Column = { table?: string; name: string; alias?: string };
export type Filter = { table?: string; column: string; op: FilterOp; value: string };

export type QuerySpec = {
  table: string;
  joins?: Join[];
  columns: Column[];
  where?: Filter[];
  combinator?: 'and' | 'or';
  limit?: number;
};

export function emptySpec(): QuerySpec {
  return { table: '', joins: [], columns: [], where: [], combinator: 'and', limit: 100 };
}

/** Every table referenced by the query, base first then each join, de-duplicated. */
export function tablesInPlay(spec: QuerySpec): string[] {
  const out: string[] = [];
  if (spec.table) out.push(spec.table);
  for (const j of spec.joins ?? []) {
    if (j.table && !out.includes(j.table)) out.push(j.table);
  }
  return out;
}

/**
 * specComplete reports whether the spec is runnable: a base table, at least one
 * column, and every join fully specified (a table + at least one complete ON).
 * The panel uses it to gate the preview and flag the node incomplete.
 */
export function specComplete(spec: QuerySpec): boolean {
  if (!spec.table.trim()) return false;
  if (!spec.columns.length || spec.columns.some((c) => !c.name.trim())) return false;
  for (const j of spec.joins ?? []) {
    if (!j.table.trim() || !j.on.length) return false;
    for (const o of j.on) {
      if (!o.leftTable || !o.leftCol || !o.rightTable || !o.rightCol) return false;
    }
  }
  for (const f of spec.where ?? []) {
    if (!f.column.trim()) return false;
  }
  return true;
}

function q(id: string): string {
  return `"${(id ?? '').replace(/"/g, '""')}"`;
}

function qualified(table: string | undefined, col: string): string {
  return table ? `${q(table)}.${q(col)}` : q(col);
}

function literal(value: string): string {
  return /^-?\d+(\.\d+)?$/.test(value.trim()) ? value.trim() : `'${String(value).replace(/'/g, "''")}'`;
}

/**
 * previewSQL renders a read-only SQL approximation of the spec for display. It is
 * NOT sent to the server (the server compiles the spec itself with bound params);
 * it just shows the analyst what their picks mean.
 */
export function previewSQL(spec: QuerySpec, maxRows = 1000): string {
  if (!spec.table) return '';
  const cols = spec.columns.length
    ? spec.columns
        .filter((c) => c.name)
        .map((c) => qualified(c.table, c.name) + (c.alias ? ` AS ${q(c.alias)}` : ''))
        .join(', ')
    : '*';
  let sql = `SELECT ${cols} FROM ${q(spec.table)}`;
  for (const j of spec.joins ?? []) {
    if (!j.table) continue;
    const kw = j.type === 'left' ? 'LEFT JOIN' : 'INNER JOIN';
    const on = j.on
      .filter((o) => o.leftTable && o.leftCol && o.rightTable && o.rightCol)
      .map((o) => `${q(o.leftTable)}.${q(o.leftCol)} = ${q(o.rightTable)}.${q(o.rightCol)}`)
      .join(' AND ');
    sql += `\n${kw} ${q(j.table)}${on ? ` ON ${on}` : ''}`;
  }
  const preds = (spec.where ?? [])
    .filter((f) => f.column)
    .map((f) => `${qualified(f.table, f.column)} ${f.op.toUpperCase()} ${literal(f.value)}`);
  if (preds.length) sql += `\nWHERE ${preds.join(spec.combinator === 'or' ? ' OR ' : ' AND ')}`;
  const limit = spec.limit && spec.limit > 0 ? Math.min(spec.limit, maxRows) : maxRows;
  sql += `\nLIMIT ${limit}`;
  return sql;
}
