'use client';

// SchemaForm — renders a form from a node's JSONSchema (nodeRegistry) using
// react-hook-form. The zod schema is rebuilt from the *current* values on every
// change so conditional (dependentSchemas) fields are shown and validated only
// when their controlling field matches. Emits validity + values upward so the
// canvas can flag incomplete nodes.
import { useEffect, useMemo, useRef } from 'react';
import { useForm, Controller } from 'react-hook-form';
import type { JSONSchema } from '@/lib/nodeRegistry';
import { cn } from '@/lib/cn';
import { ExpressionInput } from './ExpressionInput';
import {
  fieldsFor,
  validate,
  defaultsFor,
  type FieldSpec,
  type FormValues,
} from './schemaToZod';

/** One option for a format:'connection' dropdown (dynamic, from the live list). */
export type ConnectionOption = { value: string; label: string; type?: string };

export type SchemaFormProps = {
  schema: JSONSchema;
  /** Current stored values for the node (merged over schema defaults). */
  value?: FormValues;
  onChange?: (values: FormValues) => void;
  onValidityChange?: (valid: boolean) => void;
  /** stable id so the form re-inits when switching between nodes. */
  formId?: string;
  /** upstream node names offered in the expression data picker (E3-S3). */
  availableNodes?: string[];
  /** live connections that populate format:'connection' fields (E6-S1). */
  connectionOptions?: ConnectionOption[];
};

function fieldId(formId: string, name: string) {
  return `${formId}__${name}`;
}

export function SchemaForm({
  schema,
  value,
  onChange,
  onValidityChange,
  formId = 'schema-form',
  availableNodes = [],
  connectionOptions = [],
}: SchemaFormProps) {
  const initial = useMemo<FormValues>(
    () => ({ ...defaultsFor(schema), ...(value ?? {}) }),
    // reset only when the node (formId) changes, not on every keystroke
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [formId],
  );

  const { control, watch, reset } = useForm<FormValues>({
    defaultValues: initial,
    mode: 'all',
  });

  // Re-seed when switching to a different node.
  useEffect(() => {
    reset(initial);
  }, [formId, initial, reset]);

  const values = watch();
  const fields = fieldsFor(schema, values);
  const { valid, errors } = validate(schema, values);

  // Notify parent of validity + value changes (guarded to avoid render loops).
  const lastValid = useRef<boolean | null>(null);
  useEffect(() => {
    if (lastValid.current !== valid) {
      lastValid.current = valid;
      onValidityChange?.(valid);
    }
  }, [valid, onValidityChange]);

  const onChangeRef = useRef(onChange);
  onChangeRef.current = onChange;
  useEffect(() => {
    const sub = watch((v) => onChangeRef.current?.(v as FormValues));
    return () => sub.unsubscribe();
  }, [watch]);

  return (
    <form className="space-y-4" aria-label="Node configuration" noValidate>
      {fields.map((field) => (
        <Field
          key={field.name}
          field={field}
          control={control}
          formId={formId}
          error={errors[field.name]}
          availableNodes={availableNodes}
          connectionOptions={connectionOptions}
        />
      ))}
      {fields.length === 0 && (
        <p className="text-sm text-ink-muted">This node has no configuration.</p>
      )}
    </form>
  );
}

