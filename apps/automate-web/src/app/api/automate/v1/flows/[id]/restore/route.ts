import { NextResponse } from 'next/server';
import { flows } from '@/lib/mock/store';

// POST /api/automate/v1/flows/{id}/restore — un-delete (admin only; E3-S6).
// Mock: echoes the flow back without deletedAt. The real API clears deletedAt +
// writes an audit entry.
export function POST(_req: Request, { params }: { params: { id: string } }) {
  const flow = flows.find((f) => f.id === params.id);
  if (!flow) return NextResponse.json({ error: 'not_found' }, { status: 404 });
  const { deletedAt: _deletedAt, ...restored } = flow;
  return NextResponse.json(restored);
}
