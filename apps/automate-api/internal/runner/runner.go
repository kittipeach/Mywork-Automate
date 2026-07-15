// Package runner is the seam between the control-plane API and Temporal. The
// httpapi handlers depend only on the Runner interface so they can be unit
// tested with a synchronous fake; the real implementation (Temporal) is wired in
// the composition root (cmd/api) and exercised by the integration environment.
package runner

import (
	"context"
	"fmt"

	"go.temporal.io/sdk/client"

	"github.com/mywork/automate/internal/flowspec"
)

// Runner starts a flow run and reports its terminal outcome. Run kicks off the
// workflow (a fast, synchronous step) and arranges for onDone to be invoked once
// the workflow completes — for the Temporal implementation that happens on a
// background goroutine, so Run returns as soon as the run is accepted. A nil
// error from Run means the run was accepted (the handler replies 202); onDone
// then carries the eventual success/failure. Cancel requests cancellation of an
// in-flight run identified by its execution id (the Temporal workflow id).
type Runner interface {
	Run(ctx context.Context, executionID string, in flowspec.FlowInput, onDone func(flowspec.FlowResult, error)) error
	Cancel(ctx context.Context, executionID string) error
}

// temporalRunner is the production Runner backed by a Temporal client.
type temporalRunner struct {
	client client.Client
}

// New builds a Temporal-backed Runner over an already-dialled client.
func New(c client.Client) Runner { return &temporalRunner{client: c} }

// Run starts FlowWorkflow with the execution id as the Temporal workflow id
// (idempotency + traceability), then waits for the result on a background
// goroutine and invokes onDone. Failure to *start* is returned synchronously so
// the handler can surface it; a workflow that starts then fails is delivered via
// onDone(err).
func (r *temporalRunner) Run(ctx context.Context, executionID string, in flowspec.FlowInput, onDone func(flowspec.FlowResult, error)) error {
	we, err := r.client.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:        executionID,
		TaskQueue: flowspec.TaskQueue,
	}, flowspec.WorkflowName, in)
	if err != nil {
		return fmt.Errorf("runner: start workflow: %w", err)
	}
	go func() {
		var res flowspec.FlowResult
		getErr := we.Get(context.Background(), &res)
		onDone(res, getErr)
	}()
	return nil
}

// Cancel requests cancellation of the workflow whose id is executionID. An empty
// run id targets the currently-running run of that workflow id. A workflow that
// is already closed (completed/failed/cancelled) surfaces as an error from
// Temporal; the handler guards the terminal case before calling Cancel so this
// path is reached only for in-flight runs.
func (r *temporalRunner) Cancel(ctx context.Context, executionID string) error {
	if err := r.client.CancelWorkflow(ctx, executionID, ""); err != nil {
		return fmt.Errorf("runner: cancel workflow: %w", err)
	}
	return nil
}
