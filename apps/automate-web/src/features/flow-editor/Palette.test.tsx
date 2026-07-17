import { describe, it, expect, vi, beforeEach } from 'vitest';
import '@testing-library/jest-dom/vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/react';
import { Palette, DND_MIME } from './Palette';

beforeEach(() => cleanup());

describe('Palette', () => {
  it('renders nodes grouped by category', () => {
    render(<Palette onAdd={() => {}} />);
    expect(screen.getByText('Trigger')).toBeInTheDocument();
    expect(screen.getByText('Data')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Add Schedule node/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Add DB Query node/i })).toBeInTheDocument();
  });

  it('filters by the search box', () => {
    render(<Palette onAdd={() => {}} />);
    fireEvent.change(screen.getByLabelText(/Search nodes/i), { target: { value: 'sftp' } });
    // MFT/SFTP matches on description
    expect(screen.getByRole('button', { name: /Add MFT \/ SFTP node/i })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Add Schedule node/i })).not.toBeInTheDocument();
  });

  it('shows an empty state when nothing matches', () => {
    render(<Palette onAdd={() => {}} />);
    fireEvent.change(screen.getByLabelText(/Search nodes/i), { target: { value: 'zzzz' } });
    expect(screen.getByText(/No nodes match/i)).toBeInTheDocument();
  });

  it('calls onAdd with the node type on click', () => {
    const onAdd = vi.fn();
    render(<Palette onAdd={onAdd} />);
    fireEvent.click(screen.getByRole('button', { name: /Add DB Query node/i }));
    expect(onAdd).toHaveBeenCalledWith('db.query');
  });

  it('sets the node type on dragStart dataTransfer', () => {
    render(<Palette onAdd={() => {}} />);
    const setData = vi.fn();
    const item = screen.getByRole('button', { name: /Add Schedule node/i });
    fireEvent.dragStart(item, {
      dataTransfer: { setData, effectAllowed: '' },
    });
    expect(setData).toHaveBeenCalledWith(DND_MIME, 'trigger.schedule');
  });
});
