// Package flowvalidate holds the pure, side-effect-free flow-graph validator
// used by POST /flows/{id}/validate (docs/spec/08 E3-S4: FR-CANVAS-007,008).
//
// It reports structural problems the UI surfaces before a run: missing/extra
// trigger, floating (disconnected) nodes, edges that reference unknown nodes,
// duplicate node ids and nodes with an empty type. It takes a flowspec.FlowDef
// and returns []Issue; it never touches the store, the network or the clock, so
// it is trivially unit-testable to 100% coverage. Config-level and credential
// validation (the "config invalid, credential missing" arms of E3-S4) are left
// to the per-node schema validators and the connection resolver respectively —
// this pass is the graph-shape gate.
package flowvalidate

import "github.com/mywork/automate/internal/flowspec"

// Severity levels for an Issue. "error" makes a flow invalid; "warning" is
// advisory (the flow is still valid).
const (
	SeverityError   = "error"
	SeverityWarning = "warning"
)

// triggerPrefix is the node-type prefix that marks a trigger node
// (trigger.manual, trigger.schedule, ...). A node is a trigger iff its Type
// starts with this prefix.
const triggerPrefix = "trigger."

// Issue is one validation finding. NodeID is the offending node ("" for
// flow-level issues such as no-trigger). Severity is SeverityError or
// SeverityWarning. Message is a stable, human-readable description (asserted on
// verbatim by callers/tests).
type Issue struct {
	NodeID   string `json:"nodeId"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

// Validate runs every graph rule over def and returns the findings in a
// deterministic order (node-declaration order, edge-declaration order). An empty
// slice means the graph is structurally sound. A flow is "valid" (for the
// endpoint's `valid` bool) when no returned issue has SeverityError — callers
// derive that with HasErrors.
//
// Rules (E3-S4):
//   - (a) no trigger node                          → error (flow-level)
//   - (b) more than one trigger node               → error (flow-level)
//   - (c) a non-trigger node with no incoming AND
//     no outgoing edge (floating/disconnected)     → warning
//   - (d) an edge whose source/target is unknown    → error
//   - (e) duplicate node ids                        → error
//   - (f) a node whose Type is empty                → error
func Validate(def flowspec.FlowDef) []Issue {
	issues := make([]Issue, 0)

	// Index node ids once. seen tracks first occurrence (duplicate detection);
	// known is the membership set used by the edge-reference check.
	known := make(map[string]bool, len(def.Nodes))
	triggerCount := 0

	for _, n := range def.Nodes {
		// (e) duplicate node ids — flag every occurrence after the first.
		if known[n.ID] {
			issues = append(issues, Issue{NodeID: n.ID, Severity: SeverityError, Message: "duplicate node id"})
		}
		known[n.ID] = true

		// (f) empty type.
		if n.Type == "" {
			issues = append(issues, Issue{NodeID: n.ID, Severity: SeverityError, Message: "node has empty type"})
		}

		// Count triggers for (a)/(b).
		if isTrigger(n.Type) {
			triggerCount++
		}
	}

	// (a)/(b) trigger cardinality.
	switch {
	case triggerCount == 0:
		issues = append(issues, Issue{Severity: SeverityError, Message: "flow has no trigger node"})
	case triggerCount > 1:
		issues = append(issues, Issue{Severity: SeverityError, Message: "flow has more than one trigger node"})
	}

	// (d) unknown edge endpoints + track connectivity for (c).
	connected := make(map[string]bool, len(def.Nodes))
	for _, e := range def.Edges {
		if !known[e.Source] {
			issues = append(issues, Issue{NodeID: e.Source, Severity: SeverityError, Message: "edge references unknown source node"})
		} else {
			connected[e.Source] = true
		}
		if !known[e.Target] {
			issues = append(issues, Issue{NodeID: e.Target, Severity: SeverityError, Message: "edge references unknown target node"})
		} else {
			connected[e.Target] = true
		}
	}

	// (c) floating nodes: a node touched by no edge, that isn't a trigger.
	// Lone triggers are legitimate starting points, not floating.
	for _, n := range def.Nodes {
		if connected[n.ID] || isTrigger(n.Type) {
			continue
		}
		issues = append(issues, Issue{NodeID: n.ID, Severity: SeverityWarning, Message: "node is not connected to any edge"})
	}

	return issues
}

// HasErrors reports whether any issue is error-severity (i.e. the flow is
// invalid). Warnings alone leave a flow valid.
func HasErrors(issues []Issue) bool {
	for _, is := range issues {
		if is.Severity == SeverityError {
			return true
		}
	}
	return false
}

// isTrigger reports whether a node type denotes a trigger (trigger.*).
func isTrigger(nodeType string) bool {
	return len(nodeType) >= len(triggerPrefix) && nodeType[:len(triggerPrefix)] == triggerPrefix
}
