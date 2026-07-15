import { NextResponse } from 'next/server';

// DELETE /api/automate/v1/flows/{id}/grants/{grantId} — revoke a grant (E2-S3).
// Mock: 204 No Content. The real API removes the row + writes an audit entry.
export function DELETE() {
  return new NextResponse(null, { status: 204 });
}
