// Pure suggestion engine for the expression editor (E3-S3). Framework-free so it
// can be exhaustively unit-tested without React. Given the current textarea text
// and caret offset it returns the completions to offer for the token the caret
// sits in:
//   - root refs:   $node["…"], $flow., $vars.
//   - $flow members: runDate, params.
//   - pipe funcs after a `|`: format, upper, lower, addDays, tz, number,
//     padLeft, padRight, sum, count, avg
//
// Expressions look like `{{ $node["Query"].rowCount | format:"..." }}` — a
// mustache-ish syntax. We only need lightweight, caret-local parsing: look at the
// text immediately before the caret and decide what class of token is being
// typed, then filter the relevant catalog by the partial word.

export type SuggestKind = 'root' | 'member' | 'node' | 'pipe';

export type Suggestion = {
  /** Text shown in the dropdown. */
  label: string;
  /** Text inserted, replacing the current partial token. */
  insert: string;
  /** Category, drives the icon/hint in the UI. */
  kind: SuggestKind;
  /** Optional one-line hint shown next to the label. */
  detail?: string;
};

/** Pipe transform functions available after a `|`. */
export const PIPE_FUNCTIONS: { name: string; detail: string }[] = [
  { name: 'format', detail: 'format:"20060102" — format a date/number' },
  { name: 'upper', detail: 'uppercase text' },
  { name: 'lower', detail: 'lowercase text' },
  { name: 'addDays', detail: 'addDays:7 — shift a date' },
  { name: 'tz', detail: 'tz:"Asia/Bangkok" — convert timezone' },
  { name: 'number', detail: 'number:2 — format as number (n decimals)' },
  { name: 'padLeft', detail: 'padLeft:8 — left-pad to width' },
  { name: 'padRight', detail: 'padRight:8 — right-pad to width' },
  { name: 'sum', detail: 'sum a numeric column' },
  { name: 'count', detail: 'count items' },
  { name: 'avg', detail: 'average a numeric column' },
];

/** Root reference roots available at the start of an expression segment. */
export const ROOT_REFS: { name: string; insert: string; detail: string }[] = [
  { name: '$node', insert: '$node["', detail: 'output of an upstream node' },
  { name: '$flow', insert: '$flow.', detail: 'flow run context' },
  { name: '$vars', insert: '$vars.', detail: 'flow variables' },
];

/** Members available directly under `$flow.`. */
export const FLOW_MEMBERS: { name: string; detail: string }[] = [
  { name: 'runDate', detail: 'the scheduled run date/time' },
  { name: 'params', detail: 'manual-trigger input parameters' },
];

/**
 * Characters that can be part of an identifier token we complete against. `$`
 * and `.` are included so `$flow.run` is treated as one token when scanning
 * back from the caret.
 */
const IDENT = /[A-Za-z0-9_$.]/;

/** Read the "word" immediately to the left of `caret`. */
function tokenBefore(text: string, caret: number): { word: string; start: number } {
  let start = caret;
  while (start > 0 && IDENT.test(text[start - 1])) start -= 1;
  return { word: text.slice(start, caret), start };
}

/** Suggestions for a bare word at segment start → the root refs. */
function rootSuggestions(word: string): Suggestion[] {
  return ROOT_REFS.filter((r) => r.name.startsWith(word)).map((r) => ({
    label: r.name,
    insert: r.insert,
    kind: 'root',
    detail: r.detail,
  }));
}

/** Suggestions for `$flow.<partial>` → flow members. */
function flowMemberSuggestions(partial: string): Suggestion[] {
  return FLOW_MEMBERS.filter((m) => m.name.startsWith(partial)).map((m) => ({
    label: m.name,
    insert: m.name,
    kind: 'member',
    detail: m.detail,
  }));
}

/** Suggestions for a pipe function after `|` → PIPE_FUNCTIONS. */
function pipeSuggestions(partial: string): Suggestion[] {
  return PIPE_FUNCTIONS.filter((f) => f.name.startsWith(partial)).map((f) => ({
    label: f.name,
    insert: f.name,
    kind: 'pipe',
    detail: f.detail,
  }));
}

/**
 * Is the caret positioned in a pipe argument slot? True when, scanning left over
 * the current line, the nearest `|` is closer than the nearest `{{` — i.e. we're
 * after a bar and haven't started a new segment. Whitespace after `|` is allowed.
 */
function afterPipe(before: string): boolean {
  const bar = before.lastIndexOf('|');
  if (bar === -1) return false;
  const open = before.lastIndexOf('{{');
  // A `|` more recent than the last `{{` means we're in a pipe position.
  return bar > open;
}

/**
 * Return completion suggestions for the token the caret is in.
 *
 * @param text  full expression text
 * @param caret zero-based caret offset (defaults to end of text)
 */
export function suggest(text: string, caret: number = text.length): Suggestion[] {
  const pos = Math.max(0, Math.min(caret, text.length));
  const before = text.slice(0, pos);
  const { word } = tokenBefore(text, pos);

  // Inside `$node["…` — the user is typing a node name; the UI's data picker /
  // availableNodes prop supplies the actual names, so the engine offers nothing
  // here (returning [] keeps this a pure, node-list-agnostic function).
  const lastOpenNode = before.lastIndexOf('$node["');
  if (lastOpenNode !== -1) {
    const afterOpen = before.slice(lastOpenNode + '$node["'.length);
    // still inside the unterminated quote (no closing `"` yet)
    if (!afterOpen.includes('"')) return [];
  }

  // Pipe position: after a `|`, offer transform functions.
  if (afterPipe(before)) {
    // the partial is the word after the bar (may be empty)
    return pipeSuggestions(word);
  }

  // `$flow.<partial>` → members of $flow.
  if (word.startsWith('$flow.')) {
    return flowMemberSuggestions(word.slice('$flow.'.length));
  }

  // A partial that is a prefix of one of the roots (including empty) → roots.
  // `$vars.` and `$node[...]` past their root produce no member catalog here
  // (vars are user-defined, node names come from availableNodes).
  if (word === '' || '$node'.startsWith(word) || '$flow'.startsWith(word) || '$vars'.startsWith(word)) {
    return rootSuggestions(word);
  }

  return [];
}

/**
 * Apply a suggestion to the text: replace the partial token ending at `caret`
 * with the suggestion's insert text. Returns the new text and the caret offset
 * to place after the inserted text. Pure — used by ExpressionInput and testable.
 */
export function applySuggestion(
  text: string,
  caret: number,
  suggestion: Suggestion,
): { text: string; caret: number } {
  const pos = Math.max(0, Math.min(caret, text.length));
  const { start } = tokenBefore(text, pos);
  const next = text.slice(0, start) + suggestion.insert + text.slice(pos);
  return { text: next, caret: start + suggestion.insert.length };
}
