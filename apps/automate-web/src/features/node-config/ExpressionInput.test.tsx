import { describe, it, expect, beforeEach, vi } from 'vitest';
import '@testing-library/jest-dom/vitest';
import { useState } from 'react';
import { render, screen, fireEvent, cleanup, within } from '@testing-library/react';
import { ExpressionInput } from './ExpressionInput';

beforeEach(() => cleanup());

/**
 * Type a value into the textarea with the caret at the end. jsdom resets
 * selectionStart to 0 on a bare change event, so we set it explicitly to mimic a
 * real user typing at the end of the field.
 */
function typeAtEnd(ta: HTMLTextAreaElement, value: string) {
  fireEvent.focus(ta); // opens the dropdown, like a real user clicking in
  fireEvent.change(ta, { target: { value } });
  // jsdom drops the caret to 0 during change; put it at the end and let the
  // component's keyUp handler (syncCaret) read it, as happens for a real user.
  ta.setSelectionRange(value.length, value.length);
  fireEvent.keyUp(ta);
}

// Controlled wrapper so the textarea reflects onChange like the real form does.
function Harness({
  initial = '',
  availableNodes = [],
  onChangeSpy,
}: {
  initial?: string;
  availableNodes?: string[];
  onChangeSpy?: (v: string) => void;
}) {
  const [v, setV] = useState(initial);
  return (
    <ExpressionInput
      id="expr"
      value={v}
      availableNodes={availableNodes}
      onChange={(nv) => {
        setV(nv);
        onChangeSpy?.(nv);
      }}
    />
  );
}

describe('ExpressionInput', () => {
  it('renders an accessible combobox textarea', () => {
    render(<Harness />);
    const ta = screen.getByRole('combobox');
    expect(ta.tagName).toBe('TEXTAREA');
    expect(ta).toHaveAttribute('aria-autocomplete', 'list');
  });

  it('opens the suggestion listbox on focus and offers root refs', () => {
    render(<Harness />);
    const ta = screen.getByRole('combobox');
    fireEvent.focus(ta);
    const listbox = screen.getByRole('listbox');
    const options = within(listbox).getAllByRole('option').map((o) => o.textContent);
    expect(options.join(' ')).toContain('$node');
    expect(options.join(' ')).toContain('$flow');
    expect(options.join(' ')).toContain('$vars');
  });

  it('inserts a suggestion on click (mousedown), replacing the partial', () => {
    const spy = vi.fn();
    render(<Harness initial="{{ $f" onChangeSpy={spy} />);
    const ta = screen.getByRole('combobox') as HTMLTextAreaElement;
    typeAtEnd(ta, '{{ $f');
    const flow = screen.getByRole('option', { name: /\$flow/ });
    fireEvent.mouseDown(flow);
    expect(spy).toHaveBeenLastCalledWith('{{ $flow.');
  });

  it('accepts the active suggestion with Enter', () => {
    const spy = vi.fn();
    render(<Harness initial="{{ $f" onChangeSpy={spy} />);
    const ta = screen.getByRole('combobox') as HTMLTextAreaElement;
    typeAtEnd(ta, '{{ $f');
    fireEvent.keyDown(ta, { key: 'Enter' });
    expect(spy).toHaveBeenLastCalledWith('{{ $flow.');
  });

  it('moves the active option with ArrowDown/ArrowUp', () => {
    render(<Harness initial="{{ " />);
    const ta = screen.getByRole('combobox') as HTMLTextAreaElement;
    typeAtEnd(ta, '{{ ');
    const first = () => screen.getAllByRole('option')[0];
    const second = () => screen.getAllByRole('option')[1];
    expect(first()).toHaveAttribute('aria-selected', 'true');
    fireEvent.keyDown(ta, { key: 'ArrowDown' });
    expect(second()).toHaveAttribute('aria-selected', 'true');
    fireEvent.keyDown(ta, { key: 'ArrowUp' });
    expect(first()).toHaveAttribute('aria-selected', 'true');
  });

  it('closes the dropdown on Escape', () => {
    render(<Harness initial="{{ " />);
    const ta = screen.getByRole('combobox') as HTMLTextAreaElement;
    typeAtEnd(ta, '{{ ');
    expect(screen.getByRole('listbox')).toBeInTheDocument();
    fireEvent.keyDown(ta, { key: 'Escape' });
    expect(screen.queryByRole('listbox')).not.toBeInTheDocument();
  });

  it('offers pipe functions after a bar', () => {
    render(<Harness initial="{{ x | " />);
    const ta = screen.getByRole('combobox') as HTMLTextAreaElement;
    typeAtEnd(ta, '{{ x | ');
    const options = screen.getAllByRole('option').map((o) => o.textContent);
    expect(options.join(' ')).toContain('format');
    expect(options.join(' ')).toContain('padLeft');
  });

  it('lists available upstream nodes plus $flow/$vars in the data picker', () => {
    render(<Harness availableNodes={['Query Headcount', 'If node']} />);
    const tree = screen.getByRole('tree');
    expect(within(tree).getByRole('button', { name: /Insert Query Headcount/ })).toBeInTheDocument();
    expect(within(tree).getByRole('button', { name: /Insert \$flow\.runDate/ })).toBeInTheDocument();
    expect(within(tree).getByRole('button', { name: /Insert \$vars/ })).toBeInTheDocument();
  });

  it('shows an empty-state when there is no upstream data', () => {
    render(<Harness availableNodes={[]} />);
    const tree = screen.getByRole('tree');
    // $flow + $vars are always present, so nodes-empty still has entries;
    // assert the flow/vars entries render and no node buttons appear.
    expect(within(tree).queryByRole('button', { name: /Insert .*Query/ })).not.toBeInTheDocument();
    expect(within(tree).getByRole('button', { name: /Insert \$vars/ })).toBeInTheDocument();
  });

  it('inserts a node reference from the data picker at the caret', () => {
    const spy = vi.fn();
    render(<Harness initial="" availableNodes={['Query']} onChangeSpy={spy} />);
    const btn = screen.getByRole('button', { name: /Insert Query/ });
    fireEvent.click(btn);
    expect(spy).toHaveBeenLastCalledWith('$node["Query"]');
  });
});
