import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent, cleanup, act } from '@testing-library/react';
import { Button } from './Button';
import { StatusBadge, Badge, type RunStatus } from './Badge';
import { Card, CardHeader, CardBody } from './Card';
import { Modal } from './Modal';
import { ToastProvider, useToast } from './Toast';

describe('Button', () => {
  it('renders children and handles click', () => {
    const onClick = vi.fn();
    render(<Button onClick={onClick}>Save</Button>);
    fireEvent.click(screen.getByRole('button', { name: 'Save' }));
    expect(onClick).toHaveBeenCalledOnce();
  });

  it.each(['primary', 'secondary', 'ghost', 'danger'] as const)('renders %s variant', (variant) => {
    render(<Button variant={variant}>x</Button>);
    expect(screen.getByRole('button')).toBeInTheDocument();
  });

  it('respects disabled', () => {
    render(<Button disabled>x</Button>);
    expect(screen.getByRole('button')).toBeDisabled();
  });

  it('applies small size', () => {
    render(<Button size="sm">x</Button>);
    expect(screen.getByRole('button').className).toContain('h-8');
  });
});

describe('StatusBadge', () => {
  const statuses: RunStatus[] = ['draft', 'published', 'paused', 'stopped', 'queued', 'running', 'success', 'failed', 'cancelled', 'skipped'];
  it.each(statuses)('renders %s', (status) => {
    render(<StatusBadge status={status} />);
    expect(screen.getByText(status)).toBeInTheDocument();
  });

  it('falls back for an unknown status without throwing', () => {
    render(<StatusBadge status={'weird' as RunStatus} />);
    expect(screen.getByText('weird')).toBeInTheDocument();
  });
});

describe('Badge', () => {
  it('renders content', () => {
    render(<Badge>admin</Badge>);
    expect(screen.getByText('admin')).toBeInTheDocument();
  });
});

describe('Card', () => {
  it('renders header (title/subtitle/actions) and body', () => {
    render(
      <Card>
        <CardHeader title="T" subtitle="S" actions={<button>A</button>} />
        <CardBody>Body</CardBody>
      </Card>,
    );
    expect(screen.getByText('T')).toBeInTheDocument();
    expect(screen.getByText('S')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'A' })).toBeInTheDocument();
    expect(screen.getByText('Body')).toBeInTheDocument();
  });

  it('renders header without optional subtitle/actions', () => {
    render(<CardHeader title="OnlyTitle" />);
    expect(screen.getByText('OnlyTitle')).toBeInTheDocument();
  });
});

describe('Modal', () => {
  it('renders nothing when closed', () => {
    const { container } = render(
      <Modal open={false} onClose={() => {}} title="T">
        body
      </Modal>,
    );
    expect(container).toBeEmptyDOMElement();
  });

  it('renders title, body, footer and closes via the close button', () => {
    cleanup();
    const onClose = vi.fn();
    render(
      <Modal open onClose={onClose} title="My dialog" footer={<button>Save</button>}>
        <p>hello</p>
      </Modal>,
    );
    expect(screen.getByRole('dialog')).toHaveAttribute('aria-label', 'My dialog');
    expect(screen.getByText('hello')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Save' })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: /Close dialog/ }));
    expect(onClose).toHaveBeenCalled();
  });

  it('closes on backdrop click but not on content click', () => {
    cleanup();
    const onClose = vi.fn();
    render(
      <Modal open onClose={onClose} title="T">
        <p>content</p>
      </Modal>,
    );
    fireEvent.click(screen.getByText('content'));
    expect(onClose).not.toHaveBeenCalled();
    fireEvent.click(screen.getByTestId('modal-backdrop'));
    expect(onClose).toHaveBeenCalled();
  });

  it('omits aria-label when the title is a non-string node', () => {
    cleanup();
    render(
      <Modal open onClose={() => {}} title={<span>Rich title</span>}>
        body
      </Modal>,
    );
    const dialog = screen.getByRole('dialog');
    expect(dialog).not.toHaveAttribute('aria-label');
    expect(screen.getByText('Rich title')).toBeInTheDocument();
  });

  it('closes on Escape and ignores other keys', () => {
    cleanup();
    const onClose = vi.fn();
    render(
      <Modal open onClose={onClose} title="T">
        body
      </Modal>,
    );
    fireEvent.keyDown(window, { key: 'Enter' });
    expect(onClose).not.toHaveBeenCalled();
    fireEvent.keyDown(window, { key: 'Escape' });
    expect(onClose).toHaveBeenCalled();
  });
});

describe('Toast', () => {
  function ToastHarnessButtons() {
    const { toast } = useToast();
    return (
      <>
        <button onClick={() => toast('success', 'Saved!')}>ok</button>
        <button onClick={() => toast('error', 'Boom')}>err</button>
      </>
    );
  }

  it('pushes success + error toasts and dismisses one', () => {
    cleanup();
    render(
      <ToastProvider>
        <ToastHarnessButtons />
      </ToastProvider>,
    );
    // no toasts initially
    expect(screen.queryByRole('region', { name: /Notifications/ })).not.toBeInTheDocument();

    act(() => {
      fireEvent.click(screen.getByRole('button', { name: 'ok' }));
      fireEvent.click(screen.getByRole('button', { name: 'err' }));
    });
    expect(screen.getByText('Saved!')).toBeInTheDocument();
    expect(screen.getByText('Boom')).toBeInTheDocument();

    act(() => {
      fireEvent.click(screen.getAllByRole('button', { name: /Dismiss notification/ })[0]);
    });
    expect(screen.queryByText('Saved!')).not.toBeInTheDocument();
    expect(screen.getByText('Boom')).toBeInTheDocument();
  });

  it('throws when useToast is used outside a provider', () => {
    const Bad = () => {
      useToast();
      return null;
    };
    const spy = vi.spyOn(console, 'error').mockImplementation(() => {});
    expect(() => render(<Bad />)).toThrow(/ToastProvider/);
    spy.mockRestore();
  });
});
