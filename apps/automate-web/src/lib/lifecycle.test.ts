import { describe, it, expect } from 'vitest';
import { availableLifecycleActions } from './lifecycle';

describe('availableLifecycleActions', () => {
  it('offers Pause + Stop for a published flow', () => {
    expect(availableLifecycleActions('published')).toEqual(['pause', 'stop']);
  });

  it('offers Resume + Stop for a paused flow', () => {
    expect(availableLifecycleActions('paused')).toEqual(['resume', 'stop']);
  });

  it('offers no actions for a draft flow', () => {
    expect(availableLifecycleActions('draft')).toEqual([]);
  });

  it('offers no actions for a stopped (terminal) flow', () => {
    expect(availableLifecycleActions('stopped')).toEqual([]);
  });

  it('offers no actions for statuses that never appear on a flow row', () => {
    expect(availableLifecycleActions('running')).toEqual([]);
    expect(availableLifecycleActions('success')).toEqual([]);
  });
});
