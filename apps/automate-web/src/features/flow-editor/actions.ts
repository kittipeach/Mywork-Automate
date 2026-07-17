// Pure builders for the mock Validate / Test-run banners. Kept out of the
// component so they're unit-testable without React.
import type { FlowNode } from './types';
import type { ConfigByNode } from './types';
import { invalidNodes } from './graph';

export type BannerTone = 'success' | 'warning' | 'danger' | 'info';
export type Banner = { tone: BannerTone; title: string; detail?: string };

/** Validate: list nodes missing required config. */
export function validateFlow(nodes: FlowNode[], config: ConfigByNode): Banner {
  if (nodes.length === 0) {
    return { tone: 'warning', title: 'Nothing to validate', detail: 'Add at least one node.' };
  }
  const bad = invalidNodes(nodes, config);
  if (bad.length === 0) {
    return { tone: 'success', title: 'Validation passed', detail: `${nodes.length} node(s) configured.` };
  }
  return {
    tone: 'danger',
    title: `${bad.length} node(s) need attention`,
    detail: bad.map((n) => n.name).join(', '),
  };
}

/** Test run: a fake per-node success sweep (only runs if valid). */
export function testRun(nodes: FlowNode[], config: ConfigByNode): Banner {
  const v = validateFlow(nodes, config);
  if (v.tone !== 'success') {
    return { tone: 'warning', title: 'Fix config before test run', detail: v.detail };
  }
  return {
    tone: 'success',
    title: 'Test run complete',
    detail: `${nodes.length}/${nodes.length} nodes succeeded.`,
  };
}

/** Publish: only allowed when everything validates. */
export function publishFlow(nodes: FlowNode[], config: ConfigByNode): Banner {
  const v = validateFlow(nodes, config);
  if (v.tone !== 'success') {
    return { tone: 'danger', title: 'Cannot publish', detail: v.detail };
  }
  return { tone: 'success', title: 'Flow published', detail: 'A new version is live.' };
}
