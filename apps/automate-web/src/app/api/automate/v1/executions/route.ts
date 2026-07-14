import { NextResponse } from 'next/server';
import { executions } from '@/lib/mock/store';

// GET /api/automate/v1/executions?status=&flowId=
export function GET(req: Request) {
  const { searchParams } = new URL(req.url);
  const status = searchParams.get('status');
  const flowId = searchParams.get('flowId');
  let result = executions;
  if (status) result = result.filter((e) => e.status === status);
  if (flowId) result = result.filter((e) => e.flowId === flowId);
  return NextResponse.json({ executions: result });
}
