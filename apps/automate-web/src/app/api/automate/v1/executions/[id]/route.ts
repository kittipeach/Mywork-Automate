import { NextResponse } from 'next/server';
import { executions } from '@/lib/mock/store';

// GET /api/automate/v1/executions/{id} — single execution with per-step detail.
export async function GET(_req: Request, props: { params: Promise<{ id: string }> }) {
  const params = await props.params;
  const run = executions.find((e) => e.id === params.id);
  if (!run) return NextResponse.json({ error: 'not_found' }, { status: 404 });
  return NextResponse.json(run);
}
