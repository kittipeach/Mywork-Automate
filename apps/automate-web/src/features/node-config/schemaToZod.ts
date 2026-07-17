// Builds a zod schema dynamically from a node's JSONSchema (from nodeRegistry).
//
// Supported: string / number / boolean / enum, required-vs-optional, number
// minimum/maximum bounds. `dependentSchemas` (fieldName -> fieldValue ->
// { prop -> schema }) are resolved against the *current* form values so that a
// dependent field is validated only when its controlling field matches — and is
// required when it appears (dependent fields have no default-empty leniency).
import { z, type ZodTypeAny } from 'zod';
import type { JSONSchema } from '@/lib/nodeRegistry';

export type FormValues = Record<string, unknown>;

/** A flattened field descriptor the renderer iterates over. */
export type FieldSpec = {
  name: string;
  schema: JSONSchema;
  required: boolean;
  /** true when this field only exists because of a dependentSchemas match. */
  dependent: boolean;
};

/**
 * Resolve which extra properties are active for the given values, based on
 * `dependentSchemas`. Returns the merged extra properties and the required set
 * contributed by the active branches (dependent props are always required when
 * shown, matching "fill in what you revealed").
 */
export function resolveDependents(
  schema: JSONSchema,
  values: FormValues,
): { props: Record<string, JSONSchema>; required: Set<string> } {
  const props: Record<string, JSONSchema> = {};
  const required = new Set<string>();
  const dep = schema.dependentSchemas;
  if (!dep) return { props, required };

  for (const [controllingField, byValue] of Object.entries(dep)) {
    const current = values[controllingField];
    if (current == null) continue;
    const branch = byValue[String(current)];
    if (!branch) continue;
    for (const [prop, propSchema] of Object.entries(branch)) {
      props[prop] = propSchema;
      required.add(prop);
    }
  }
  return { props, required };
}

/**
 * The ordered list of fields to render for the given current values: base
 * properties first (registry order), then any active dependent properties.
 */
export function fieldsFor(schema: JSONSchema, values: FormValues): FieldSpec[] {
  const baseProps = schema.properties ?? {};
  const baseRequired = new Set(schema.required ?? []);
  const { props: depProps, required: depRequired } = resolveDependents(schema, values);

  const fields: FieldSpec[] = Object.entries(baseProps).map(([name, s]) => ({
    name,
    schema: s,
    required: baseRequired.has(name),
    dependent: false,
  }));

  for (const [name, s] of Object.entries(depProps)) {
    // A dependent prop may share a name with a base prop; skip duplicates.
    if (baseProps[name]) continue;
    fields.push({ name, schema: s, required: depRequired.has(name), dependent: true });
  }
  return fields;
}

function leafZod(field: FieldSpec): ZodTypeAny {
  const s = field.schema;

  // enum → string enum (labels are handled at render time)
  if (Array.isArray(s.enum) && s.enum.length > 0) {
    const values = s.enum as [string, ...string[]];
    let e: ZodTypeAny = z.enum(values);
    if (!field.required) {
      // allow empty string (nothing selected) when optional
      e = z.union([z.literal(''), z.enum(values)]);
    }
    return e;
  }

  if (s.type === 'number') {
    let n = z.coerce.number({ invalid_type_error: 'Must be a number' });
    if (typeof s.minimum === 'number') n = n.min(s.minimum, `Must be ≥ ${s.minimum}`);
    if (typeof s.maximum === 'number') n = n.max(s.maximum, `Must be ≤ ${s.maximum}`);
    return n;
  }

  if (s.type === 'boolean') {
    return z.boolean();
  }

  // default: string
  let str = z.string();
  if (field.required) str = str.min(1, 'Required');
  return str;
}

/**
 * Build a zod object schema for the given JSONSchema and current values. The
 * dependent fields that are currently active are included so validation reflects
 * exactly what the user sees.
 */
export function buildZodSchema(schema: JSONSchema, values: FormValues): z.ZodTypeAny {
  const fields = fieldsFor(schema, values);
  const shape: Record<string, ZodTypeAny> = {};

  for (const field of fields) {
    let zodType = leafZod(field);

    if (!field.required) {
      // Optional fields tolerate an unset value regardless of type. (Optional
      // enums also tolerate '' via leafZod; optional strings tolerate '' too.)
      zodType = zodType.optional();
    } else if (field.schema.type === 'number') {
      // required number: reject undefined explicitly (coerce turns undefined
      // into NaN which min/max wouldn't catch cleanly).
      zodType = zodType.refine((v) => v !== undefined && v !== null, 'Required');
    }
    shape[field.name] = zodType;
  }

  return z.object(shape);
}

/** Convenience: is the given value set valid against the schema right now? */
export function validate(
  schema: JSONSchema,
  values: FormValues,
): { valid: boolean; errors: Record<string, string> } {
  const zodSchema = buildZodSchema(schema, values);
  const result = zodSchema.safeParse(values);
  if (result.success) return { valid: true, errors: {} };
  const errors: Record<string, string> = {};
  for (const issue of result.error.issues) {
    const key = String(issue.path[0] ?? '');
    if (key && !errors[key]) errors[key] = issue.message;
  }
  return { valid: false, errors };
}

/** Initial form values from schema defaults (used to seed react-hook-form). */
export function defaultsFor(schema: JSONSchema): FormValues {
  const values: FormValues = {};
  const collect = (props: Record<string, JSONSchema> | undefined) => {
    if (!props) return;
    for (const [name, s] of Object.entries(props)) {
      if (s.default !== undefined) values[name] = s.default;
      else if (s.type === 'boolean') values[name] = false;
      else if (s.type === 'number') values[name] = undefined;
      else values[name] = '';
    }
  };
  collect(schema.properties);
  // seed dependent defaults for whatever branch the base defaults select
  const { props } = resolveDependents(schema, values);
  collect(props);
  return values;
}
