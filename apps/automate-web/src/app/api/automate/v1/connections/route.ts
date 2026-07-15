import { NextResponse } from 'next/server';
import { connections } from '@/lib/mock/store';

// GET /api/automate/v1/connections — RBAC-filtered in the real API.
export function GET() {
  return NextResponse.json({ connections });
}

// POST /api/automate/v1/connections — create. Mock: echoes back with an id.
export async function POST(req: Request) {
  const body = (await req.json()) as Record<string, unknown>;
  const created = {
    id: `conn_${Math.random().toString(36).slice(2, 8)}`,
    status: 'untested',
    allowedRoles: [],
    ...body,
  };
  return NextResponse.json(created, { status: 201 });
}
