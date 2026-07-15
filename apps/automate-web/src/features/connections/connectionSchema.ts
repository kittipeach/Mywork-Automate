// Zod schema + role catalog for the connection create/edit form (E6-S1). Kept
// separate from the presentational modal so the validation contract is unit-
// tested (src/features is gated ≥90%).
import { z } from 'zod';
import type { ConnectionType } from '@/api/connections';

export const CONNECTION_TYPES: { value: ConnectionType; label: string }[] = [
  { value: 'postgres', label: 'Postgres' },
  { value: 'sftp', label: 'SFTP' },
  { value: 'smtp', label: 'SMTP' },
  { value: 'graph', label: 'MS Graph' },
];

/** Roles selectable in the allowed-roles multi-select. */
export const ROLES = ['admin', 'designer', 'operator', 'viewer'] as const;

export const connectionFormSchema = z.object({
  name: z.string().trim().min(1, 'Name is required'),
  type: z.enum(['postgres', 'sftp', 'smtp', 'graph']),
  host: z.string().trim().min(1, 'Host is required'),
  allowedRoles: z.array(z.string()).min(1, 'Pick at least one role'),
});

export type ConnectionFormValues = z.infer<typeof connectionFormSchema>;

export const emptyConnectionForm: ConnectionFormValues = {
  name: '',
  type: 'postgres',
  host: '',
  allowedRoles: ['admin'],
};
