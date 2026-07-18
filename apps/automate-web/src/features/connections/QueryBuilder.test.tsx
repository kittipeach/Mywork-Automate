import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/react';
import { QueryBuilder } from './QueryBuilder';
import { emptySpec, type QuerySpec } from './queryBuilderSpec';

// Mock the schema hook so the builder has tables/columns without a network.
let schemaState: { data?: unknown; isLoading?: boolean; isError?: boolean };
vi.mock('@/api/connections', () => ({
  useConnectionSchema: () => schemaState,
}));

const schema = {
  tables: [
    { name: 'employees', columns: [{ name: 'id', type: 'int' }, { name: 'name', type: 'text' }, { name: 'dept_id', type: 'int' }] },
    { name: 'departments', columns: [{ name: 'id', type: 'int' }, { name: 'name', type: 'text' }] },
  ],
};

beforeEach(() => {
  cleanup();
  schemaState = { data: schema, isLoading: false, isError: false };
});

function renderBuilder(spec: QuerySpec) {
  const onChange = vi.fn();
  render(<QueryBuilder connectionId="conn_1" spec={spec} onChange={onChange} maxRows={500} />);
  return onChange;
}

describe('QueryBuilder', () => {
  it('prompts to pick a connection first', () => {
    render(<QueryBuilder connectionId="" spec={emptySpec()} onChange={() => {}} />);
    expect(screen.getByText(/pick a connection first/i)).toBeInTheDocument();
  });

  it('shows loading + error states', () => {
    schemaState = { isLoading: true };
    const { rerender } = render(<QueryBuilder connectionId="c" spec={emptySpec()} onChange={() => {}} />);
    expect(screen.getByText(/loading schema/i)).toBeInTheDocument();
    schemaState = { isError: true };
    rerender(<QueryBuilder connectionId="c" spec={emptySpec()} onChange={() => {}} />);
    expect(screen.getByText(/could not load/i)).toBeInTheDocument();
  });

  it('selects a base table', () => {
    const onChange = renderBuilder(emptySpec());
    fireEvent.change(screen.getByLabelText('Base table'), { target: { value: 'employees' } });
    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ table: 'employees', columns: [] }));
  });

  it('toggles a column on and off', () => {
    const spec: QuerySpec = { ...emptySpec(), table: 'employees' };
    const onChange = renderBuilder(spec);
    fireEvent.click(screen.getByText('name'));
    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ columns: [{ table: 'employees', name: 'name' }] }));
    // un-toggle from a spec that already has it
    cleanup();
    const onChange2 = renderBuilder({ ...spec, columns: [{ table: 'employees', name: 'name' }] });
    fireEvent.click(screen.getByText('name'));
    expect(onChange2).toHaveBeenCalledWith(expect.objectContaining({ columns: [] }));
  });

  it('adds a join and wires its table + ON columns', () => {
    const base: QuerySpec = { ...emptySpec(), table: 'employees' };
    let onChange = renderBuilder(base);
    fireEvent.click(screen.getByText(/add join/i));
    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ joins: [expect.objectContaining({ type: 'inner' })] }));

    // with a join present, pick its table → right side of ON adopts it
    cleanup();
    const withJoin: QuerySpec = { ...base, joins: [{ type: 'inner', table: '', on: [{ leftTable: 'employees', leftCol: '', rightTable: '', rightCol: '' }] }] };
    onChange = renderBuilder(withJoin);
    fireEvent.change(screen.getByLabelText('Join 1 table'), { target: { value: 'departments' } });
    expect(onChange).toHaveBeenCalledWith(
      expect.objectContaining({ joins: [expect.objectContaining({ table: 'departments', on: [expect.objectContaining({ rightTable: 'departments' })] })] }),
    );
  });

  it('sets a join ON left column (from the left table schema)', () => {
    const spec: QuerySpec = { ...emptySpec(), table: 'employees', joins: [{ type: 'inner', table: 'departments', on: [{ leftTable: 'employees', leftCol: '', rightTable: 'departments', rightCol: '' }] }] };
    const onChange = renderBuilder(spec);
    fireEvent.change(screen.getByLabelText('Join 1 left column'), { target: { value: 'dept_id' } });
    expect(onChange).toHaveBeenCalledWith(
      expect.objectContaining({ joins: [expect.objectContaining({ on: [expect.objectContaining({ leftCol: 'dept_id' })] })] }),
    );
  });

  it('removes a join', () => {
    const spec: QuerySpec = { ...emptySpec(), table: 'employees', joins: [{ type: 'left', table: 'departments', on: [] }] };
    const onChange = renderBuilder(spec);
    fireEvent.click(screen.getByLabelText('Remove join 1'));
    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ joins: [] }));
  });

  it('adds a filter, sets its column/op/value, and picks a combinator', () => {
    const spec: QuerySpec = { ...emptySpec(), table: 'employees' };
    let onChange = renderBuilder(spec);
    fireEvent.click(screen.getByText(/add filter/i));
    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ where: [expect.objectContaining({ op: '=' })] }));

    cleanup();
    const withFilter: QuerySpec = { ...spec, where: [{ table: 'employees', column: '', op: '=', value: '' }] };
    onChange = renderBuilder(withFilter);
    fireEvent.change(screen.getByLabelText('Filter 1 column'), { target: { value: 'employees.name' } });
    fireEvent.change(screen.getByLabelText('Filter 1 operator'), { target: { value: 'like' } });
    fireEvent.change(screen.getByLabelText('Filter 1 value'), { target: { value: 'Som%' } });
    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ where: [expect.objectContaining({ column: 'name', table: 'employees' })] }));
    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ where: [expect.objectContaining({ op: 'like' })] }));
    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ where: [expect.objectContaining({ value: 'Som%' })] }));
  });

  it('shows the combinator when there are 2+ filters and changes it', () => {
    const spec: QuerySpec = {
      ...emptySpec(),
      table: 'employees',
      where: [{ table: 'employees', column: 'id', op: '=', value: '1' }, { table: 'employees', column: 'name', op: '=', value: 'x' }],
    };
    const onChange = renderBuilder(spec);
    fireEvent.change(screen.getByLabelText('Combinator'), { target: { value: 'or' } });
    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ combinator: 'or' }));
  });

  it('removes a filter and sets the row limit', () => {
    const spec: QuerySpec = { ...emptySpec(), table: 'employees', where: [{ table: 'employees', column: 'id', op: '=', value: '1' }] };
    const onChange = renderBuilder(spec);
    fireEvent.click(screen.getByLabelText('Remove filter 1'));
    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ where: [] }));
    fireEvent.change(screen.getByLabelText('Row limit'), { target: { value: '42' } });
    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ limit: 42 }));
  });

  it('renders the read-only generated SQL preview', () => {
    const spec: QuerySpec = { ...emptySpec(), table: 'employees', columns: [{ table: 'employees', name: 'name' }] };
    renderBuilder(spec);
    expect(screen.getByTestId('sql-preview').textContent).toContain('SELECT "employees"."name" FROM "employees"');
  });
});
