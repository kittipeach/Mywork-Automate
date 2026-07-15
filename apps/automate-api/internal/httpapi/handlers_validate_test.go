package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mywork/automate/internal/flowspec"
)

// issueList extracts the "issues" array from a validate response body.
func issueList(t *testing.T, body map[string]any) []map[string]any {
	t.Helper()
	raw, ok := body["issues"].([]any)
	if !ok {
		t.Fatalf("issues missing/not an array: %v", body["issues"])
	}
	out := make([]map[string]any, len(raw))
	for i, r := range raw {
		out[i] = r.(map[string]any)
	}
	return out
}

func hasBodyIssue(issues []map[string]any, nodeID, severity, msg string) bool {
	for _, is := range issues {
		if is["nodeId"] == nodeID && is["severity"] == severity && is["message"] == msg {
			return true
		}
	}
	return false
}

func TestValidateFlow_CleanFlow_Valid(t *testing.T) {
	fake := seedFake()
	fake.def = flowspec.FlowDef{
		Nodes: []flowspec.NodeDef{
			{ID: "t1", Type: "trigger.manual", Name: "Manual"},
			{ID: "q1", Type: "db.query", Name: "Query"},
		},
		Edges: []flowspec.EdgeDef{{Source: "t1", Target: "q1"}},
	}
	r := routerWithRunner(fake, &fakeRunner{})
	w, body := doReq(t, r, http.MethodPost, "/api/automate/v1/flows/flw_payroll/validate", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if body["valid"] != true {
		t.Errorf("valid = %v, want true", body["valid"])
	}
	if len(issueList(t, body)) != 0 {
		t.Errorf("clean flow should report no issues, got %v", body["issues"])
	}
}

func TestValidateFlow_NoTrigger_InvalidWithError(t *testing.T) {
	fake := seedFake()
	fake.def = flowspec.FlowDef{
		Nodes: []flowspec.NodeDef{
			{ID: "a", Type: "db.query"},
			{ID: "b", Type: "transform.map"},
		},
		Edges: []flowspec.EdgeDef{{Source: "a", Target: "b"}},
	}
	r := routerWithRunner(fake, &fakeRunner{})
	w, body := doReq(t, r, http.MethodPost, "/api/automate/v1/flows/flw_payroll/validate", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if body["valid"] != false {
		t.Errorf("valid = %v, want false", body["valid"])
	}
	if !hasBodyIssue(issueList(t, body), "", "error", "flow has no trigger node") {
		t.Errorf("want no-trigger error, got %v", body["issues"])
	}
}

func TestValidateFlow_FloatingNode_WarningStillValid(t *testing.T) {
	fake := seedFake()
	fake.def = flowspec.FlowDef{
		Nodes: []flowspec.NodeDef{
			{ID: "t1", Type: "trigger.manual"},
			{ID: "q1", Type: "db.query"},
			{ID: "orphan", Type: "transform.map"},
		},
		Edges: []flowspec.EdgeDef{{Source: "t1", Target: "q1"}},
	}
	r := routerWithRunner(fake, &fakeRunner{})
	w, body := doReq(t, r, http.MethodPost, "/api/automate/v1/flows/flw_payroll/validate", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	// Warnings alone leave the flow valid.
	if body["valid"] != true {
		t.Errorf("valid = %v, want true (warnings don't invalidate)", body["valid"])
	}
	if !hasBodyIssue(issueList(t, body), "orphan", "warning", "node is not connected to any edge") {
		t.Errorf("want floating warning, got %v", body["issues"])
	}
}

func TestValidateFlow_UnknownEdgeRef_Invalid(t *testing.T) {
	fake := seedFake()
	fake.def = flowspec.FlowDef{
		Nodes: []flowspec.NodeDef{{ID: "t1", Type: "trigger.manual"}},
		Edges: []flowspec.EdgeDef{{Source: "t1", Target: "ghost"}},
	}
	r := routerWithRunner(fake, &fakeRunner{})
	w, body := doReq(t, r, http.MethodPost, "/api/automate/v1/flows/flw_payroll/validate", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if body["valid"] != false {
		t.Errorf("valid = %v, want false", body["valid"])
	}
	if !hasBodyIssue(issueList(t, body), "ghost", "error", "edge references unknown target node") {
		t.Errorf("want unknown-target error, got %v", body["issues"])
	}
}

func TestValidateFlow_DuplicateIDsAndEmptyType_Invalid(t *testing.T) {
	fake := seedFake()
	fake.def = flowspec.FlowDef{
		Nodes: []flowspec.NodeDef{
			{ID: "t1", Type: "trigger.manual"},
			{ID: "dup", Type: "db.query"},
			{ID: "dup", Type: ""}, // duplicate id AND empty type
		},
		Edges: []flowspec.EdgeDef{{Source: "t1", Target: "dup"}},
	}
	r := routerWithRunner(fake, &fakeRunner{})
	_, body := doReq(t, r, http.MethodPost, "/api/automate/v1/flows/flw_payroll/validate", nil)
	issues := issueList(t, body)
	if !hasBodyIssue(issues, "dup", "error", "duplicate node id") {
		t.Errorf("want duplicate-id error, got %v", body["issues"])
	}
	if !hasBodyIssue(issues, "dup", "error", "node has empty type") {
		t.Errorf("want empty-type error, got %v", body["issues"])
	}
	if body["valid"] != false {
		t.Errorf("valid = %v, want false", body["valid"])
	}
}

func TestValidateFlow_NoDefinition_404(t *testing.T) {
	fake := seedFake()
	fake.defMissing = true
	r := routerWithRunner(fake, &fakeRunner{})
	w, body := doReq(t, r, http.MethodPost, "/api/automate/v1/flows/flw_newhire/validate", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
	assertErrorEnvelope(t, body, "not_found")
}

func TestValidateFlow_StoreError_500(t *testing.T) {
	fake := seedFake()
	fake.errDef = errors.New("db down")
	r := routerWithRunner(fake, &fakeRunner{})
	w, body := doReq(t, r, http.MethodPost, "/api/automate/v1/flows/flw_payroll/validate", nil)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	assertErrorEnvelope(t, body, "internal")
}

func TestValidateFlow_Forbidden_WhenNoFlowView(t *testing.T) {
	fake := seedFake()
	fake.def = sampleDef()
	r := routerWithRunner(fake, &fakeRunner{})
	// A role with no permissions (unknown) must be denied by RequirePermission.
	w, _ := doReq(t, r, http.MethodPost, "/api/automate/v1/flows/flw_payroll/validate",
		map[string]string{"X-Role": "nobody"})
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
}

// The issues array serialises as [] (never null) for a clean flow, so the UI can
// iterate it unconditionally.
func TestValidateFlow_IssuesAlwaysArray(t *testing.T) {
	fake := seedFake()
	fake.def = sampleDef()
	r := routerWithRunner(fake, &fakeRunner{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/automate/v1/flows/flw_payroll/validate", nil)
	r.ServeHTTP(w, req)
	var parsed struct {
		Valid  bool              `json:"valid"`
		Issues []json.RawMessage `json:"issues"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("unmarshal: %v (%s)", err, w.Body.String())
	}
	if parsed.Issues == nil {
		t.Errorf("issues should serialise as [], got null: %s", w.Body.String())
	}
}
