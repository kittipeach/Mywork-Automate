import { NextResponse } from 'next/server';

// PUT /api/automate/v1/connections/{id} — update. Mock: echoes back merged.
export async function PUT(req: Request, props: { params: Promise<{ id: string }> }) {
  const params = await props.params;
  const body = (await req.json()) as Record<string, unknown>;
  return NextResponse.json({ id: params.id, status: 'untested', ...body });
}

// DELETE /api/automate/v1/connections/{id} — remove. Mock: 204 No Content.
export function DELETE() {
  return new NextResponse(null, { status: 204 });
}
