package flowvalidate

import (
	"sort"
	"testing"

	"github.com/mywork/automate/internal/flowspec"
)

// findIssue returns the first issue matching nodeID+severity+message, or nil.
func hasIssue(issues []Issue, nodeID, severity, msg string) bool {
	for _, is := range issues {
		if is.NodeID == nodeID && is.Severity == severity && is.Message == msg {
			return true
		}
	}
	return false
}

func TestValidate_CleanFlow_NoIssues(t *testing.T) {
	def := flowspec.FlowDef{
		Nodes: []flowspec.NodeDef{
			{ID: "t1", Type: "trigger.manual", Name: "Manual"},
			{ID: "q1", Type: "db.query", Name: "Query"},
		},
		Edges: []flowspec.EdgeDef{{Source: "t1", Target: "q1"}},
	}
	issues := Validate(def)
	if len(issues) != 0 {
		t.Fatalf("clean flow should have no issues, got %+v", issues)
	}
}

func TestValidate_NoTrigger_Error(t *testing.T) {
	def := flowspec.FlowDef{
		Nodes: []flowspec.NodeDef{
			{ID: "a", Type: "db.query"},
			{ID: "b", Type: "transform.map"},
		},
		Edges: []flowspec.EdgeDef{{Source: "a", Target: "b"}},
	}
	issues := Validate(def)
	if !hasIssue(issues, "", SeverityError, "flow has no trigger node") {
		t.Fatalf("want no-trigger error, got %+v", issues)
	}
}

func TestValidate_MultipleTriggers_Error(t *testing.T) {
	def := flowspec.FlowDef{
		Nodes: []flowspec.NodeDef{
			{ID: "t1", Type: "trigger.manual"},
			{ID: "t2", Type: "trigger.schedule"},
			{ID: "q1", Type: "db.query"},
		},
		Edges: []flowspec.EdgeDef{
			{Source: "t1", Target: "q1"},
			{Source: "t2", Target: "q1"},
		},
	}
	issues := Validate(def)
	if !hasIssue(issues, "", SeverityError, "flow has more than one trigger node") {
		t.Fatalf("want multiple-trigger error, got %+v", issues)
	}
}

func TestValidate_FloatingNode_Warning(t *testing.T) {
	def := flowspec.FlowDef{
		Nodes: []flowspec.NodeDef{
			{ID: "t1", Type: "trigger.manual"},
			{ID: "q1", Type: "db.query"},
			{ID: "orphan", Type: "transform.map"}, // no in or out edge
		},
		Edges: []flowspec.EdgeDef{{Source: "t1", Target: "q1"}},
	}
	issues := Validate(def)
	if !hasIssue(issues, "orphan", SeverityWarning, "node is not connected to any edge") {
		t.Fatalf("want floating-node warning, got %+v", issues)
	}
}

func TestValidate_FloatingTrigger_NotWarned(t *testing.T) {
	// A trigger with no edges is a lone trigger, not a "floating" warning.
	def := flowspec.FlowDef{
		Nodes: []flowspec.NodeDef{
			{ID: "t1", Type: "trigger.manual"},
		},
	}
	issues := Validate(def)
	for _, is := range issues {
		if is.Severity == SeverityWarning {
			t.Fatalf("lone trigger must not produce a floating warning, got %+v", issues)
		}
	}
}

func TestValidate_UnknownEdgeSource_Error(t *testing.T) {
	def := flowspec.FlowDef{
		Nodes: []flowspec.NodeDef{
			{ID: "t1", Type: "trigger.manual"},
		},
		Edges: []flowspec.EdgeDef{{Source: "ghost", Target: "t1"}},
	}
	issues := Validate(def)
	if !hasIssue(issues, "ghost", SeverityError, "edge references unknown source node") {
		t.Fatalf("want unknown-source error, got %+v", issues)
	}
}

func TestValidate_UnknownEdgeTarget_Error(t *testing.T) {
	def := flowspec.FlowDef{
		Nodes: []flowspec.NodeDef{
			{ID: "t1", Type: "trigger.manual"},
		},
		Edges: []flowspec.EdgeDef{{Source: "t1", Target: "ghost"}},
	}
	issues := Validate(def)
	if !hasIssue(issues, "ghost", SeverityError, "edge references unknown target node") {
		t.Fatalf("want unknown-target error, got %+v", issues)
	}
}

