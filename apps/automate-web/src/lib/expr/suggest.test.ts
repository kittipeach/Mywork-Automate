import { describe, it, expect } from 'vitest';
import {
  suggest,
  applySuggestion,
  PIPE_FUNCTIONS,
  ROOT_REFS,
  FLOW_MEMBERS,
} from './suggest';

const labels = (text: string, caret?: number) => suggest(text, caret).map((s) => s.label);
const kinds = (text: string, caret?: number) => suggest(text, caret).map((s) => s.kind);

describe('catalogs', () => {
  it('exposes the pipe function names required by the spec', () => {
    const names = PIPE_FUNCTIONS.map((f) => f.name);
    expect(names).toEqual([
      'format', 'upper', 'lower', 'addDays', 'tz',
      'number', 'padLeft', 'padRight', 'sum', 'count', 'avg',
    ]);
  });

  it('exposes the three root refs with insert text', () => {
    expect(ROOT_REFS.map((r) => r.name)).toEqual(['$node', '$flow', '$vars']);
    expect(ROOT_REFS.find((r) => r.name === '$node')?.insert).toBe('$node["');
    expect(ROOT_REFS.find((r) => r.name === '$flow')?.insert).toBe('$flow.');
    expect(ROOT_REFS.find((r) => r.name === '$vars')?.insert).toBe('$vars.');
  });

  it('exposes $flow members', () => {
    expect(FLOW_MEMBERS.map((m) => m.name)).toEqual(['runDate', 'params']);
  });
});

describe('suggest — root refs', () => {
  it('offers all three roots on an empty expression', () => {
    expect(labels('')).toEqual(['$node', '$flow', '$vars']);
    expect(kinds('')).toEqual(['root', 'root', 'root']);
  });

  it('offers all three roots inside an empty mustache segment', () => {
    expect(labels('{{ ')).toEqual(['$node', '$flow', '$vars']);
  });

  it('filters roots by the partial word', () => {
    expect(labels('{{ $f')).toEqual(['$flow']);
    expect(labels('{{ $v')).toEqual(['$vars']);
    expect(labels('{{ $n')).toEqual(['$node']);
  });

  it('a bare $ still offers every root', () => {
    expect(labels('{{ $')).toEqual(['$node', '$flow', '$vars']);
  });

  it('root insert text carries the opening syntax', () => {
    const node = suggest('{{ $n')[0];
    expect(node.insert).toBe('$node["');
    const flow = suggest('{{ $f')[0];
    expect(flow.insert).toBe('$flow.');
  });
});

describe('suggest — $flow members', () => {
  it('offers members after $flow.', () => {
    expect(labels('{{ $flow.')).toEqual(['runDate', 'params']);
    expect(kinds('{{ $flow.')).toEqual(['member', 'member']);
  });

  it('filters members by the partial', () => {
    expect(labels('{{ $flow.run')).toEqual(['runDate']);
    expect(labels('{{ $flow.par')).toEqual(['params']);
  });

  it('unknown $flow member yields nothing', () => {
    expect(labels('{{ $flow.zzz')).toEqual([]);
  });
});

describe('suggest — node refs', () => {
  it('offers nothing while typing inside an open $node["…', () => {
    // node names come from availableNodes in the UI, not the pure engine
    expect(suggest('{{ $node["Que')).toEqual([]);
    expect(suggest('{{ $node["')).toEqual([]);
  });

  it('resumes suggestions once the node bracket is closed', () => {
    // after `$node["Query"]` the caret token is empty (] is not an ident char),
    // so we are back at segment start and the roots are offered again.
    expect(labels('{{ $node["Query"]')).toEqual(['$node', '$flow', '$vars']);
  });

  it('a closed node ref followed by a pipe offers pipe funcs', () => {
    expect(labels('{{ $node["Query"].rowCount | ')).toEqual(PIPE_FUNCTIONS.map((f) => f.name));
  });
});

