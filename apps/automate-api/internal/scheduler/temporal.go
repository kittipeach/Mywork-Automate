package scheduler

import (
	"context"
	"fmt"

	"go.temporal.io/sdk/client"

	"github.com/mywork/automate/internal/flowspec"
)

// temporalScheduler is the production Scheduler backed by a Temporal client's
// ScheduleClient. It is a thin adapter: all the mappable logic (overlap policy,
// interval build, schedule id) lives in the pure helpers in mapping.go, which
// are unit-tested; the methods here only issue client calls and wrap errors.
// It is exercised end-to-end by the integration environment (needs a live
// Temporal server) and is excluded from the unit-coverage gate.
type temporalScheduler struct {
	client client.Client
}

// New builds a Temporal-backed Scheduler over an already-dialled client. The
// composition root passes the same client used by the runner.
func New(c client.Client) Scheduler { return &temporalScheduler{client: c} }

// Sync creates the flow's schedule, or updates the existing one if Create
// reports it already exists (idempotent republish). The schedule starts
// WorkflowName on TaskQueue with in, in the flow's timezone, honouring the
// overlap policy.
func (s *temporalScheduler) Sync(ctx context.Context, flowID string, spec Spec, in flowspec.FlowInput) error {
	// Validate the policy up front (mapOverlap is the pure, tested mapper); the
	// enum value itself is applied by setScheduleOverlap so this package never
	// names the api enum module (absent from go.mod).
	if _, err := mapOverlap(spec.OverlapPolicy); err != nil {
		return fmt.Errorf("scheduler: sync %s: %w", flowID, err)
	}

	id := scheduleID(flowID)
	action := &client.ScheduleWorkflowAction{
		ID:        id,
		Workflow:  flowspec.WorkflowName,
		Args:      []interface{}{in},
		TaskQueue: flowspec.TaskQueue,
	}

	options := client.ScheduleOptions{
		ID:     id,
		Spec:   buildScheduleSpec(spec),
		Action: action,
	}
	setScheduleOverlap(&options.Overlap, spec.OverlapPolicy)

	_, err := s.client.ScheduleClient().Create(ctx, options)
	if err == nil {
		return nil
	}
	if !isAlreadyExists(err) {
		return fmt.Errorf("scheduler: create schedule %s: %w", id, err)
	}

	// A schedule for this flow already exists: replace its spec/action/policy.
	updateErr := s.client.ScheduleClient().GetHandle(ctx, id).Update(ctx, client.ScheduleUpdateOptions{
		DoUpdate: func(client.ScheduleUpdateInput) (*client.ScheduleUpdate, error) {
			newSpec := buildScheduleSpec(spec)
			policies := &client.SchedulePolicies{}
			setScheduleOverlap(&policies.Overlap, spec.OverlapPolicy)
			return &client.ScheduleUpdate{
				Schedule: &client.Schedule{
					Action: action,
					Spec:   &newSpec,
					Policy: policies,
				},
			}, nil
		},
	})
	if updateErr != nil {
		return fmt.Errorf("scheduler: update schedule %s: %w", id, updateErr)
	}
	return nil
}

// Pause suspends the flow's schedule.
func (s *temporalScheduler) Pause(ctx context.Context, flowID string) error {
	id := scheduleID(flowID)
	if err := s.client.ScheduleClient().GetHandle(ctx, id).Pause(ctx, client.SchedulePauseOptions{}); err != nil {
		return fmt.Errorf("scheduler: pause schedule %s: %w", id, err)
	}
	return nil
}

// Resume un-pauses the flow's schedule.
func (s *temporalScheduler) Resume(ctx context.Context, flowID string) error {
	id := scheduleID(flowID)
	if err := s.client.ScheduleClient().GetHandle(ctx, id).Unpause(ctx, client.ScheduleUnpauseOptions{}); err != nil {
		return fmt.Errorf("scheduler: resume schedule %s: %w", id, err)
	}
	return nil
}

// Delete removes the flow's schedule.
func (s *temporalScheduler) Delete(ctx context.Context, flowID string) error {
	id := scheduleID(flowID)
	if err := s.client.ScheduleClient().GetHandle(ctx, id).Delete(ctx); err != nil {
		return fmt.Errorf("scheduler: delete schedule %s: %w", id, err)
	}
	return nil
}

// Ensure temporalScheduler satisfies the interface at compile time.
var _ Scheduler = (*temporalScheduler)(nil)
