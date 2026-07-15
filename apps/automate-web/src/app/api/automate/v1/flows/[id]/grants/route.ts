import { NextResponse } from 'next/server';
import { grants } from '@/lib/mock/store';

// GET /api/automate/v1/flows/{id}/grants — list a flow's access grants (E2-S3).
export function GET(_req: Request, { params }: { params: { id: string } }) {
  const forFlow = grants
    .filter((g) => g.flowId === params.id)
    .map(({ flowId: _flowId, ...g }) => g);
  return NextResponse.json({ grants: forFlow });
}

// POST /api/automate/v1/flows/{id}/grants — add a grant. Mock: echoes it with a
// generated id (the real API persists + audits).
export async function POST(req: Request, { params }: { params: { id: string } }) {
  const body = (await req.json()) as Record<string, unknown>;
  return NextResponse.json(
    { id: `grn_${params.id}_${Date.now()}`, ...body },
    { status: 201 },
  );
}
