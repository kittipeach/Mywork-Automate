import { NextResponse } from 'next/server';
import { users } from '@/lib/mock/store';

// GET /api/automate/v1/users — directory users + roles (admin only; E2-S3).
// The real API returns 403 for non-admins; the mock always returns the list.
export function GET() {
  return NextResponse.json({ users });
}
