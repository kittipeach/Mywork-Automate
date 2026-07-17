import { describe, it, expect, vi, beforeEach } from 'vitest';
import '@testing-library/jest-dom/vitest';
import { render, screen, fireEvent, cleanup, waitFor } from '@testing-library/react';
import { NODE_BY_TYPE, type JSONSchema } from '@/lib/nodeRegistry';
import { SchemaForm } from './SchemaForm';

const scheduleSchema = NODE_BY_TYPE['trigger.schedule'].schema;
const fileSchema = NODE_BY_TYPE['file.generate'].schema;
const dbSchema = NODE_BY_TYPE['db.query'].schema;
const emailSchema = NODE_BY_TYPE['delivery.email'].schema;
const downloadSchema = NODE_BY_TYPE['delivery.download'].schema;
const manualSchema = NODE_BY_TYPE['trigger.manual'].schema;

beforeEach(() => cleanup());

describe('SchemaForm — render per type', () => {
  it('renders a text input with a linked label (htmlFor)', () => {
    render(<SchemaForm schema={dbSchema} formId="db" />);
    const input = screen.getByLabelText(/Connection/i);
    expect(input).toBeInTheDocument();
    expect(input.tagName).toBe('INPUT');
    expect(input).toHaveAttribute('id');
  });

  it('renders an enum as a select using enumLabels', () => {
    render(<SchemaForm schema={dbSchema} formId="db" />);
    const select = screen.getByLabelText(/Query mode/i) as HTMLSelectElement;
    expect(select.tagName).toBe('SELECT');
    // enumLabels: 'Visual builder' / 'SQL'
    expect(screen.getByRole('option', { name: 'Visual builder' })).toBeInTheDocument();
    expect(screen.getByRole('option', { name: 'SQL' })).toBeInTheDocument();
  });

  it('renders a number input with min/max', () => {
    render(<SchemaForm schema={dbSchema} formId="db" />);
    const input = screen.getByLabelText(/Max rows/i) as HTMLInputElement;
    expect(input.type).toBe('number');
    expect(input).toHaveAttribute('min', '1');
    expect(input).toHaveAttribute('max', '100000');
  });

  it('renders a boolean as a checkbox', () => {
    render(<SchemaForm schema={downloadSchema} formId="dl" />);
    const cb = screen.getByLabelText(/Notify users/i) as HTMLInputElement;
    expect(cb.type).toBe('checkbox');
  });

  it('renders a sql format field as a monospace textarea with SQL affordance', () => {
    render(<SchemaForm schema={dbSchema} formId="db" />);
    const ta = screen.getByLabelText(/SQL \(SELECT only\)/i) as HTMLTextAreaElement;
    expect(ta.tagName).toBe('TEXTAREA');
    expect(ta.className).toContain('font-mono');
    // affordance badge: a span (not the enum <option>) carrying 'SQL'
    const badge = screen
      .getAllByText('SQL')
      .find((el) => el.tagName === 'SPAN' && el.className.includes('font-mono'));
    expect(badge).toBeTruthy();
  });

  it('renders an expression format field with the ƒx affordance', () => {
    render(<SchemaForm schema={emailSchema} formId="em" />);
    // 'to' and 'subject' are expression fields
    expect(screen.getAllByText('ƒx').length).toBeGreaterThan(0);
  });

  it('renders a plain textarea for format:textarea (email body)', () => {
    render(<SchemaForm schema={emailSchema} formId="em" />);
    const body = screen.getByLabelText(/Body/i) as HTMLTextAreaElement;
    expect(body.tagName).toBe('TEXTAREA');
    expect(body.className).not.toContain('font-mono');
  });

  it('shows an empty-state message when the schema has no fields', () => {
    render(<SchemaForm schema={manualSchema} formId="man" />);
    expect(screen.getByText(/no configuration/i)).toBeInTheDocument();
  });

  it('marks required fields with an asterisk', () => {
    render(<SchemaForm schema={dbSchema} formId="db" />);
    // Connection is required
    const label = screen.getByText('Connection').closest('label');
    expect(label?.textContent).toContain('*');
  });
});

