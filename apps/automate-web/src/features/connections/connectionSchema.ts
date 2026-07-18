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

/** SSL modes offered for a Postgres connection (libpq semantics). */
export const SSL_MODES = ['disable', 'require', 'verify-ca', 'verify-full'] as const;

export const connectionFormSchema = z
  .object({
    name: z.string().trim().min(1, 'Name is required'),
    type: z.enum(['postgres', 'sftp', 'smtp', 'graph']),
    host: z.string().trim().min(1, 'Host is required'),
    // Postgres dial fields. Optional at the type level; required for postgres via
    // the refinement below. port is coerced so the number input's string value
    // parses; 0 means "unset" (the backend defaults it to 5432).
    port: z.coerce.number().int().min(0).max(65535).optional(),
    database: z.string().trim().optional(),
    username: z.string().trim().optional(),
    sslMode: z.enum(SSL_MODES).optional(),
    // password is write-only (dev): saved to the secret store, never read back.
    // secretRef names an existing secret (prod path). Either is optional.
    password: z.string().optional(),
    secretRef: z.string().trim().optional(),
    allowedRoles: z.array(z.string()).min(1, 'Pick at least one role'),
  })
  .superRefine((v, ctx) => {
    if (v.type !== 'postgres') return;
    if (!v.database || v.database.trim() === '') {
      ctx.addIssue({ code: z.ZodIssueCode.custom, path: ['database'], message: 'Database is required for Postgres' });
    }
    if (!v.username || v.username.trim() === '') {
      ctx.addIssue({ code: z.ZodIssueCode.custom, path: ['username'], message: 'Username is required for Postgres' });
    }
  });

export type ConnectionFormValues = z.infer<typeof connectionFormSchema>;

export const emptyConnectionForm: ConnectionFormValues = {
  name: '',
  type: 'postgres',
  host: '',
  port: 5432,
  database: '',
  username: '',
  sslMode: 'disable',
  password: '',
  secretRef: '',
  allowedRoles: ['admin'],
};
