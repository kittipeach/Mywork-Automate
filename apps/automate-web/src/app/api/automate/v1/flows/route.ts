import { NextResponse } from 'next/server';
import { flows } from '@/lib/mock/store';

// GET /api/automate/v1/flows?q=&status=&folder=&includeDeleted= — RBAC-filtered
// in the real API. Soft-deleted flows are hidden unless includeDeleted=true
// (admin-gated on the client; server enforces the role in the real API).
export function GET(req: Request) {
  const { searchParams } = new URL(req.url);
  const q = (searchParams.get('q') ?? '').toLowerCase();
  const status = searchParams.get('status');
  const folder = searchParams.get('folder');
  const includeDeleted = searchParams.get('includeDeleted') === 'true';
  let result = flows;
  if (!includeDeleted) result = result.filter((f) => !f.deletedAt);
  if (q) result = result.filter((f) => f.name.toLowerCase().includes(q));
  if (status) result = result.filter((f) => f.status === status);
  if (folder) result = result.filter((f) => f.folder === folder);
  return NextResponse.json({ flows: result });
}
