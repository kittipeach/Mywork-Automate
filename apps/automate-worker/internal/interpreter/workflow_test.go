package interpreter

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"
)

// seqFlow: trigger.manual -> db.query -> noop (3-node sequential).
func seqFlow() FlowDef {
	return FlowDef{
		Nodes: []NodeDef{
			{ID: "trg", Type: "trigger.manual", Name: "Manual"},
			{ID: "q", Type: "db.query", Name: "Query"},
			{ID: "end", Type: "noop", Name: "Done"},
		},
		Edges: []EdgeDef{
			{Source: "trg", Target: "q"},
			{Source: "q", Target: "end"},
		},
	}
}

// branchFlow: trigger.manual -> logic.if -(true)-> nodeA / -(false)-> nodeB.
func branchFlow() FlowDef {
	return FlowDef{
		Nodes: []NodeDef{
			{ID: "trg", Type: "trigger.manual", Name: "Manual"},
			{ID: "if", Type: "logic.if", Name: "Gate"},
			{ID: "nodeA", Type: "noop", Name: "A"},
			{ID: "nodeB", Type: "noop", Name: "B"},
		},
		Edges: []EdgeDef{
			{Source: "trg", Target: "if"},
			{Source: "if", Target: "nodeA", Label: "true"},
			{Source: "if", Target: "nodeB", Label: "false"},
		},
	}
}

func TestFlowWorkflow_Sequential(t *testing.T) {
	var ts testsuite.WorkflowTestSuite
	env := ts.NewTestWorkflowEnvironment()

	var a *Activities
	// trigger passes through empty items.
	env.OnActivity(a.ExecuteNode, mock.Anything, mock.MatchedBy(func(r NodeExecRequest) bool {
		return r.NodeID == "trg"
	})).Return(NodeExecResult{Items: nil}, nil)
	// db.query returns two rows; assert this output propagates into the next node's input.
	dbRows := []map[string]any{{"id": 1}, {"id": 2}}
	env.OnActivity(a.ExecuteNode, mock.Anything, mock.MatchedBy(func(r NodeExecRequest) bool {
		return r.NodeID == "q"
	})).Return(NodeExecResult{Items: dbRows, Meta: map[string]any{"rowCount": 2}}, nil)
	// noop must receive the db.query rows as its input items (payloads round-trip
	// through JSON so the count, not the Go numeric type, is the assertion here).
	env.OnActivity(a.ExecuteNode, mock.Anything, mock.MatchedBy(func(r NodeExecRequest) bool {
		return r.NodeID == "end" && len(r.InItems) == 2
	})).Return(func(_ context.Context, r NodeExecRequest) (NodeExecResult, error) {
		// Echo back exactly what flowed in, proving db.query output propagated.
		return NodeExecResult{Items: r.InItems}, nil
	})

	env.ExecuteWorkflow(FlowWorkflow, FlowInput{Flow: seqFlow(), ViewerRoles: []string{"analyst"}})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var res FlowResult
	require.NoError(t, env.GetWorkflowResult(&res))
	require.Equal(t, []string{"trg", "q", "end"}, res.Path)
	// db.query output propagated through to the final node. Payloads round-trip
	// via JSON, so numeric values normalize to float64; assert row count + the
	// carried id values rather than the exact Go type.
	require.Len(t, res.Outputs["q"].Items, 2)
	require.Len(t, res.Outputs["end"].Items, 2)
	require.EqualValues(t, 1, res.Outputs["end"].Items[0]["id"])
	require.EqualValues(t, 2, res.Outputs["end"].Items[1]["id"])
	env.AssertExpectations(t)
}

func TestFlowWorkflow_BranchTrue(t *testing.T) {
	var ts testsuite.WorkflowTestSuite
	env := ts.NewTestWorkflowEnvironment()

	var a *Activities
	env.OnActivity(a.ExecuteNode, mock.Anything, mock.MatchedBy(func(r NodeExecRequest) bool {
		return r.NodeID == "trg"
	})).Return(NodeExecResult{}, nil)
	env.OnActivity(a.ExecuteNode, mock.Anything, mock.MatchedBy(func(r NodeExecRequest) bool {
		return r.NodeID == "if"
	})).Return(NodeExecResult{Decision: "true"}, nil)
	// Only nodeA must run on the true branch. nodeB has NO mock; if the workflow
	// tried to run it the test env would fail on an unexpected activity call.
	env.OnActivity(a.ExecuteNode, mock.Anything, mock.MatchedBy(func(r NodeExecRequest) bool {
		return r.NodeID == "nodeA"
	})).Return(NodeExecResult{}, nil)

	env.ExecuteWorkflow(FlowWorkflow, FlowInput{Flow: branchFlow()})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var res FlowResult
	require.NoError(t, env.GetWorkflowResult(&res))
	require.Equal(t, []string{"trg", "if", "nodeA"}, res.Path)
	require.NotContains(t, res.Path, "nodeB")
	env.AssertExpectations(t)
}

