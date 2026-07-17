import { NextResponse } from 'next/server';

// GET /api/automate/v1/connections/{id}/schema — table/column catalog.
// Mock fixture for the visual builder; the real API introspects the DB.
export function GET() {
  return NextResponse.json({
    tables: [
      {
        name: 'employees',
        columns: [
          { name: 'id', type: 'int' },
          { name: 'name', type: 'text' },
          { name: 'department', type: 'text' },
          { name: 'salary', type: 'numeric' },
          { name: 'hired_at', type: 'date' },
        ],
      },
      {
        name: 'departments',
        columns: [
          { name: 'id', type: 'int' },
          { name: 'name', type: 'text' },
          { name: 'headcount', type: 'int' },
        ],
      },
    ],
  });
}
