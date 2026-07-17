import { NextResponse } from 'next/server';

// POST /api/automate/v1/connections/{id}/test — probe. Mock: always ok.
export function POST() {
  return NextResponse.json({ status: 'ok' });
}
