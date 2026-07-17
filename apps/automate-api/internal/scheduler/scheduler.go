package scheduler

import (
	"context"

	"github.com/mywork/automate/internal/flowspec"
)

// Scheduler is the seam handlers use to keep a flow's recurring schedule in sync
// with its lifecycle, without knowing about Temporal. The API composition root
// picks the implementation: the Temporal-backed one when a client is available,
// or Noop when scheduling is disabled.
//
// Handler wiring (docs/spec/08 E5-S1, E4-S2 lifecycle):
//   - publish: compile with SpecFromDefinition; if hasSchedule, call Sync,
//     otherwise Delete (the flow no longer schedules itself).
//   - pause:   call Pause (running runs finish; no new runs start).
//   - resume:  call Resume.
//   - stop:    call Delete (and gracefully cancel in-flight runs elsewhere).
type Scheduler interface {
	// Sync creates the schedule for flowID, or updates it if one already
	// exists (idempotent on publish/republish).
	Sync(ctx context.Context, flowID string, spec Spec, in flowspec.FlowInput) error
	// Pause suspends the schedule; in-flight runs continue.
	Pause(ctx context.Context, flowID string) error
	// Resume un-pauses a paused schedule.
	Resume(ctx context.Context, flowID string) error
	// Delete removes the schedule entirely.
	Delete(ctx context.Context, flowID string) error
}

// Noop is a Scheduler that does nothing and always succeeds. It lets the API run
// (publish/pause/stop still return 2xx) when no Temporal client is configured,
// e.g. local dev or tests, without special-casing a nil Scheduler in handlers.
type Noop struct{}

// Sync implements Scheduler.
func (Noop) Sync(context.Context, string, Spec, flowspec.FlowInput) error { return nil }

// Pause implements Scheduler.
func (Noop) Pause(context.Context, string) error { return nil }

// Resume implements Scheduler.
func (Noop) Resume(context.Context, string) error { return nil }

// Delete implements Scheduler.
func (Noop) Delete(context.Context, string) error { return nil }

// Ensure Noop satisfies the interface at compile time.
var _ Scheduler = Noop{}
