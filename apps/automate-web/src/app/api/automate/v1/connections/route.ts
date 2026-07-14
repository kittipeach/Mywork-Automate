import { NextResponse } from 'next/server';
import { connections } from '@/lib/mock/store';

// GET /api/automate/v1/connections — RBAC-filtered in the real API.
export function GET() {
  return NextResponse.json({ connections });
}
