package interpreter

import (
	"reflect"
	"testing"
)

func TestFindTrigger_ByNoIncomingEdge(t *testing.T) {
	flow := FlowDef{
		Nodes: []NodeDef{
			{ID: "b", Type: "noop"},
			{ID: "a", Type: "db.query"},
			{ID: "c", Type: "noop"},
		},
		Edges: []EdgeDef{
			{Source: "a", Target: "b"},
			{Source: "b", Target: "c"},
		},
	}
	got, err := findTrigger(flow)
	if err != nil {
		t.Fatalf("findTrigger() error = %v", err)
	}
	if got != "a" {
		t.Errorf("findTrigger() = %q, want %q", got, "a")
	}
}

func TestFindTrigger_ByTypePrefixWinsOverGraph(t *testing.T) {
	// Even though "start" has an incoming edge (a cycle-ish self-loopless graph),
	// a node whose type has the trigger. prefix is preferred deterministically.
	flow := FlowDef{
		Nodes: []NodeDef{
			{ID: "x", Type: "db.query"},
			{ID: "start", Type: "trigger.manual"},
		},
		Edges: []EdgeDef{
			{Source: "x", Target: "start"},
			{Source: "start", Target: "x"},
		},
	}
	got, err := findTrigger(flow)
	if err != nil {
		t.Fatalf("findTrigger() error = %v", err)
	}
	if got != "start" {
		t.Errorf("findTrigger() = %q, want %q", got, "start")
	}
}

func TestFindTrigger_DeterministicOnTies(t *testing.T) {
	// Two trigger-prefixed nodes: the lexicographically smallest id wins so the
	// choice is deterministic regardless of slice order.
	flow := FlowDef{
		Nodes: []NodeDef{
			{ID: "t2", Type: "trigger.manual"},
			{ID: "t1", Type: "trigger.schedule"},
		},
	}
	got, err := findTrigger(flow)
	if err != nil {
		t.Fatalf("findTrigger() error = %v", err)
	}
	if got != "t1" {
		t.Errorf("findTrigger() = %q, want %q (deterministic tie-break)", got, "t1")
	}
}

func TestFindTrigger_Errors(t *testing.T) {
	tests := []struct {
		name string
		flow FlowDef
	}{
		{
			name: "empty flow",
			flow: FlowDef{},
		},
		{
			name: "every node has an incoming edge and none is trigger-typed",
			flow: FlowDef{
				Nodes: []NodeDef{{ID: "a", Type: "noop"}, {ID: "b", Type: "noop"}},
				Edges: []EdgeDef{{Source: "a", Target: "b"}, {Source: "b", Target: "a"}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := findTrigger(tt.flow); err == nil {
				t.Fatalf("findTrigger() error = nil, want error")
			}
		})
	}
}

func TestOutEdges_UnlabeledFollowsAll(t *testing.T) {
	edges := []EdgeDef{
		{Source: "a", Target: "z"},
		{Source: "a", Target: "y"},
		{Source: "b", Target: "q"},
		{Source: "a", Target: "x"},
	}
	// Decision "" => follow all unlabeled out-edges of "a", sorted by target.
	got := outEdges(edges, "a", "")
	want := []string{"x", "y", "z"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("outEdges(a, \"\") = %v, want %v", got, want)
	}
}

func TestOutEdges_UnlabeledSkipsLabeledEdges(t *testing.T) {
	edges := []EdgeDef{
		{Source: "a", Target: "plain"},
		{Source: "a", Target: "branchT", Label: "true"},
		{Source: "a", Target: "branchF", Label: "false"},
	}
	// With no decision we only follow unlabeled edges.
	got := outEdges(edges, "a", "")
	want := []string{"plain"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("outEdges(a, \"\") = %v, want %v", got, want)
	}
}

func TestOutEdges_DecisionFollowsMatchingLabelOnly(t *testing.T) {
	edges := []EdgeDef{
		{Source: "if", Target: "yes", Label: "true"},
		{Source: "if", Target: "no", Label: "false"},
		{Source: "if", Target: "ignored"}, // unlabeled edge is not followed on a decision
	}
	tests := []struct {
		decision string
		want     []string
	}{
		{"true", []string{"yes"}},
		{"false", []string{"no"}},
		{"maybe", nil}, // no edge matches an unknown decision
	}
	for _, tt := range tests {
		t.Run(tt.decision, func(t *testing.T) {
			got := outEdges(edges, "if", tt.decision)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("outEdges(if, %q) = %v, want %v", tt.decision, got, tt.want)
			}
		})
	}
}

func TestNodeByID(t *testing.T) {
	flow := FlowDef{Nodes: []NodeDef{
		{ID: "a", Type: "db.query", Name: "Q"},
		{ID: "b", Type: "noop", Name: "N"},
	}}
	n, ok := nodeByID(flow, "b")
	if !ok || n.Type != "noop" || n.Name != "N" {
		t.Errorf("nodeByID(b) = %+v, %v", n, ok)
	}
	if _, ok := nodeByID(flow, "missing"); ok {
		t.Errorf("nodeByID(missing) ok = true, want false")
	}
}
