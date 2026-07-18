import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor, cleanup, within } from '@testing-library/react';
import { ConnectionFormModal } from './ConnectionFormModal';
import type { Connection } from '@/lib/mock/store';

// --- mock the mutation hooks so we assert on the calls, not the network ---
const createMutate = vi.fn();
const updateMutate = vi.fn();
const deleteMutate = vi.fn();
const testMutate = vi.fn();
const testReset = vi.fn();
let testData: { status: string; message?: string } | undefined;

vi.mock('@/api/connections', () => ({
  useCreateConnection: () => ({ mutate: createMutate, isPending: false }),
  useUpdateConnection: () => ({ mutate: updateMutate, isPending: false }),
  useDeleteConnection: () => ({ mutate: deleteMutate, isPending: false }),
  useTestConnection: () => ({ mutate: testMutate, isPending: false, reset: testReset, data: testData }),
}));

/** Fill the Postgres-only dial fields so the form validates. */
function fillPostgres(scope = screen) {
  fireEvent.change(scope.getByLabelText('Database'), { target: { value: 'hr' } });
  fireEvent.change(scope.getByLabelText('Username'), { target: { value: 'reader' } });
}

const toastFn = vi.fn();
vi.mock('@/components/ui/Toast', () => ({
  useToast: () => ({ toast: toastFn }),
}));

const conn: Connection = {
  id: 'conn_1',
  name: 'HR DB',
  type: 'postgres',
  host: 'hr-db:5432',
  database: 'hr',
  username: 'reader',
  port: 5432,
  sslMode: 'disable',
  status: 'ok',
  allowedRoles: ['admin'],
};

beforeEach(() => {
  cleanup();
  createMutate.mockReset();
  updateMutate.mockReset();
  deleteMutate.mockReset();
  testMutate.mockReset();
  testReset.mockReset();
  toastFn.mockReset();
  testData = undefined;
});

describe('ConnectionFormModal — create mode', () => {
  it('renders nothing when closed', () => {
    const { container } = render(<ConnectionFormModal open={false} onClose={() => {}} />);
    expect(container).toBeEmptyDOMElement();
  });

  it('shows the create title and no Test/Delete buttons', () => {
    render(<ConnectionFormModal open onClose={() => {}} />);
    expect(screen.getByRole('dialog')).toHaveTextContent('New connection');
    expect(screen.queryByRole('button', { name: /^Test$/ })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Delete/ })).not.toBeInTheDocument();
  });

  it('validates required fields before submitting', async () => {
    render(<ConnectionFormModal open onClose={() => {}} />);
    // wipe the pre-filled name so validation trips
    fireEvent.change(screen.getByLabelText('Name'), { target: { value: '' } });
    fireEvent.click(screen.getByRole('button', { name: /Create/ }));
    await waitFor(() => expect(screen.getByText('Name is required')).toBeInTheDocument());
    expect(createMutate).not.toHaveBeenCalled();
  });

  it('creates on valid submit and passes the form values', async () => {
    createMutate.mockImplementation((_v, opts) => opts?.onSuccess?.());
    const onClose = vi.fn();
    render(<ConnectionFormModal open onClose={onClose} />);

    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'New SFTP' } });
    fireEvent.change(screen.getByLabelText('Type'), { target: { value: 'sftp' } });
    fireEvent.change(screen.getByLabelText('Host'), { target: { value: 'sftp:22' } });
    fireEvent.click(screen.getByRole('button', { name: /Create/ }));

    await waitFor(() => expect(createMutate).toHaveBeenCalled());
    // sftp has no DB dial fields; assert the meaningful fields (the form also
    // carries defaulted postgres fields, which the backend ignores for sftp).
    expect(createMutate.mock.calls[0][0]).toMatchObject({
      name: 'New SFTP',
      type: 'sftp',
      host: 'sftp:22',
      allowedRoles: ['admin'],
    });
    expect(toastFn).toHaveBeenCalledWith('success', 'Connection created');
    expect(onClose).toHaveBeenCalled();
  });

  it('submits the Postgres dial fields + password', async () => {
    createMutate.mockImplementation((_v, opts) => opts?.onSuccess?.());
    render(<ConnectionFormModal open onClose={() => {}} />);
    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'HR' } });
    fireEvent.change(screen.getByLabelText('Host'), { target: { value: 'hr-db' } });
    fireEvent.change(screen.getByLabelText('Port'), { target: { value: '6000' } });
    fillPostgres();
    fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'p@ss' } });
    fireEvent.click(screen.getByRole('button', { name: /Create/ }));

    await waitFor(() => expect(createMutate).toHaveBeenCalled());
    expect(createMutate.mock.calls[0][0]).toMatchObject({
      name: 'HR', type: 'postgres', host: 'hr-db', port: 6000,
      database: 'hr', username: 'reader', password: 'p@ss',
    });
  });

  it('toasts an error when create fails', async () => {
    createMutate.mockImplementation((_v, opts) => opts?.onError?.());
    render(<ConnectionFormModal open onClose={() => {}} />);
    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'X' } });
    fireEvent.change(screen.getByLabelText('Host'), { target: { value: 'h' } });
    fillPostgres();
    fireEvent.click(screen.getByRole('button', { name: /Create/ }));
    await waitFor(() => expect(toastFn).toHaveBeenCalledWith('error', 'Could not create connection'));
  });

  it('toggles a role in the multi-select', async () => {
    createMutate.mockImplementation((_v, opts) => opts?.onSuccess?.());
    render(<ConnectionFormModal open onClose={() => {}} />);
    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'X' } });
    fireEvent.change(screen.getByLabelText('Host'), { target: { value: 'h' } });
    fillPostgres();

    const group = screen.getByRole('group', { name: /Allowed roles/i });
    fireEvent.click(within(group).getByText('designer')); // add
    fireEvent.click(within(group).getByText('admin')); // remove the default
    fireEvent.click(screen.getByRole('button', { name: /Create/ }));

    await waitFor(() => expect(createMutate).toHaveBeenCalled());
    expect(createMutate.mock.calls[0][0].allowedRoles).toEqual(['designer']);
  });
});

