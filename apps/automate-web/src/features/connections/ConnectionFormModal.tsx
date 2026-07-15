'use client';

// Create/edit connection modal (E6-S1). react-hook-form + zod. Wires Test
// (probe an already-saved connection), Save (create or update), and Delete
// (with confirm) through the connection mutation hooks; invalidation refreshes
// the list. Purely presentational logic lives in connectionSchema.ts (tested).
import { useEffect, useState } from 'react';
import { useForm, Controller } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { Loader2, PlugZap, Trash2 } from 'lucide-react';
import { Modal } from '@/components/ui/Modal';
import { Button } from '@/components/ui/Button';
import { useToast } from '@/components/ui/Toast';
import { cn } from '@/lib/cn';
import type { Connection } from '@/lib/mock/store';
import {
  useCreateConnection,
  useUpdateConnection,
  useDeleteConnection,
  useTestConnection,
} from '@/api/connections';
import {
  connectionFormSchema,
  emptyConnectionForm,
  CONNECTION_TYPES,
  ROLES,
  type ConnectionFormValues,
} from './connectionSchema';

export type ConnectionFormModalProps = {
  open: boolean;
  onClose: () => void;
  /** Present → edit mode; absent → create mode. */
  connection?: Connection | null;
};

function toFormValues(c?: Connection | null): ConnectionFormValues {
  if (!c) return emptyConnectionForm;
  return { name: c.name, type: c.type, host: c.host, allowedRoles: c.allowedRoles };
}

export function ConnectionFormModal({ open, onClose, connection }: ConnectionFormModalProps) {
  const isEdit = !!connection;
  const { toast } = useToast();
  const {
    register,
    control,
    handleSubmit,
    reset,
    formState: { errors },
  } = useForm<ConnectionFormValues>({
    resolver: zodResolver(connectionFormSchema),
    defaultValues: toFormValues(connection),
  });

  const create = useCreateConnection();
  const update = useUpdateConnection();
  const del = useDeleteConnection();
  const test = useTestConnection();

  const [confirmDelete, setConfirmDelete] = useState(false);

  // Re-seed when the target connection changes / the modal (re)opens.
  useEffect(() => {
    if (open) {
      reset(toFormValues(connection));
      setConfirmDelete(false);
      test.reset();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, connection]);

  const onSubmit = handleSubmit((values) => {
    if (isEdit && connection) {
      update.mutate(
        { id: connection.id, ...values },
        {
          onSuccess: () => {
            toast('success', 'Connection updated');
            onClose();
          },
          onError: () => toast('error', 'Could not save connection'),
        },
      );
    } else {
      create.mutate(values, {
        onSuccess: () => {
          toast('success', 'Connection created');
          onClose();
        },
        onError: () => toast('error', 'Could not create connection'),
      });
    }
  });

  const onTest = () => {
    if (!connection) return;
    test.mutate(connection.id, {
      onSuccess: () => toast('success', 'Connection OK'),
      onError: () => toast('error', 'Connection test failed'),
    });
  };

  const onDelete = () => {
    if (!connection) return;
    del.mutate(connection.id, {
      onSuccess: () => {
        toast('success', 'Connection deleted');
        onClose();
      },
      onError: () => toast('error', 'Could not delete connection'),
    });
  };

  const saving = create.isPending || update.isPending;

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={isEdit ? 'Edit connection' : 'New connection'}
      footer={
        <>
          {isEdit &&
            (confirmDelete ? (
              <div className="mr-auto flex items-center gap-2">
                <span className="text-sm text-ink-muted">Delete this connection?</span>
                <Button size="sm" variant="danger" onClick={onDelete} disabled={del.isPending}>
                  {del.isPending ? <Loader2 className="h-4 w-4 animate-spin" /> : 'Confirm'}
                </Button>
                <Button size="sm" variant="ghost" onClick={() => setConfirmDelete(false)}>
                  Keep
                </Button>
              </div>
            ) : (
              <Button
                size="sm"
                variant="ghost"
                className="mr-auto text-danger hover:text-danger"
                onClick={() => setConfirmDelete(true)}
              >
                <Trash2 className="h-4 w-4" /> Delete
              </Button>
            ))}
          {isEdit && (
            <Button size="sm" variant="secondary" onClick={onTest} disabled={test.isPending}>
              {test.isPending ? <Loader2 className="h-4 w-4 animate-spin" /> : <PlugZap className="h-4 w-4" />}
              Test
            </Button>
          )}
          <Button size="sm" variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button size="sm" onClick={onSubmit} disabled={saving}>
            {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : null}
            {isEdit ? 'Save changes' : 'Create'}
          </Button>
        </>
      }
    >
      <form className="space-y-4" onSubmit={onSubmit} noValidate>
        <Field label="Name" htmlFor="conn-name" error={errors.name?.message}>
          <input
            id="conn-name"
            {...register('name')}
            aria-invalid={errors.name ? 'true' : undefined}
            className={inputCls(!!errors.name)}
          />
        </Field>

        <Field label="Type" htmlFor="conn-type" error={errors.type?.message}>
          <select id="conn-type" {...register('type')} className={inputCls(!!errors.type)}>
            {CONNECTION_TYPES.map((t) => (
              <option key={t.value} value={t.value}>
                {t.label}
              </option>
            ))}
          </select>
        </Field>

        <Field label="Host" htmlFor="conn-host" error={errors.host?.message}>
          <input
            id="conn-host"
            placeholder="host.internal:5432"
            {...register('host')}
            aria-invalid={errors.host ? 'true' : undefined}
            className={inputCls(!!errors.host)}
          />
        </Field>

        <Field label="Allowed roles" htmlFor="conn-roles" error={errors.allowedRoles?.message}>
          <Controller
            name="allowedRoles"
            control={control}
            render={({ field }) => (
              <div id="conn-roles" className="flex flex-wrap gap-2" role="group" aria-label="Allowed roles">
                {ROLES.map((role) => {
                  const checked = field.value?.includes(role) ?? false;
                  return (
                    <label
                      key={role}
                      className={cn(
                        'flex cursor-pointer items-center gap-1.5 rounded-md border px-2.5 py-1 text-sm capitalize',
                        checked ? 'border-brand bg-brand/10 text-brand-700' : 'border-border text-ink-muted',
                      )}
                    >
                      <input
                        type="checkbox"
                        className="sr-only"
                        checked={checked}
                        onChange={(e) => {
                          const next = e.target.checked
                            ? [...(field.value ?? []), role]
                            : (field.value ?? []).filter((r) => r !== role);
                          field.onChange(next);
                        }}
                      />
                      {role}
                    </label>
                  );
                })}
              </div>
            )}
          />
        </Field>
      </form>
    </Modal>
  );
}

function inputCls(hasError: boolean) {
  return cn(
    'h-9 w-full rounded-md border bg-surface px-3 text-sm outline-none focus:ring-2 focus:ring-brand',
    hasError ? 'border-danger' : 'border-border',
  );
}

function Field({
  label,
  htmlFor,
  error,
  children,
}: {
  label: string;
  htmlFor: string;
  error?: string;
  children: React.ReactNode;
}) {
  return (
    <div className="space-y-1">
      <label htmlFor={htmlFor} className="text-sm font-medium text-ink">
        {label}
      </label>
      {children}
      {error && (
        <p role="alert" className="text-xs font-medium text-danger">
          {error}
        </p>
      )}
    </div>
  );
}
