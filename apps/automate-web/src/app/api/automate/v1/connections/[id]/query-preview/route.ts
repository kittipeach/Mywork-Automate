import { NextResponse } from 'next/server';

// POST /api/automate/v1/connections/{id}/query-preview {sql, maxRows?} — mock.
// Rejects anything that isn't a SELECT with 400 invalid_sql (mirrors the real
// sqlguard); otherwise returns a small masked sample.
export async function POST(req: Request) {
  const { sql } = (await req.json()) as { sql?: string };
  if (!sql || !/^\s*select\b/i.test(sql)) {
    return NextResponse.json({ error: 'invalid_sql' }, { status: 400 });
  }
  return NextResponse.json({
    items: [
      { id: 1, name: 'A. Somchai', department: 'HR Ops', salary: '••••' },
      { id: 2, name: 'B. Wanida', department: 'Finance', salary: '••••' },
      { id: 3, name: 'C. Krit', department: 'HR Ops', salary: '••••' },
    ],
    meta: { columns: ['id', 'name', 'department', 'salary'], rowCount: 3, truncated: false },
  });
}
