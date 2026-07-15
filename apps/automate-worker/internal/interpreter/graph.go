// Package interpreter is the generic flow interpreter for automate-worker
// (docs/spec/04 §2.3, story E1-S7). A flow run maps to a single Temporal
// workflow (FlowWorkflow) that walks the flow-definition graph, executing one
// activity (ExecuteNode) per node and following edges — sequential and branch.
//
// The graph helpers in this file are pure and deterministic (no time.Now, no
// rand, all map/slice iteration is explicitly sorted) so the workflow that
// composes them replays identically. They are unit-tested directly; the
// workflow control flow is covered via the Temporal test suite.
package interpreter

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
)

// triggerPrefix marks a node type as a flow entry point (e.g. "trigger.manual",
// "trigger.schedule"). A node with this prefix is preferred as the start node.
const triggerPrefix = "trigger."

// FlowDef is the flow-definition graph (docs/spec/06 §1: nodes[]/edges[]).
type FlowDef struct {
	Nodes []NodeDef `json:"nodes"`
	Edges []EdgeDef `json:"edges"`
}

// NodeDef is one graph node. Config is the node's opaque, type-specific config.
// OnError controls how the interpreter reacts when the node's activity fails:
// "fail" (default, or "") fails the run; "continue" records the error and carries
// on down the node's normal out-edges with empty items; "errorBranch" records the
// error and follows only the node's "error"-labelled out-edge(s). Retry is the
// per-node activity retry policy (E9-S4 / FR-LOGIC-008). These mirror
// flowspec.NodeDef byte-for-byte so definitions round-trip through Temporal JSON.
type NodeDef struct {
	ID      string          `json:"id"`
	Type    string          `json:"type"`
	Name    string          `json:"name"`
	Config  json.RawMessage `json:"config"`
	OnError string          `json:"onError,omitempty"` // "fail"|"continue"|"errorBranch"
	Retry   *RetryPolicy    `json:"retry,omitempty"`
}

// RetryPolicy is a node's activity retry configuration (mirrors flowspec.RetryPolicy).
type RetryPolicy struct {
	MaxAttempts            int `json:"maxAttempts,omitempty"`            // total attempts (1 = no retry)
	InitialIntervalSeconds int `json:"initialIntervalSeconds,omitempty"` // backoff seed
}

// onError modes.
const (
	onErrorFail        = "fail"
	onErrorContinue    = "continue"
	onErrorErrorBranch = "errorBranch"
	// errorEdgeLabel is the out-edge label followed for an errorBranch node.
	errorEdgeLabel = "error"
)

// EdgeDef is a directed edge. Label is "" for a plain edge, or "true"/"false"
// for the branches out of a decision node (logic.if).
type EdgeDef struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Label  string `json:"label"`
}

// errNoTrigger is returned when the graph has no discernible entry node.
var errNoTrigger = errors.New("interpreter: flow has no trigger node (no trigger.* node and no node without an incoming edge)")

// findTrigger picks the single deterministic start node. A node whose type has
// the "trigger." prefix wins; otherwise the node with no incoming edge is used.
// Ties are broken by the lexicographically smallest node id so the choice never
// depends on slice order (determinism for Temporal replay).
func findTrigger(flow FlowDef) (string, error) {
	// 1. Prefer an explicit trigger.* node (smallest id on ties).
	best := ""
	for _, n := range flow.Nodes {
		if strings.HasPrefix(n.Type, triggerPrefix) {
			if best == "" || n.ID < best {
				best = n.ID
			}
		}
	}
	if best != "" {
		return best, nil
	}
	// 2. Fall back to a node with no incoming edge (smallest id on ties).
	hasIncoming := make(map[string]bool, len(flow.Nodes))
	for _, e := range flow.Edges {
		hasIncoming[e.Target] = true
	}
	ids := make([]string, 0, len(flow.Nodes))
	for _, n := range flow.Nodes {
		if !hasIncoming[n.ID] {
			ids = append(ids, n.ID)
		}
	}
	if len(ids) == 0 {
		return "", errNoTrigger
	}
	sort.Strings(ids)
	return ids[0], nil
}

// outEdges returns the target node ids to follow out of nodeID, sorted for
// determinism. When decision is "" only unlabeled edges are followed (all of
// them). When decision is non-empty only edges whose Label equals decision are
// followed (branch selection: logic.if -> "true"/"false").
func outEdges(edges []EdgeDef, nodeID, decision string) []string {
	var targets []string
	for _, e := range edges {
		if e.Source != nodeID {
			continue
		}
		if decision == "" {
			if e.Label == "" {
				targets = append(targets, e.Target)
			}
			continue
		}
		if e.Label == decision {
			targets = append(targets, e.Target)
		}
	}
	sort.Strings(targets)
	return targets
}

// nodeByID looks up a node by id.
func nodeByID(flow FlowDef, id string) (NodeDef, bool) {
	for _, n := range flow.Nodes {
		if n.ID == id {
			return n, true
		}
	}
	return NodeDef{}, false
}
