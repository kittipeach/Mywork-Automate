import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/react';
import { QueryPanel } from './QueryPanel';
import { ApiError } from '@/api/mutations';

// Controllable stand-ins for the two hooks the panel uses.
let previewState: any;
let schemaState: any;
const previewMutate = vi.fn();

vi.mock('@/api/connections', () => ({
  useQueryPreview: () => ({ ...previewState, mutate: previewMutate }),
  useConnectionSchema: () => schemaState,
}));

beforeEach(() => {
  cleanup();
  previewMutate.mockReset();
  previewState = { isPending: false, isSuccess: false, isError: false, error: null, data: undefined };
  schemaState = {
    isLoading: false,
    isError: false,
    data: {
      tables: [
        {
          name: 'employees',
          columns: [
            { name: 'id', type: 'int' },
            { name: 'name', type: 'text' },
          ],
        },
      ],
    },
  };
});

function Harness(props: { connectionId?: string; sql?: string }) {
  return (
    <QueryPanel
      connectionId={props.connectionId ?? 'conn_1'}
      sql={props.sql ?? 'SELECT 1'}
      onSqlChange={() => {}}
    />
  );
}

describe('QueryPanel — preview', () => {
  it('runs a preview with the current sql', () => {
    render(<Harness sql="SELECT * FROM t" />);
    fireEvent.click(screen.getByRole('button', { name: /Preview/ }));
    expect(previewMutate).toHaveBeenCalledWith({ connectionId: 'conn_1', sql: 'SELECT * FROM t', maxRows: 100 });
  });

  it('disables Preview with no connection or blank sql', () => {
    const { rerender } = render(<Harness connectionId="" sql="SELECT 1" />);
    expect(screen.getByRole('button', { name: /Preview/ })).toBeDisabled();
    rerender(<Harness connectionId="conn_1" sql="   " />);
    expect(screen.getByRole('button', { name: /Preview/ })).toBeDisabled();
  });

  it('renders the masked results table with a truncation note', () => {
    previewState = {
      isPending: false,
      isSuccess: true,
      isError: false,
      error: null,
      data: {
        items: [
          { id: 1, name: 'a' },
          { id: 2, name: 'b' },
        ],
        meta: { columns: ['id', 'name'], rowCount: 999, truncated: true },
      },
    };
    render(<Harness />);
    expect(screen.getByText('id')).toBeInTheDocument();
    expect(screen.getByText('a')).toBeInTheDocument();
    expect(screen.getByText(/Showing 2 of 999 \(truncated\)/)).toBeInTheDocument();
  });

  it('shows the invalid_sql message on a 400', () => {
    previewState = {
      isPending: false,
      isSuccess: false,
      isError: true,
      error: new ApiError(400, '/connections/conn_1/query-preview'),
      data: undefined,
    };
    render(<Harness />);
    expect(screen.getByRole('alert')).toHaveTextContent(/only single-statement SELECT/i);
  });

  it('renders an empty-result note when no columns come back', () => {
    previewState = {
      isPending: false,
      isSuccess: true,
      isError: false,
      error: null,
      data: { items: [], meta: { columns: [], rowCount: 0, truncated: false } },
    };
    render(<Harness />);
    expect(screen.getByText(/no columns returned/i)).toBeInTheDocument();
  });

  it('formats null and object cell values', () => {
    previewState = {
      isPending: false,
      isSuccess: true,
      isError: false,
      error: null,
      data: {
        items: [{ a: null, b: { x: 1 } }],
        meta: { columns: ['a', 'b'], rowCount: 1, truncated: false },
      },
    };
    render(<Harness />);
    expect(screen.getByText('∅')).toBeInTheDocument();
    expect(screen.getByText('{"x":1}')).toBeInTheDocument();
  });

  it('hints to pick a connection first when none is selected', () => {
    render(<Harness connectionId="" />);
    expect(screen.getByText(/Select a connection first/i)).toBeInTheDocument();
  });

  it('shows a generic message on a non-400 failure', () => {
    previewState = {
      isPending: false,
      isSuccess: false,
      isError: true,
      error: new ApiError(500, '/x'),
      data: undefined,
    };
    render(<Harness />);
    expect(screen.getByRole('alert')).toHaveTextContent(/Preview failed/i);
  });
});

describe('QueryPanel — visual builder', () => {
  it('toggles the builder and generates SELECT from a table + column + where', () => {
    const onSqlChange = vi.fn();
    render(
      <QueryPanel connectionId="conn_1" sql="" onSqlChange={onSqlChange} />,
    );
    fireEvent.click(screen.getByRole('button', { name: /Visual builder/ }));

    fireEvent.change(screen.getByLabelText('Table'), { target: { value: 'employees' } });
    // pick the "name" column via its checkbox (labels also carry the type)
    fireEvent.click(screen.getByRole('checkbox', { name: /name text/ }));
    fireEvent.change(screen.getByLabelText('Where column'), { target: { value: 'id' } });
    fireEvent.change(screen.getByLabelText('Where operator'), { target: { value: '>' } });
    fireEvent.change(screen.getByLabelText('Where value'), { target: { value: '5' } });

    // generated SQL preview appears
    expect(screen.getByText(/SELECT "name" FROM "employees" WHERE "id" > 5/)).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: /Use this SQL/ }));
    expect(onSqlChange).toHaveBeenCalledWith('SELECT "name" FROM "employees" WHERE "id" > 5');
  });

  it('prompts to select a connection when none is set', () => {
    render(<QueryPanel connectionId="" sql="" onSqlChange={() => {}} />);
    fireEvent.click(screen.getByRole('button', { name: /Visual builder/ }));
    expect(screen.getByText(/Select a connection to browse/i)).toBeInTheDocument();
  });

  it('shows loading and error states for the schema', () => {
    schemaState = { isLoading: true, isError: false, data: undefined };
    const { rerender } = render(<QueryPanel connectionId="conn_1" sql="" onSqlChange={() => {}} />);
    fireEvent.click(screen.getByRole('button', { name: /Visual builder/ }));
    expect(screen.getByText(/Loading schema/i)).toBeInTheDocument();

    schemaState = { isLoading: false, isError: true, data: undefined };
    rerender(<QueryPanel connectionId="conn_1" sql="" onSqlChange={() => {}} />);
    expect(screen.getByText(/Could not load schema/i)).toBeInTheDocument();
  });
});
