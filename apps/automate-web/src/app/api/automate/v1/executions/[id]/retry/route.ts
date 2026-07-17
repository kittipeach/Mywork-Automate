import { NextResponse } from 'next/server';

// POST /api/automate/v1/executions/{id}/retry — mock. Returns a new run id.
export function POST() {
  return NextResponse.json({ executionId: `exe_${Math.random().toString(36).slice(2, 8)}` });
}
