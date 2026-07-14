package interpreter

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mywork/automate/apps/automate-worker/internal/executors/dbquery"
)

// NodeExecRequest is the input to one node activity: the node identity/type, its
// opaque config, the items flowing in from the predecessor, and the viewer roles
// (threaded through to masking in the db.query executor).
type NodeExecRequest struct {
	NodeID      string           `json:"nodeId"`
	Type        string           `json:"type"`
	Config      json.RawMessage  `json:"config"`
	InItems     []map[string]any `json:"inItems"`
	ViewerRoles []string         `json:"viewerRoles"`
}

// NodeExecResult is a node's output. Decision is "" for plain nodes, or the
// branch label ("true"/"false") for a decision node (logic.if) so the workflow
// can select the matching out-edge.
type NodeExecResult struct {
	Items    []map[string]any `json:"items"`
	Meta     map[string]any   `json:"meta"`
	Decision string           `json:"decision"`
}

// Activities holds the real per-node collaborators injected by the worker
// (a live dbquery.Deps: Querier, Masking engine, Secrets resolver, MaskPoint).
// Tests mock the ExecuteNode method via the Temporal test suite, so the deps are
// only touched on the real db.query path.
type Activities struct {
	deps dbquery.Deps
}

// NewActivities builds the activity struct around a wired dbquery.Deps.
func NewActivities(deps dbquery.Deps) *Activities {
	return &Activities{deps: deps}
}

// dbQueryConfig is the db.query node config shape.
type dbQueryConfig struct {
	SQL        string `json:"sql"`
	ConnSecret string `json:"connSecret"`
	MaxRows    int    `json:"maxRows"`
}

// ifConfig is the logic.if node config: a simple {left op right} comparison.
// left is currently always "rowCount" (count of incoming items).
type ifConfig struct {
	Left  string  `json:"left"`
	Op    string  `json:"op"`
	Right float64 `json:"right"`
}

// ExecuteNode dispatches one node by Type. It is the single activity registered
// on the worker; the interpreter workflow calls it once per node.
func (a *Activities) ExecuteNode(ctx context.Context, r NodeExecRequest) (NodeExecResult, error) {
	switch r.Type {
	case "db.query":
		return a.execDBQuery(ctx, r)
	case "logic.if":
		return execIf(r)
	case "trigger.manual", "noop":
		// Pass incoming items through with no side effects.
		return NodeExecResult{Items: r.InItems}, nil
	default:
		return NodeExecResult{}, fmt.Errorf("interpreter: unknown node type %q", r.Type)
	}
}

// execDBQuery unmarshals the db.query config and runs the verified executor with
// the worker-injected deps and the request's viewer roles.
func (a *Activities) execDBQuery(ctx context.Context, r NodeExecRequest) (NodeExecResult, error) {
	var cfg dbQueryConfig
	if err := json.Unmarshal(r.Config, &cfg); err != nil {
		return NodeExecResult{}, fmt.Errorf("interpreter: db.query bad config: %w", err)
	}
	out, err := dbquery.Execute(ctx, dbquery.Input{
		SQL:         cfg.SQL,
		MaxRows:     cfg.MaxRows,
		ViewerRoles: r.ViewerRoles,
		ConnSecret:  cfg.ConnSecret,
	}, a.deps)
	if err != nil {
		return NodeExecResult{}, fmt.Errorf("interpreter: db.query node %q: %w", r.NodeID, err)
	}
	return NodeExecResult{
		Items: out.Items,
		Meta: map[string]any{
			"rowCount":  out.Meta.RowCount,
			"truncated": out.Meta.Truncated,
			"columns":   out.Meta.Columns,
		},
	}, nil
}

// execIf evaluates the simple condition and sets Decision to "true"/"false".
// Items pass through unchanged (the branch itself carries no transformation).
func execIf(r NodeExecRequest) (NodeExecResult, error) {
	var cfg ifConfig
	if err := json.Unmarshal(r.Config, &cfg); err != nil {
		return NodeExecResult{}, fmt.Errorf("interpreter: logic.if bad config: %w", err)
	}
	// The only supported left operand today is the incoming item count.
	left := float64(len(r.InItems))
	pass, err := compare(left, cfg.Op, cfg.Right)
	if err != nil {
		return NodeExecResult{}, fmt.Errorf("interpreter: logic.if node %q: %w", r.NodeID, err)
	}
	decision := "false"
	if pass {
		decision = "true"
	}
	return NodeExecResult{Items: r.InItems, Decision: decision}, nil
}

// compare evaluates left <op> right for the supported comparison operators.
func compare(left float64, op string, right float64) (bool, error) {
	switch op {
	case ">":
		return left > right, nil
	case ">=":
		return left >= right, nil
	case "<":
		return left < right, nil
	case "<=":
		return left <= right, nil
	case "==":
		return left == right, nil
	case "!=":
		return left != right, nil
	default:
		return false, fmt.Errorf("unsupported operator %q", op)
	}
}
