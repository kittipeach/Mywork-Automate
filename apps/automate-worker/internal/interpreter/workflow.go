package interpreter

import (
	"fmt"
	"sort"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// FlowInput starts a flow run: the pinned flow definition, the viewer's roles
// (for masking downstream) and optional trigger params.
type FlowInput struct {
	Flow        FlowDef        `json:"flow"`
	ViewerRoles []string       `json:"viewerRoles"`
	Params      map[string]any `json:"params"`
}

// NodeOutput is one node's produced items plus metadata, kept in FlowResult.
type NodeOutput struct {
	Items []map[string]any `json:"items"`
	Meta  map[string]any   `json:"meta"`
}

// FlowResult is the workflow result: the node ids in execution order plus each
// executed node's output.
type FlowResult struct {
	Path    []string              `json:"path"`
	Outputs map[string]NodeOutput `json:"outputs"`
}

// activityStartToClose bounds a single node activity attempt. The db.query
// per-statement timeout (FR-DB-007) is enforced inside dbquery/pgxquerier; this
// is the outer Temporal bound so a wedged attempt cannot run forever.
const activityStartToClose = 2 * time.Minute

// FlowWorkflow is the generic interpreter. It is deterministic: it finds the
// single trigger node, then walks the graph node-by-node, executing each via the
// ExecuteNode activity and following edges. For a node whose activity returns a
// non-empty Decision it follows only the edge whose Label equals that Decision
// (logic.if -> "true"/"false"); otherwise it follows all unlabeled out-edges.
// All iteration order is sorted (see graph helpers) so replay is identical.
func FlowWorkflow(ctx workflow.Context, in FlowInput) (FlowResult, error) {
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: activityStartToClose,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    time.Minute,
			MaximumAttempts:    5,
		},
	})

	start, err := findTrigger(in.Flow)
	if err != nil {
		return FlowResult{}, err
	}

	result := FlowResult{Outputs: make(map[string]NodeOutput)}
	var a *Activities // nil pointer receiver: names the registered struct method.

	// Iterative walk with an explicit queue. The visited set makes each node
	// execute at most once, which guarantees termination even for a mis-authored
	// definition that contains a cycle among unlabeled edges (a cyclic edge just
	// re-enqueues an already-visited node, which is then skipped).
	queue := []string{start}
	visited := make(map[string]bool)

	for len(queue) > 0 {
		nodeID := queue[0]
		queue = queue[1:]
		if visited[nodeID] {
			continue
		}
		visited[nodeID] = true

		node, ok := nodeByID(in.Flow, nodeID)
		if !ok {
			return FlowResult{}, fmt.Errorf("interpreter: edge points at unknown node %q", nodeID)
		}

		// The input items of this node are the output items of its (single)
		// predecessor already recorded in the outputs map, if any.
		inItems := predecessorItems(in.Flow, result.Outputs, nodeID)

		req := NodeExecRequest{
			NodeID:      node.ID,
			Type:        node.Type,
			Config:      node.Config,
			InItems:     inItems,
			ViewerRoles: in.ViewerRoles,
		}
		var res NodeExecResult
		if err := workflow.ExecuteActivity(ctx, a.ExecuteNode, req).Get(ctx, &res); err != nil {
			return FlowResult{}, fmt.Errorf("interpreter: node %q (%s) failed: %w", node.ID, node.Type, err)
		}

		result.Path = append(result.Path, node.ID)
		result.Outputs[node.ID] = NodeOutput{Items: res.Items, Meta: res.Meta}

		for _, next := range outEdges(in.Flow.Edges, node.ID, res.Decision) {
			if !visited[next] {
				queue = append(queue, next)
			}
		}
	}

	return result, nil
}

// predecessorItems returns the items to feed nodeID: the output items of the
// first (sorted) source node that has an edge into nodeID and has already run.
// This threads db.query output into the next node's input.
func predecessorItems(flow FlowDef, outputs map[string]NodeOutput, nodeID string) []map[string]any {
	var sources []string
	for _, e := range flow.Edges {
		if e.Target == nodeID {
			sources = append(sources, e.Source)
		}
	}
	sort.Strings(sources)
	for _, s := range sources {
		if out, ok := outputs[s]; ok {
			return out.Items
		}
	}
	return nil
}
