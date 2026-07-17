import { NextResponse } from 'next/server';

// DELETE /api/automate/v1/flows/{id} — soft delete (E3-S6). Mock: 204 No Content.
// The real API stamps deletedAt + writes an audit entry.
export function DELETE() {
  return new NextResponse(null, { status: 204 });
}