func TestFlowWorkflow_BranchFalse(t *testing.T) {
	var ts testsuite.WorkflowTestSuite
	env := ts.NewTestWorkflowEnvironment()

	var a *Activities
	env.OnActivity(a.ExecuteNode, mock.Anything, mock.MatchedBy(func(r NodeExecRequest) bool {
		return r.NodeID == "trg"
	})).Return(NodeExecResult{}, nil)
	env.OnActivity(a.ExecuteNode, mock.Anything, mock.MatchedBy(func(r NodeExecRequest) bool {
		return r.NodeID == "if"
	})).Return(NodeExecResult{Decision: "false"}, nil)
	// Only nodeB must run on the false branch.
	env.OnActivity(a.ExecuteNode, mock.Anything, mock.MatchedBy(func(r NodeExecRequest) bool {
		return r.NodeID == "nodeB"
	})).Return(NodeExecResult{}, nil)

	env.ExecuteWorkflow(FlowWorkflow, FlowInput{Flow: branchFlow()})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var res FlowResult
	require.NoError(t, env.GetWorkflowResult(&res))
	require.Equal(t, []string{"trg", "if", "nodeB"}, res.Path)
	require.NotContains(t, res.Path, "nodeA")
	env.AssertExpectations(t)
}

func TestFlowWorkflow_ActivityError(t *testing.T) {
	var ts testsuite.WorkflowTestSuite
	env := ts.NewTestWorkflowEnvironment()

	var a *Activities
	env.OnActivity(a.ExecuteNode, mock.Anything, mock.MatchedBy(func(r NodeExecRequest) bool {
		return r.NodeID == "trg"
	})).Return(NodeExecResult{}, nil)
	// db.query fails; the workflow must surface an error.
	env.OnActivity(a.ExecuteNode, mock.Anything, mock.MatchedBy(func(r NodeExecRequest) bool {
		return r.NodeID == "q"
	})).Return(NodeExecResult{}, errors.New("boom"))

	env.ExecuteWorkflow(FlowWorkflow, FlowInput{Flow: seqFlow()})

	require.True(t, env.IsWorkflowCompleted())
	require.Error(t, env.GetWorkflowError())
}

func TestFlowWorkflow_DiamondVisitsConvergingNodeOnce(t *testing.T) {
	// trg -> a, trg -> b, a -> join, b -> join. "join" is enqueued twice (once per
	// predecessor) but must execute exactly once (the visited guard).
	flow := FlowDef{
		Nodes: []NodeDef{
			{ID: "trg", Type: "trigger.manual"},
			{ID: "a", Type: "noop"},
			{ID: "b", Type: "noop"},
			{ID: "join", Type: "noop"},
		},
		Edges: []EdgeDef{
			{Source: "trg", Target: "a"},
			{Source: "trg", Target: "b"},
			{Source: "a", Target: "join"},
			{Source: "b", Target: "join"},
		},
	}

	var ts testsuite.WorkflowTestSuite
	env := ts.NewTestWorkflowEnvironment()
	var a *Activities
	joinCalls := 0
	env.OnActivity(a.ExecuteNode, mock.Anything, mock.Anything).Return(
		func(_ context.Context, r NodeExecRequest) (NodeExecResult, error) {
			if r.NodeID == "join" {
				joinCalls++
			}
			return NodeExecResult{}, nil
		})

	env.ExecuteWorkflow(FlowWorkflow, FlowInput{Flow: flow})
	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var res FlowResult
	require.NoError(t, env.GetWorkflowResult(&res))
	require.Equal(t, 1, joinCalls, "converging node must run exactly once")
	// trg then a,b (sorted) then join — join appears exactly once.
	require.Equal(t, []string{"trg", "a", "b", "join"}, res.Path)
}

func TestFlowWorkflow_EdgeToUnknownNodeError(t *testing.T) {
	// trg has an out-edge to a target that is not declared in Nodes.
	flow := FlowDef{
		Nodes: []NodeDef{{ID: "trg", Type: "trigger.manual"}},
		Edges: []EdgeDef{{Source: "trg", Target: "ghost"}},
	}
	var ts testsuite.WorkflowTestSuite
	env := ts.NewTestWorkflowEnvironment()
	var a *Activities
	env.OnActivity(a.ExecuteNode, mock.Anything, mock.MatchedBy(func(r NodeExecRequest) bool {
		return r.NodeID == "trg"
	})).Return(NodeExecResult{}, nil)

	env.ExecuteWorkflow(FlowWorkflow, FlowInput{Flow: flow})
	require.True(t, env.IsWorkflowCompleted())
	require.Error(t, env.GetWorkflowError())
}

func TestFlowWorkflow_NoTriggerError(t *testing.T) {
	var ts testsuite.WorkflowTestSuite
	env := ts.NewTestWorkflowEnvironment()

	// A cyclic graph with no trigger node => workflow errors before any activity.
	flow := FlowDef{
		Nodes: []NodeDef{{ID: "a", Type: "noop"}, {ID: "b", Type: "noop"}},
		Edges: []EdgeDef{{Source: "a", Target: "b"}, {Source: "b", Target: "a"}},
	}
	env.ExecuteWorkflow(FlowWorkflow, FlowInput{Flow: flow})

	require.True(t, env.IsWorkflowCompleted())
	require.Error(t, env.GetWorkflowError())
}
