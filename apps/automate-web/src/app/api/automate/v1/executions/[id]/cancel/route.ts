import { NextResponse } from 'next/server';
import { executions } from '@/lib/mock/store';

const TERMINAL = new Set(['success', 'failed', 'cancelled', 'skipped']);

// POST /api/automate/v1/executions/{id}/cancel — mock. 409 if already terminal.
export function POST(_req: Request, { params }: { params: { id: string } }) {
  const run = executions.find((e) => e.id === params.id);
  if (run && TERMINAL.has(run.status)) {
    return NextResponse.json({ error: 'already_terminal' }, { status: 409 });
  }
  return NextResponse.json({ status: 'cancelled' });
}
