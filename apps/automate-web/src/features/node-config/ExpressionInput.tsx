'use client';

// ExpressionInput (E3-S3) — a textarea for `format:'expression'` fields with:
//   - an autocomplete dropdown driven by the pure suggest() engine (root refs,
//     $flow members, and pipe functions after a `|`). ↑/↓ move, Enter/Tab accept,
//     Esc closes; clicking a row inserts it.
//   - a "data picker" side tree of the references actually available here
//     (upstream node names + $flow.* + $vars) that inserts a reference on click.
//
// All parsing lives in src/lib/expr/suggest.ts (unit-tested); this file is the
// thin presentational shell.
import { useMemo, useRef, useState, type KeyboardEvent } from 'react';
import { cn } from '@/lib/cn';
import {
  suggest,
  applySuggestion,
  FLOW_MEMBERS,
  type Suggestion,
} from '@/lib/expr/suggest';

export type ExpressionInputProps = {
  id?: string;
  value: string;
  onChange: (value: string) => void;
  onBlur?: () => void;
  /** upstream node names available to reference via $node["…"]. */
  availableNodes?: string[];
  rows?: number;
  invalid?: boolean;
  'aria-describedby'?: string;
};

type PickerRef = { label: string; insert: string; group: string };

/** Build the flat list of clickable references for the data picker tree. */
function pickerRefs(availableNodes: string[]): PickerRef[] {
  const refs: PickerRef[] = [];
  for (const name of availableNodes) {
    refs.push({ label: name, insert: `$node["${name}"]`, group: 'Nodes' });
  }
  for (const m of FLOW_MEMBERS) {
    refs.push({ label: `$flow.${m.name}`, insert: `$flow.${m.name}`, group: 'Flow' });
  }
  refs.push({ label: '$vars', insert: '$vars.', group: 'Variables' });
  return refs;
}

export function ExpressionInput({
  id,
  value,
  onChange,
  onBlur,
  availableNodes = [],
  rows = 4,
  invalid = false,
  'aria-describedby': describedBy,
}: ExpressionInputProps) {
  const taRef = useRef<HTMLTextAreaElement>(null);
  const [caret, setCaret] = useState(value.length);
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(0);

  const suggestions = useMemo<Suggestion[]>(
    () => (open ? suggest(value, caret) : []),
    [open, value, caret],
  );

  const refs = useMemo(() => pickerRefs(availableNodes), [availableNodes]);

  const listboxId = id ? `${id}__suggest` : 'expr-suggest';

  // Insert text at the caret, replacing the current partial token via the pure
  // helper, and move focus/caret to the end of the inserted text.
  const accept = (s: Suggestion) => {
    const next = applySuggestion(value, caret, s);
    onChange(next.text);
    setCaret(next.caret);
    setOpen(false);
    setActive(0);
    requestAnimationFrame(() => {
      const el = taRef.current;
      if (el) {
        el.focus();
        el.setSelectionRange(next.caret, next.caret);
      }
    });
  };

  // Insert a full reference from the data picker at the current caret.
  const insertRef = (ref: PickerRef) => {
    const el = taRef.current;
    const at = el ? el.selectionStart : value.length;
    const next = value.slice(0, at) + ref.insert + value.slice(at);
    const newCaret = at + ref.insert.length;
    onChange(next);
    setCaret(newCaret);
    requestAnimationFrame(() => {
      if (el) {
        el.focus();
        el.setSelectionRange(newCaret, newCaret);
      }
    });
  };

  const syncCaret = () => {
    const el = taRef.current;
    if (el) setCaret(el.selectionStart);
  };

  const onKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (open && suggestions.length > 0) {
      if (e.key === 'ArrowDown') {
        e.preventDefault();
        setActive((a) => (a + 1) % suggestions.length);
        return;
      }
      if (e.key === 'ArrowUp') {
        e.preventDefault();
        setActive((a) => (a - 1 + suggestions.length) % suggestions.length);
        return;
      }
      if (e.key === 'Enter' || e.key === 'Tab') {
        e.preventDefault();
        accept(suggestions[active]);
        return;
      }
      if (e.key === 'Escape') {
        e.preventDefault();
        setOpen(false);
        return;
      }
    }
  };

  return (
    <div className="grid grid-cols-[1fr,10rem] gap-2">
      <div className="relative">
        <textarea
          ref={taRef}
          id={id}
          value={value}
          rows={rows}
          role="combobox"
          aria-expanded={open && suggestions.length > 0}
          aria-controls={listboxId}
          aria-autocomplete="list"
          aria-invalid={invalid ? 'true' : undefined}
          aria-describedby={describedBy}
          spellCheck={false}
          className={cn(
            'w-full rounded-md border bg-surface px-3 py-2 font-mono text-sm outline-none focus:ring-2 focus:ring-brand',
            invalid ? 'border-danger' : 'border-border',
          )}
          onChange={(e) => {
            onChange(e.target.value);
            setCaret(e.target.selectionStart);
            setOpen(true);
            setActive(0);
          }}
          onKeyUp={syncCaret}
          onClick={syncCaret}
          onKeyDown={onKeyDown}
          onFocus={() => setOpen(true)}
          onBlur={() => {
            // let a click on a suggestion register before closing
            requestAnimationFrame(() => setOpen(false));
            onBlur?.();
          }}
        />

        {open && suggestions.length > 0 && (
          <ul
            id={listboxId}
            role="listbox"
            aria-label="Expression suggestions"
            className="absolute left-0 right-0 top-full z-20 mt-1 max-h-56 overflow-auto rounded-md border border-border bg-surface py-1 shadow-pop"
          >
            {suggestions.map((s, i) => (
              <li
                key={s.label}
                role="option"
                aria-selected={i === active}
                // onMouseDown (not onClick) so it fires before the textarea blur
                onMouseDown={(e) => {
                  e.preventDefault();
                  accept(s);
                }}
                onMouseEnter={() => setActive(i)}
                className={cn(
                  'flex cursor-pointer items-baseline justify-between gap-3 px-3 py-1.5 text-sm',
                  i === active ? 'bg-brand/10 text-ink' : 'text-ink-muted',
                )}
              >
                <span className="font-mono">{s.label}</span>
                {s.detail && <span className="truncate text-[11px] text-ink-subtle">{s.detail}</span>}
              </li>
            ))}
          </ul>
        )}
      </div>

      {/* data picker tree */}
      <div
        className="rounded-md border border-border bg-surface-sunken p-2"
        aria-label="Data picker"
        role="tree"
      >
        <div className="mb-1 text-[11px] font-semibold uppercase tracking-wide text-ink-subtle">
          Insert reference
        </div>
        {refs.length === 0 && (
          <p className="text-[11px] text-ink-subtle">No upstream data.</p>
        )}
        <ul className="space-y-0.5">
          {refs.map((r) => (
            <li key={`${r.group}:${r.label}`} role="treeitem">
              <button
                type="button"
                onClick={() => insertRef(r)}
                title={`Insert ${r.insert}`}
                aria-label={`Insert ${r.label}`}
                className="w-full truncate rounded px-2 py-1 text-left font-mono text-[11px] text-ink-muted hover:bg-brand/10 hover:text-ink"
              >
                {r.label}
              </button>
            </li>
          ))}
        </ul>
      </div>
    </div>
  );
}
