import { NextResponse } from 'next/server';
import { grants } from '@/lib/mock/store';

// GET /api/automate/v1/flows/{id}/grants — list a flow's access grants (E2-S3).
export async function GET(_req: Request, props: { params: Promise<{ id: string }> }) {
  const params = await props.params;
  const forFlow = grants
    .filter((g) => g.flowId === params.id)
    .map(({ flowId: _flowId, ...g }) => g);
  return NextResponse.json({ grants: forFlow });
}

// POST /api/automate/v1/flows/{id}/grants — add a grant. Mock: echoes it with a
// generated id (the real API persists + audits).
export async function POST(req: Request, props: { params: Promise<{ id: string }> }) {
  const params = await props.params;
  const body = (await req.json()) as Record<string, unknown>;
  return NextResponse.json(
    { id: `grn_${params.id}_${Date.now()}`, ...body },
    { status: 201 },
  );
}
