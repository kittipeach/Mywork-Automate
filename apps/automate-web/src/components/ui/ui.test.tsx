import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { Button } from './Button';
import { StatusBadge, Badge, type RunStatus } from './Badge';
import { Card, CardHeader, CardBody } from './Card';

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
