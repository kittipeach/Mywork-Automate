import type { RunStatus } from '@/components/ui/Badge';
import type { LifecycleAction } from '@/api/mutations';

/**
 * Which lifecycle actions a flow offers, keyed by its current status. Mirrors
 * the server-side transition rules (a 409 is returned for anything not listed):
 *   published → Pause, Stop
 *   paused    → Resume, Stop
 *   draft     → (none — must be published first)
 *   stopped   → (none — terminal)
 * Any status not represented here (execution statuses that never appear on a
 * flow row) yields no actions.
 */
const ACTIONS_BY_STATUS: Partial<Record<RunStatus, LifecycleAction[]>> = {
  published: ['pause', 'stop'],
  paused: ['resume', 'stop'],
  draft: [],
  stopped: [],
};

/** The lifecycle actions available for a flow in the given status. */
export function availableLifecycleActions(status: RunStatus): LifecycleAction[] {
  return ACTIONS_BY_STATUS[status] ?? [];
}