describe('suggest — pipe functions after a bar', () => {
  it('offers all pipe funcs immediately after |', () => {
    expect(labels('{{ $flow.runDate | ')).toEqual(PIPE_FUNCTIONS.map((f) => f.name));
    expect(kinds('{{ $flow.runDate | ')[0]).toBe('pipe');
  });

  it('offers pipe funcs with no space after the bar', () => {
    expect(labels('{{ $flow.runDate |')).toEqual(PIPE_FUNCTIONS.map((f) => f.name));
  });

  it('filters pipe funcs by the partial after the bar', () => {
    expect(labels('{{ $flow.runDate | for')).toEqual(['format']);
    expect(labels('{{ $flow.runDate | pad')).toEqual(['padLeft', 'padRight']);
    expect(labels('{{ x | a')).toEqual(['addDays', 'avg']);
  });

  it('a bar from a previous closed segment does not leak into a new one', () => {
    // second `{{` opens a fresh segment; the earlier `|` is before it
    const text = '{{ x | upper }} {{ $f';
    expect(labels(text)).toEqual(['$flow']);
  });
});

describe('suggest — no-suggestion cases', () => {
  it('a non-matching identifier yields nothing', () => {
    expect(labels('{{ foobar')).toEqual([]);
  });

  it('a completed known keyword with no dot/bar yields nothing', () => {
    expect(labels('{{ $vars')).toEqual(['$vars']); // still a prefix of $vars → offered
    expect(labels('{{ $varsX')).toEqual([]);
  });
});

describe('suggest — caret positions', () => {
  it('uses the caret, not end of text, to decide the token', () => {
    const text = '{{ $flow. | upper }}';
    // caret right after the dot → member suggestions, ignoring the later pipe
    const caret = text.indexOf('.') + 1;
    expect(labels(text, caret)).toEqual(['runDate', 'params']);
  });

  it('caret in the middle of a root partial filters correctly', () => {
    const text = '{{ $flow }}';
    const caret = text.indexOf('$flow') + 2; // after "$f"
    expect(labels(text, caret)).toEqual(['$flow']);
  });

  it('caret past the pipe offers pipe funcs even with trailing text', () => {
    const text = '{{ v | fo XX }}';
    const caret = text.indexOf('fo') + 2;
    expect(labels(text, caret)).toEqual(['format']);
  });

  it('clamps out-of-range carets', () => {
    expect(labels('', -5)).toEqual(['$node', '$flow', '$vars']);
    expect(labels('{{ $f', 999)).toEqual(['$flow']);
  });

  it('defaults caret to end of text when omitted', () => {
    expect(suggest('{{ $f')).toEqual(suggest('{{ $f', 5));
  });
});

describe('applySuggestion', () => {
  it('replaces the partial token with the insert text and returns the new caret', () => {
    const text = '{{ $f';
    const s = suggest(text)[0]; // $flow
    const out = applySuggestion(text, text.length, s);
    expect(out.text).toBe('{{ $flow.');
    expect(out.caret).toBe(out.text.length);
  });

  it('inserts a pipe function replacing the partial after the bar', () => {
    const text = '{{ v | for';
    const s = suggest(text)[0]; // format
    const out = applySuggestion(text, text.length, s);
    expect(out.text).toBe('{{ v | format');
    expect(out.caret).toBe('{{ v | format'.length);
  });

  it('inserts at a mid-string caret, keeping the tail', () => {
    const text = '{{ $f }}';
    const caret = text.indexOf('$f') + 2;
    const s = suggest(text, caret)[0];
    const out = applySuggestion(text, caret, s);
    expect(out.text).toBe('{{ $flow. }}');
    expect(out.caret).toBe('{{ $flow.'.length);
  });

  it('clamps out-of-range carets', () => {
    const s = suggest('{{ $f')[0];
    const out = applySuggestion('{{ $f', 999, s);
    expect(out.text).toBe('{{ $flow.');
  });
});