function Field({
  field,
  control,
  formId,
  error,
  availableNodes,
  connectionOptions = [],
}: {
  field: FieldSpec;
  // react-hook-form's Control is intentionally type-erased here (the form shape
  // is dynamic, driven by the node schema); core-web-vitals does not enforce
  // no-explicit-any so no disable directive is needed.
  control: any;
  formId: string;
  error?: string;
  availableNodes: string[];
  connectionOptions?: ConnectionOption[];
}) {
  const s = field.schema;
  const id = fieldId(formId, field.name);
  const label = s.title ?? field.name;
  const isEnum = Array.isArray(s.enum) && s.enum.length > 0;
  const errorId = `${id}__error`;

  const labelEl = (
    <label htmlFor={id} className="flex items-center gap-1.5 text-sm font-medium text-ink">
      {label}
      {field.required && <span className="text-danger" aria-hidden="true">*</span>}
      {field.dependent && (
        <span className="rounded bg-brand/10 px-1 text-[10px] font-medium uppercase tracking-wide text-brand-700">
          conditional
        </span>
      )}
    </label>
  );

  const describedBy = error ? errorId : undefined;

  // boolean → toggle/checkbox
  if (s.type === 'boolean') {
    return (
      <div className="space-y-1">
        <div className="flex items-center gap-2">
          <Controller
            name={field.name}
            control={control}
            render={({ field: f }) => (
              <input
                id={id}
                type="checkbox"
                checked={Boolean(f.value)}
                onChange={(e) => f.onChange(e.target.checked)}
                onBlur={f.onBlur}
                className="h-4 w-4 rounded border-border text-brand focus:ring-2 focus:ring-brand"
              />
            )}
          />
          {labelEl}
        </div>
        {s.description && <p className="text-xs text-ink-subtle">{s.description}</p>}
        <FieldError id={errorId} message={error} />
      </div>
    );
  }

  // enum → select
  if (isEnum) {
    const options = s.enum ?? [];
    const labels = s.enumLabels ?? [];
    return (
      <div className="space-y-1">
        {labelEl}
        <Controller
          name={field.name}
          control={control}
          render={({ field: f }) => (
            <select
              id={id}
              value={(f.value as string) ?? ''}
              onChange={f.onChange}
              onBlur={f.onBlur}
              aria-invalid={error ? 'true' : undefined}
              aria-describedby={describedBy}
              className={cn(
                'h-9 w-full rounded-md border bg-surface px-3 text-sm outline-none focus:ring-2 focus:ring-brand',
                error ? 'border-danger' : 'border-border',
              )}
            >
              {!field.required && <option value="">— Select —</option>}
              {options.map((opt, i) => (
                <option key={opt} value={opt}>
                  {labels[i] ?? opt}
                </option>
              ))}
            </select>
          )}
        />
        {s.description && <p className="text-xs text-ink-subtle">{s.description}</p>}
        <FieldError id={errorId} message={error} />
      </div>
    );
  }

  // format:'connection' → a dropdown of live connections (dynamic, not free text),
  // narrowed to the field's connectionType when set (E6-S1).
  if (s.format === 'connection') {
    const opts = connectionOptions.filter((o) => !s.connectionType || o.type === s.connectionType);
    return (
      <div className="space-y-1">
        {labelEl}
        <Controller
          name={field.name}
          control={control}
          render={({ field: f }) => (
            <select
              id={id}
              value={(f.value as string) ?? ''}
              onChange={f.onChange}
              onBlur={f.onBlur}
              aria-invalid={error ? 'true' : undefined}
              aria-describedby={describedBy}
              className={cn(
                'h-9 w-full rounded-md border bg-surface px-3 text-sm outline-none focus:ring-2 focus:ring-brand',
                error ? 'border-danger' : 'border-border',
              )}
            >
              <option value="">— select a connection —</option>
              {opts.map((o) => (
                <option key={o.value} value={o.value}>
                  {o.label}
                </option>
              ))}
            </select>
          )}
        />
        {s.description && <p className="text-xs text-ink-subtle">{s.description}</p>}
        {opts.length === 0 && <p className="text-xs text-ink-subtle">No matching connections — create one first.</p>}
        <FieldError id={errorId} message={error} />
      </div>
    );
  }

  // number → number input
  if (s.type === 'number') {
    return (
      <div className="space-y-1">
        {labelEl}
        <Controller
          name={field.name}
          control={control}
          render={({ field: f }) => (
            <input
              id={id}
              type="number"
              value={f.value === undefined || f.value === null ? '' : (f.value as number)}
              min={s.minimum}
              max={s.maximum}
              onChange={(e) =>
                f.onChange(e.target.value === '' ? undefined : Number(e.target.value))
              }
              onBlur={f.onBlur}
              aria-invalid={error ? 'true' : undefined}
              aria-describedby={describedBy}
              className={cn(
                'h-9 w-full rounded-md border bg-surface px-3 text-sm outline-none focus:ring-2 focus:ring-brand',
                error ? 'border-danger' : 'border-border',
              )}
            />
          )}
        />
        {s.description && <p className="text-xs text-ink-subtle">{s.description}</p>}
        <FieldError id={errorId} message={error} />
      </div>
    );
  }

  // string with format:'expression' → the rich ExpressionInput (E3-S3): a
  // monospace textarea with a suggestion dropdown + data-picker side tree.
  if (s.format === 'expression') {
    return (
      <div className="space-y-1">
        <div className="flex items-center justify-between">
          {labelEl}
          <span
            className="rounded bg-surface-sunken px-1.5 py-0.5 font-mono text-[11px] font-semibold text-ink-muted"
            aria-hidden="true"
          >
            ƒx
          </span>
        </div>
        <Controller
          name={field.name}
          control={control}
          render={({ field: f }) => (
            <ExpressionInput
              id={id}
              value={(f.value as string) ?? ''}
              onChange={f.onChange}
              onBlur={f.onBlur}
              invalid={Boolean(error)}
              availableNodes={availableNodes}
              aria-describedby={describedBy}
            />
          )}
        />
        {s.description && <p className="text-xs text-ink-subtle">{s.description}</p>}
        <FieldError id={errorId} message={error} />
      </div>
    );
  }

  // string with format sql → monospace textarea with affordance
  const isCode = s.format === 'sql';
  const isTextarea = s.format === 'textarea' || isCode;
  const affordance = s.format === 'sql' ? 'SQL' : null;

  if (isTextarea) {
    return (
      <div className="space-y-1">
        <div className="flex items-center justify-between">
          {labelEl}
          {affordance && (
            <span
              className="rounded bg-surface-sunken px-1.5 py-0.5 font-mono text-[11px] font-semibold text-ink-muted"
              aria-hidden="true"
            >
              {affordance}
            </span>
          )}
        </div>
        <Controller
          name={field.name}
          control={control}
          render={({ field: f }) => (
            <textarea
              id={id}
              value={(f.value as string) ?? ''}
              onChange={f.onChange}
              onBlur={f.onBlur}
              rows={isCode ? 4 : 3}
              aria-invalid={error ? 'true' : undefined}
              aria-describedby={describedBy}
              className={cn(
                'w-full rounded-md border bg-surface px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-brand',
                isCode && 'font-mono',
                error ? 'border-danger' : 'border-border',
              )}
            />
          )}
        />
        {s.description && <p className="text-xs text-ink-subtle">{s.description}</p>}
        <FieldError id={errorId} message={error} />
      </div>
    );
  }

  // password
  if (s.format === 'password') {
    return (
      <div className="space-y-1">
        {labelEl}
        <Controller
          name={field.name}
          control={control}
          render={({ field: f }) => (
            <input
              id={id}
              type="password"
              value={(f.value as string) ?? ''}
              onChange={f.onChange}
              onBlur={f.onBlur}
              aria-invalid={error ? 'true' : undefined}
              aria-describedby={describedBy}
              className={cn(
                'h-9 w-full rounded-md border bg-surface px-3 text-sm outline-none focus:ring-2 focus:ring-brand',
                error ? 'border-danger' : 'border-border',
              )}
            />
          )}
        />
        {s.description && <p className="text-xs text-ink-subtle">{s.description}</p>}
        <FieldError id={errorId} message={error} />
      </div>
    );
  }

  // default: text input
  return (
    <div className="space-y-1">
      {labelEl}
      <Controller
        name={field.name}
        control={control}
        render={({ field: f }) => (
          <input
            id={id}
            type="text"
            value={(f.value as string) ?? ''}
            onChange={f.onChange}
            onBlur={f.onBlur}
            aria-invalid={error ? 'true' : undefined}
            aria-describedby={describedBy}
            className={cn(
              'h-9 w-full rounded-md border bg-surface px-3 text-sm outline-none focus:ring-2 focus:ring-brand',
              error ? 'border-danger' : 'border-border',
            )}
          />
        )}
      />
      {s.description && <p className="text-xs text-ink-subtle">{s.description}</p>}
      <FieldError id={errorId} message={error} />
    </div>
  );
}

function FieldError({ id, message }: { id: string; message?: string }) {
  if (!message) return null;
  return (
    <p id={id} role="alert" className="text-xs font-medium text-danger">
      {message}
    </p>
  );
}