describe('SchemaForm — validation + validity callback', () => {
  it('reports invalid on mount when required fields are empty, then valid once filled', async () => {
    const onValidity = vi.fn();
    render(<SchemaForm schema={dbSchema} formId="db" onValidityChange={onValidity} />);
    // db.query requires connectionId (empty by default) → invalid
    await waitFor(() => expect(onValidity).toHaveBeenCalledWith(false));

    fireEvent.change(screen.getByLabelText(/Connection/i), { target: { value: 'conn_hr' } });
    await waitFor(() => expect(onValidity).toHaveBeenLastCalledWith(true));
  });

  it('shows an inline error message for an out-of-bounds number', async () => {
    render(<SchemaForm schema={dbSchema} formId="db" />);
    const maxRows = screen.getByLabelText(/Max rows/i);
    fireEvent.change(maxRows, { target: { value: '0' } });
    await waitFor(() => {
      expect(screen.getByText(/Must be ≥ 1/)).toBeInTheDocument();
    });
    expect(maxRows).toHaveAttribute('aria-invalid', 'true');
  });

  it('emits value changes through onChange', async () => {
    const onChange = vi.fn();
    render(<SchemaForm schema={dbSchema} formId="db" onChange={onChange} />);
    fireEvent.change(screen.getByLabelText(/Connection/i), { target: { value: 'abc' } });
    await waitFor(() => {
      const last = onChange.mock.calls.at(-1)?.[0];
      expect(last.connectionId).toBe('abc');
    });
  });
});

describe('SchemaForm — conditional (dependentSchemas) fields', () => {
  it('shows everyMinutes for simple mode and hides cron', () => {
    render(<SchemaForm schema={scheduleSchema} formId="sched" />);
    expect(screen.getByLabelText(/Every \(minutes\)/i)).toBeInTheDocument();
    expect(screen.queryByLabelText(/Cron expression/i)).not.toBeInTheDocument();
  });

  it('swaps everyMinutes → cron when mode changes to cron', async () => {
    render(<SchemaForm schema={scheduleSchema} formId="sched" />);
    const mode = screen.getByRole('combobox', { name: /Mode/i });
    fireEvent.change(mode, { target: { value: 'cron' } });
    await waitFor(() => {
      expect(screen.getByLabelText(/Cron expression/i)).toBeInTheDocument();
    });
    expect(screen.queryByLabelText(/Every \(minutes\)/i)).not.toBeInTheDocument();
  });

  it('marks conditional fields with a "conditional" tag', () => {
    render(<SchemaForm schema={scheduleSchema} formId="sched" />);
    expect(screen.getAllByText(/conditional/i).length).toBeGreaterThan(0);
  });

  it('reveals delimiter only when file format = txt', async () => {
    render(<SchemaForm schema={fileSchema} formId="file" />);
    expect(screen.queryByLabelText(/Delimiter/i)).not.toBeInTheDocument();
    fireEvent.change(screen.getByRole('combobox', { name: /Format/i }), { target: { value: 'txt' } });
    await waitFor(() => {
      expect(screen.getByLabelText(/Delimiter/i)).toBeInTheDocument();
    });
  });

  it('re-initialises when formId (node) changes', () => {
    const { rerender } = render(<SchemaForm schema={dbSchema} formId="node-a" value={{ connectionId: 'a' }} />);
    expect((screen.getByLabelText(/Connection/i) as HTMLInputElement).value).toBe('a');
    rerender(<SchemaForm schema={dbSchema} formId="node-b" value={{ connectionId: 'b' }} />);
    expect((screen.getByLabelText(/Connection/i) as HTMLInputElement).value).toBe('b');
  });
});

describe('SchemaForm — password + accessibility', () => {
  it('renders a password input for format:password', () => {
    const schema: JSONSchema = {
      type: 'object',
      properties: { secret: { type: 'string', title: 'Secret', format: 'password' } },
      required: ['secret'],
    };
    render(<SchemaForm schema={schema} formId="pw" />);
    const input = screen.getByLabelText(/Secret/i) as HTMLInputElement;
    expect(input.type).toBe('password');
  });

  it('links errors via aria-describedby', async () => {
    render(<SchemaForm schema={dbSchema} formId="db" />);
    const conn = screen.getByLabelText(/Connection/i);
    fireEvent.change(conn, { target: { value: 'x' } });
    fireEvent.change(conn, { target: { value: '' } });
    await waitFor(() => {
      expect(conn).toHaveAttribute('aria-describedby');
    });
  });
});
