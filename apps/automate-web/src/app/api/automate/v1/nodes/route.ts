import { NextResponse } from 'next/server';
import { NODE_TYPES } from '@/lib/nodeRegistry';

// GET /api/automate/v1/nodes — node registry (type, category, JSON Schema).
export function GET() {
  return NextResponse.json({ nodes: NODE_TYPES });
}
