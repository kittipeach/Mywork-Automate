import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { ReactNode } from 'react';
import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { useAuditLogs } from './audit';

function wrapper() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
}

const fetchMock = vi.fn();
beforeEach(() => {
  fetchMock.mockReset();
  globalThis.fetch = fetchMock as unknown as typeof fetch;
});
const ok = (b: unknown) => Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve(b) } as Response);

describe('useAuditLogs', () => {
  it('fetches /audit-logs with no params', async () => {
    fetchMock.mockReturnValueOnce(ok({ auditLogs: [] }));
    const { result } = renderHook(() => useAuditLogs(), { wrapper: wrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(fetchMock.mock.calls[0][0]).toBe('/api/automate/v1/audit-logs');
  });

  it('threads action + limit into the querystring', async () => {
    fetchMock.mockReturnValueOnce(ok({ auditLogs: [{ id: 1, role: 'admin', action: 'flow.publish', createdAt: '2026-07-15T00:00:00Z' }] }));
    const { result } = renderHook(() => useAuditLogs({ action: 'flow.publish', limit: 50 }), { wrapper: wrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    const url = fetchMock.mock.calls[0][0] as string;
    expect(url).toContain('action=flow.publish');
    expect(url).toContain('limit=50');
  });

  it('surfaces a 403 as an error (non-admin)', async () => {
    fetchMock.mockReturnValueOnce(Promise.resolve({ ok: false, status: 403 } as Response));
    const { result } = renderHook(() => useAuditLogs(), { wrapper: wrapper() });
    await waitFor(() => expect(result.current.isError).toBe(true));
  });
});
