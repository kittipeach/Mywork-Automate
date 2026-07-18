// In-memory mock store — stands in for the Go control-plane during UI dev.
// Persists for the lifetime of the dev server. Swap the api hooks to the real
// /api/automate/v1 (Go) later; the shapes match docs/spec/06.
import type { RunStatus } from '@/components/ui/Badge';

export type FlowSummary = {
  id: string;
  name: string;
  folder: string;
  status: RunStatus;
  updatedAt: string;
  lastRun?: { status: RunStatus; at: string };
  version: number;
  /** Soft-delete marker (E3-S6). Absent for live flows; ISO timestamp once deleted. */
  deletedAt?: string;
};

export type Execution = {
  id: string;
  flowId: string;
  flowName: string;
  status: RunStatus;
  trigger: 'schedule' | 'manual';
  startedAt: string;
  durationMs: number;
  version: number;
  steps: {
    nodeId: string;
    nodeName: string;
    nodeType: string;
    status: RunStatus;
    durationMs: number;
    inputCount: number;
    outputCount: number;
    error?: string;
  }[];
};

export type Connection = {
  id: string;
  name: string;
  type: 'postgres' | 'sftp' | 'smtp' | 'graph';
  host: string;
  // Postgres dial fields (E6-S1). The password is never on this shape — only
  // secretRef, the NAME of the secret that holds it.
  port?: number;
  database?: string;
  username?: string;
  sslMode?: string;
  secretRef?: string;
  status: 'ok' | 'untested' | 'error';
  allowedRoles: string[];
};

function iso(daysAgo: number, h = 6): string {
  // deterministic timestamps (no Date.now) for stable snapshots
  const base = new Date('2026-07-14T00:00:00+07:00').getTime();
  return new Date(base - daysAgo * 86400000 + h * 3600000).toISOString();
}

export const flows: FlowSummary[] = [
  { id: 'flw_payroll', name: 'Payroll → Bank MFT', folder: 'Finance', status: 'published', version: 3, updatedAt: iso(1), lastRun: { status: 'success', at: iso(0) } },
  { id: 'flw_headcount', name: 'Daily Headcount → Email', folder: 'HR Ops', status: 'published', version: 2, updatedAt: iso(2), lastRun: { status: 'success', at: iso(0) } },
  { id: 'flw_leave', name: 'Leave Balance Report', folder: 'HR Ops', status: 'paused', version: 1, updatedAt: iso(5), lastRun: { status: 'failed', at: iso(3) } },
  { id: 'flw_newhire', name: 'New Hire Onboarding Export', folder: 'HR Ops', status: 'draft', version: 0, updatedAt: iso(0) },
  { id: 'flw_gov', name: 'Gov Submission (TIS-620)', folder: 'Compliance', status: 'stopped', version: 4, updatedAt: iso(9), lastRun: { status: 'cancelled', at: iso(8) } },
  { id: 'flw_legacy', name: 'Legacy Import (deprecated)', folder: 'HR Ops', status: 'stopped', version: 2, updatedAt: iso(30), deletedAt: iso(12), lastRun: { status: 'success', at: iso(31) } },
];

export const executions: Execution[] = [
  {
    id: 'exe_1001', flowId: 'flw_payroll', flowName: 'Payroll → Bank MFT', status: 'success', trigger: 'schedule',
    startedAt: iso(0), durationMs: 48213, version: 3,
    steps: [
      { nodeId: 't1', nodeName: 'Schedule 06:00', nodeType: 'trigger.schedule', status: 'success', durationMs: 5, inputCount: 0, outputCount: 1 },
      { nodeId: 'q1', nodeName: 'Query Payroll', nodeType: 'db.query', status: 'success', durationMs: 2103, inputCount: 1, outputCount: 45012 },
      { nodeId: 'if1', nodeName: 'Has rows?', nodeType: 'logic.if', status: 'success', durationMs: 2, inputCount: 45012, outputCount: 45012 },
      { nodeId: 'f1', nodeName: 'Gen XLSX', nodeType: 'file.generate', status: 'success', durationMs: 41880, inputCount: 45012, outputCount: 1 },
      { nodeId: 'm1', nodeName: 'MFT to Bank', nodeType: 'delivery.mft', status: 'success', durationMs: 4120, inputCount: 1, outputCount: 1 },
    ],
  },
  {
    id: 'exe_1002', flowId: 'flw_leave', flowName: 'Leave Balance Report', status: 'failed', trigger: 'schedule',
    startedAt: iso(3), durationMs: 9210, version: 1,
    steps: [
      { nodeId: 't1', nodeName: 'Schedule 07:00', nodeType: 'trigger.schedule', status: 'success', durationMs: 4, inputCount: 0, outputCount: 1 },
      { nodeId: 'q1', nodeName: 'Query Leave', nodeType: 'db.query', status: 'failed', durationMs: 9200, inputCount: 1, outputCount: 0, error: 'connection timeout to hr-db' },
    ],
  },
  {
    id: 'exe_1003', flowId: 'flw_headcount', flowName: 'Daily Headcount → Email', status: 'running', trigger: 'manual',
    startedAt: iso(0, 9), durationMs: 0, version: 2,
    steps: [
      { nodeId: 't1', nodeName: 'Manual', nodeType: 'trigger.manual', status: 'success', durationMs: 2, inputCount: 0, outputCount: 1 },
      { nodeId: 'q1', nodeName: 'Query Headcount', nodeType: 'db.query', status: 'running', durationMs: 0, inputCount: 1, outputCount: 0 },
    ],
  },
];

export type Grant = {
  id: string;
  flowId: string;
  subjectType: 'role' | 'user';
  subjectId: string;
  access: 'viewer' | 'editor' | 'owner';
};

export const grants: Grant[] = [
  { id: 'grn_1', flowId: 'flw_payroll', subjectType: 'role', subjectId: 'operator', access: 'viewer' },
  { id: 'grn_2', flowId: 'flw_payroll', subjectType: 'user', subjectId: 'jane@mywork.co', access: 'editor' },
];

export type DirectoryUser = {
  id: string;
  email: string;
  displayName: string;
  roles: string[];
};

export const users: DirectoryUser[] = [
  { id: 'usr_admin', email: 'admin@mywork.co', displayName: 'Ada Admin', roles: ['admin'] },
  { id: 'usr_designer', email: 'dan@mywork.co', displayName: 'Dan Designer', roles: ['designer'] },
  { id: 'usr_op', email: 'olive@mywork.co', displayName: 'Olive Operator', roles: ['operator'] },
  { id: 'usr_multi', email: 'mia@mywork.co', displayName: 'Mia Multi', roles: ['designer', 'operator'] },
];

export const connections: Connection[] = [
  { id: 'conn_hr', name: 'HR Postgres (read-only)', type: 'postgres', host: 'hr-db.internal:5432', status: 'ok', allowedRoles: ['admin', 'designer'] },
  { id: 'conn_pay', name: 'Payroll Postgres', type: 'postgres', host: 'pay-db.internal:5432', status: 'ok', allowedRoles: ['admin'] },
  { id: 'conn_bankmft', name: 'Bank MFT', type: 'sftp', host: 'mft.bank.co.th:22', status: 'ok', allowedRoles: ['admin', 'designer'] },
  { id: 'conn_smtp', name: 'Corp SMTP', type: 'smtp', host: 'smtp.mywork.co:587', status: 'untested', allowedRoles: ['admin', 'designer', 'operator'] },
];
