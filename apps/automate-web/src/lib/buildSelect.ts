// Pure SQL generator for the visual query builder (E6-S4). Produces a single
// read-only SELECT from a table + column pick + optional simple WHERE. The
// server (pkg/sqlguard) is the real gate — this only has to emit well-formed,
// SELECT-only SQL with identifiers safely quoted so the generated preview is
// valid and injection-resistant. The user can still hand-edit the SQL after.

/** The comparison operators the simple WHERE builder exposes. */
export const WHERE_OPS = ['=', '!=', '>', '>=', '<', '<=', 'LIKE'] as const;
export type WhereOp = (typeof WHERE_OPS)[number];

export type WhereClause = {
  column: string;
  op: WhereOp;
  /** Literal compared against; numbers stay unquoted, everything else is quoted. */
  value: string;
};

export type BuildSelectInput = {
  table: string;
  /** Empty / omitted → SELECT * (all columns). */
  columns?: string[];
  where?: WhereClause;
  limit?: number;
};

/**
 * Quote a Postgres identifier: wrap in double quotes and double any embedded
 * quote. Rejects empty identifiers so a half-filled builder can't emit `SELECT
 * FROM ""`.
 */
export function quoteIdent(name: string): string {
  const trimmed = (name ?? '').trim();
  if (!trimmed) throw new Error('empty identifier');
  return `"${trimmed.replace(/"/g, '""')}"`;
}

/** Quote a string literal: single quotes, doubling any embedded single quote. */
function quoteLiteral(value: string): string {
  return `'${String(value).replace(/'/g, "''")}'`;
}

/** True when the trimmed value is a plain numeric literal (int or decimal). */
function isNumeric(value: string): boolean {
  return /^-?\d+(\.\d+)?$/.test(value.trim());
}

function renderWhere(where: WhereClause): string {
  const col = quoteIdent(where.column);
  if (!WHERE_OPS.includes(where.op)) throw new Error(`unsupported operator: ${where.op}`);
  // LIKE always compares against text; numeric-looking values are otherwise
  // emitted bare so range comparisons work as expected.
  const rhs =
    where.op !== 'LIKE' && isNumeric(where.value)
      ? where.value.trim()
      : quoteLiteral(where.value);
  return `${col} ${where.op} ${rhs}`;
}

/**
 * Build a single-statement SELECT. Identifiers are quoted, WHERE literals are
 * escaped, and an optional LIMIT is appended. No trailing semicolon (the API
 * validates single-statement itself, and omitting it keeps re-editing clean).
 */
export function buildSelect({ table, columns, where, limit }: BuildSelectInput): string {
  const from = quoteIdent(table);
  const cols =
    columns && columns.length > 0 ? columns.map(quoteIdent).join(', ') : '*';

  let sql = `SELECT ${cols} FROM ${from}`;
  if (where && where.column) sql += ` WHERE ${renderWhere(where)}`;
  if (typeof limit === 'number' && Number.isFinite(limit) && limit > 0) {
    sql += ` LIMIT ${Math.floor(limit)}`;
  }
  return sql;
}
