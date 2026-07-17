import { NextResponse } from 'next/server';
import { executions } from '@/lib/mock/store';

const TERMINAL = new Set(['success', 'failed', 'cancelled', 'skipped']);

// POST /api/automate/v1/executions/{id}/cancel — mock. 409 if already terminal.
export async function POST(_req: Request, props: { params: Promise<{ id: string }> }) {
  const params = await props.params;
  const run = executions.find((e) => e.id === params.id);
  if (run && TERMINAL.has(run.status)) {
    return NextResponse.json({ error: 'already_terminal' }, { status: 409 });
  }
  return NextResponse.json({ status: 'cancelled' });
}