describe('ConnectionFormModal — edit mode', () => {
  it('pre-fills the form and shows Test + Delete', () => {
    render(<ConnectionFormModal open onClose={() => {}} connection={conn} />);
    expect(screen.getByRole('dialog')).toHaveTextContent('Edit connection');
    expect(screen.getByLabelText('Name')).toHaveValue('HR DB');
    expect(screen.getByLabelText('Host')).toHaveValue('hr-db:5432');
    expect(screen.getByRole('button', { name: /Test/ })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Delete/ })).toBeInTheDocument();
  });

  it('updates with the id on save', async () => {
    updateMutate.mockImplementation((_v, opts) => opts?.onSuccess?.());
    const onClose = vi.fn();
    render(<ConnectionFormModal open onClose={onClose} connection={conn} />);
    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'HR DB v2' } });
    fireEvent.click(screen.getByRole('button', { name: /Save changes/ }));

    await waitFor(() => expect(updateMutate).toHaveBeenCalled());
    expect(updateMutate.mock.calls[0][0]).toMatchObject({ id: 'conn_1', name: 'HR DB v2' });
    expect(toastFn).toHaveBeenCalledWith('success', 'Connection updated');
    expect(onClose).toHaveBeenCalled();
  });

  it('toasts an error when update fails', async () => {
    updateMutate.mockImplementation((_v, opts) => opts?.onError?.());
    render(<ConnectionFormModal open onClose={() => {}} connection={conn} />);
    fireEvent.click(screen.getByRole('button', { name: /Save changes/ }));
    await waitFor(() => expect(toastFn).toHaveBeenCalledWith('error', 'Could not save connection'));
  });

  it('tests the connection and toasts ok/fail', async () => {
    // success result → toast success
    testMutate.mockImplementationOnce((_id, opts) => opts?.onSuccess?.({ status: 'ok' }));
    render(<ConnectionFormModal open onClose={() => {}} connection={conn} />);
    fireEvent.click(screen.getByRole('button', { name: /Test/ }));
    expect(testMutate).toHaveBeenCalledWith('conn_1', expect.anything());
    await waitFor(() => expect(toastFn).toHaveBeenCalledWith('success', 'Connection OK'));

    // reachable server, unreachable target → status:"error" with a message → toast error
    testMutate.mockImplementationOnce((_id, opts) =>
      opts?.onSuccess?.({ status: 'error', message: 'could not connect: refused' }));
    fireEvent.click(screen.getByRole('button', { name: /Test/ }));
    await waitFor(() => expect(toastFn).toHaveBeenCalledWith('error', 'could not connect: refused'));
  });

  it('renders the live-probe result banner', () => {
    testData = { status: 'error', message: 'could not connect: refused' };
    render(<ConnectionFormModal open onClose={() => {}} connection={conn} />);
    const banner = screen.getByRole('status');
    expect(banner).toHaveTextContent('could not connect: refused');
  });

  it('requires confirmation before deleting', async () => {
    deleteMutate.mockImplementation((_id, opts) => opts?.onSuccess?.());
    const onClose = vi.fn();
    render(<ConnectionFormModal open onClose={onClose} connection={conn} />);

    fireEvent.click(screen.getByRole('button', { name: /Delete/ }));
    expect(screen.getByText('Delete this connection?')).toBeInTheDocument();
    // back out first
    fireEvent.click(screen.getByRole('button', { name: /Keep/ }));
    expect(screen.queryByText('Delete this connection?')).not.toBeInTheDocument();
    // then confirm
    fireEvent.click(screen.getByRole('button', { name: /Delete/ }));
    fireEvent.click(screen.getByRole('button', { name: /Confirm/ }));

    await waitFor(() => expect(deleteMutate).toHaveBeenCalledWith('conn_1', expect.anything()));
    expect(toastFn).toHaveBeenCalledWith('success', 'Connection deleted');
    expect(onClose).toHaveBeenCalled();
  });

  it('toasts an error when delete fails', async () => {
    deleteMutate.mockImplementation((_id, opts) => opts?.onError?.());
    render(<ConnectionFormModal open onClose={() => {}} connection={conn} />);
    fireEvent.click(screen.getByRole('button', { name: /Delete/ }));
    fireEvent.click(screen.getByRole('button', { name: /Confirm/ }));
    await waitFor(() => expect(toastFn).toHaveBeenCalledWith('error', 'Could not delete connection'));
  });

  it('closes via Cancel', () => {
    const onClose = vi.fn();
    render(<ConnectionFormModal open onClose={onClose} connection={conn} />);
    fireEvent.click(screen.getByRole('button', { name: /^Cancel$/ }));
    expect(onClose).toHaveBeenCalled();
  });
});