func TestValidate_DuplicateNodeIDs_Error(t *testing.T) {
	def := flowspec.FlowDef{
		Nodes: []flowspec.NodeDef{
			{ID: "t1", Type: "trigger.manual"},
			{ID: "dup", Type: "db.query"},
			{ID: "dup", Type: "transform.map"},
		},
		Edges: []flowspec.EdgeDef{
			{Source: "t1", Target: "dup"},
		},
	}
	issues := Validate(def)
	if !hasIssue(issues, "dup", SeverityError, "duplicate node id") {
		t.Fatalf("want duplicate-id error, got %+v", issues)
	}
}

func TestValidate_EmptyNodeType_Error(t *testing.T) {
	def := flowspec.FlowDef{
		Nodes: []flowspec.NodeDef{
			{ID: "t1", Type: "trigger.manual"},
			{ID: "bad", Type: ""},
		},
		Edges: []flowspec.EdgeDef{{Source: "t1", Target: "bad"}},
	}
	issues := Validate(def)
	if !hasIssue(issues, "bad", SeverityError, "node has empty type") {
		t.Fatalf("want empty-type error, got %+v", issues)
	}
}

// A single edge to an unknown node reports both the source and target arms only
// when each is genuinely unknown; here only the target is unknown so exactly one
// unknown-ref issue is produced (guards against double reporting a known end).
func TestValidate_KnownSource_UnknownTarget_OnlyTargetReported(t *testing.T) {
	def := flowspec.FlowDef{
		Nodes: []flowspec.NodeDef{
			{ID: "t1", Type: "trigger.manual"},
			{ID: "q1", Type: "db.query"},
		},
		Edges: []flowspec.EdgeDef{
			{Source: "q1", Target: "ghost"},
			{Source: "t1", Target: "q1"},
		},
	}
	issues := Validate(def)
	if hasIssue(issues, "q1", SeverityError, "edge references unknown source node") {
		t.Fatalf("known source must not be flagged, got %+v", issues)
	}
	if !hasIssue(issues, "ghost", SeverityError, "edge references unknown target node") {
		t.Fatalf("want target flagged, got %+v", issues)
	}
}

func TestHasErrors(t *testing.T) {
	cases := []struct {
		name   string
		issues []Issue
		want   bool
	}{
		{"nil", nil, false},
		{"empty", []Issue{}, false},
		{"warning only", []Issue{{Severity: SeverityWarning, Message: "x"}}, false},
		{"has error", []Issue{{Severity: SeverityWarning}, {Severity: SeverityError}}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := HasErrors(c.issues); got != c.want {
				t.Errorf("HasErrors = %v, want %v", got, c.want)
			}
		})
	}
}

// Duplicate ids are reported once per extra occurrence, deterministically.
func TestValidate_Deterministic(t *testing.T) {
	def := flowspec.FlowDef{
		Nodes: []flowspec.NodeDef{
			{ID: "t1", Type: "trigger.manual"},
			{ID: "a", Type: "db.query"},
			{ID: "b", Type: "transform.map"}, // floating
			{ID: "c", Type: "transform.map"}, // floating
		},
		Edges: []flowspec.EdgeDef{{Source: "t1", Target: "a"}},
	}
	run1 := Validate(def)
	run2 := Validate(def)
	key := func(is []Issue) []string {
		out := make([]string, len(is))
		for i, x := range is {
			out[i] = x.NodeID + "|" + x.Severity + "|" + x.Message
		}
		sort.Strings(out)
		return out
	}
	k1, k2 := key(run1), key(run2)
	if len(k1) != len(k2) {
		t.Fatalf("nondeterministic issue count: %d vs %d", len(k1), len(k2))
	}
	for i := range k1 {
		if k1[i] != k2[i] {
			t.Fatalf("nondeterministic issues: %v vs %v", k1, k2)
		}
	}
}
