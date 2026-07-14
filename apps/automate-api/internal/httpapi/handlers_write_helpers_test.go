package httpapi

import (
	"errors"
	"net/http"
	"testing"

	"github.com/mywork/automate/internal/flowspec"
)

func TestToInt64(t *testing.T) {
	cases := []struct {
		in   any
		want int64
		ok   bool
	}{
		{int64(7), 7, true},
		{int(5), 5, true},
		{float64(3), 3, true},
		{"nope", 0, false},
		{nil, 0, false},
	}
	for _, c := range cases {
		got, ok := toInt64(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("toInt64(%#v) = %d,%v want %d,%v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestOutputCount(t *testing.T) {
	if n := outputCount(flowspec.NodeOutput{Meta: map[string]any{"rowCount": float64(9)}}); n != 9 {
		t.Errorf("rowCount meta: got %d", n)
	}
	if n := outputCount(flowspec.NodeOutput{Meta: map[string]any{"rowCount": "x"}, Items: []map[string]any{{}, {}}}); n != 2 {
		t.Errorf("non-numeric rowCount should fall back to len(items): got %d", n)
	}
	if n := outputCount(flowspec.NodeOutput{Items: []map[string]any{{}}}); n != 1 {
		t.Errorf("no meta: got %d", n)
	}
}

func TestNodeNameAndType(t *testing.T) {
	def := flowspec.FlowDef{Nodes: []flowspec.NodeDef{
		{ID: "a", Type: "db.query", Name: "Query"},
		{ID: "b", Type: "noop", Name: ""},
	}}
	if nodeName(def, "a") != "Query" {
		t.Error("named node")
	}
	if nodeName(def, "b") != "b" {
		t.Error("empty name should fall back to id")
	}
	if nodeName(def, "missing") != "missing" {
		t.Error("unknown node should return id")
	}
	if nodeType(def, "a") != "db.query" {
		t.Error("type")
	}
	if nodeType(def, "missing") != "" {
		t.Error("unknown type should be empty")
	}
}

func TestBuildSteps_NothingRan_SynthesisesFailedStep(t *testing.T) {
	steps := buildSteps(sampleDef(), flowspec.FlowResult{Path: nil}, errors.New("no trigger"))
	if len(steps) != 1 || steps[0].NodeID != "flow" || steps[0].Status != "failed" || steps[0].Error == nil {
		t.Fatalf("expected one synthetic failed step, got %+v", steps)
	}
}

// runFlow error branches
func TestRunFlow_BadJSON_400(t *testing.T) {
	fake := seedFake()
	fake.def = sampleDef()
	r := routerWithRunner(fake, &fakeRunner{})
	w, _ := doReqBody(t, r, http.MethodPost, "/api/automate/v1/flows/flw_payroll/run", `{bad`, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestRunFlow_CreateExecutionError_500(t *testing.T) {
	fake := seedFake()
	fake.def = sampleDef()
	fake.errWrite = errors.New("db down")
	r := routerWithRunner(fake, &fakeRunner{})
	w, _ := doReq(t, r, http.MethodPost, "/api/automate/v1/flows/flw_payroll/run", nil)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}

func TestRunFlow_RunnerStartError_500(t *testing.T) {
	fake := seedFake()
	fake.def = sampleDef()
	r := routerWithRunner(fake, &fakeRunner{startErr: errors.New("temporal gone")})
	w, _ := doReq(t, r, http.MethodPost, "/api/automate/v1/flows/flw_payroll/run", nil)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}

func TestRunFlow_RoleHeaderRespected(t *testing.T) {
	fake := seedFake()
	fake.def = sampleDef()
	r := routerWithRunner(fake, &fakeRunner{result: flowspec.FlowResult{Path: []string{"t1"}}})
	w, _ := doReq(t, r, http.MethodPost, "/api/automate/v1/flows/flw_payroll/run", map[string]string{"X-Role": "operator"})
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", w.Code)
	}
}

func TestCreateFlow_BadJSON_400(t *testing.T) {
	r := routerWithRunner(seedFake(), &fakeRunner{})
	w, _ := doReqBody(t, r, http.MethodPost, "/api/automate/v1/flows", `{bad`, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestCreateFlow_StoreError_500(t *testing.T) {
	fake := seedFake()
	fake.errWrite = errors.New("db down")
	r := routerWithRunner(fake, &fakeRunner{})
	w, _ := doReqBody(t, r, http.MethodPost, "/api/automate/v1/flows", `{"name":"x","folder":"y"}`, nil)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}

func TestUpdateDraft_BadJSON_400(t *testing.T) {
	r := routerWithRunner(seedFake(), &fakeRunner{})
	w, _ := doReqBody(t, r, http.MethodPut, "/api/automate/v1/flows/flw_payroll/draft", `{bad`, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestCreateConnection_BadJSON_And_StoreError(t *testing.T) {
	r := routerWithRunner(seedFake(), &fakeRunner{})
	w, _ := doReqBody(t, r, http.MethodPost, "/api/automate/v1/connections", `{bad`, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad json = %d, want 400", w.Code)
	}
	fake := seedFake()
	fake.errWrite = errors.New("db down")
	r2 := routerWithRunner(fake, &fakeRunner{})
	w2, _ := doReqBody(t, r2, http.MethodPost, "/api/automate/v1/connections", `{"name":"n","type":"postgres"}`, nil)
	if w2.Code != http.StatusInternalServerError {
		t.Fatalf("store err = %d, want 500", w2.Code)
	}
}

func TestUpdateConnection_BadJSON_400(t *testing.T) {
	r := routerWithRunner(seedFake(), &fakeRunner{})
	w, _ := doReqBody(t, r, http.MethodPut, "/api/automate/v1/connections/conn_hr", `{bad`, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestRunFlow_GetDefinitionError_500(t *testing.T) {
	fake := seedFake()
	fake.errDef = errors.New("db down")
	r := routerWithRunner(fake, &fakeRunner{})
	w, _ := doReq(t, r, http.MethodPost, "/api/automate/v1/flows/flw_payroll/run", nil)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}

func TestRunFlow_FlowNotFound_404(t *testing.T) {
	fake := seedFake()
	fake.def = sampleDef() // definition resolves, but the flow id is unknown
	r := routerWithRunner(fake, &fakeRunner{})
	w, _ := doReq(t, r, http.MethodPost, "/api/automate/v1/flows/does_not_exist/run", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestRunFlow_GetFlowError_500(t *testing.T) {
	fake := seedFake()
	fake.def = sampleDef()
	fake.errGetFlow = errors.New("db down")
	r := routerWithRunner(fake, &fakeRunner{})
	w, _ := doReq(t, r, http.MethodPost, "/api/automate/v1/flows/flw_payroll/run", nil)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}

// 500 branches on the flow/connection write methods.
func TestWriteMethods_StoreErrors_500(t *testing.T) {
	mk := func() (http.Handler, *fakeStore) {
		f := seedFake()
		f.errWrite = errors.New("db down")
		return routerWithRunner(f, &fakeRunner{}), f
	}
	r1, _ := mk()
	if w, _ := doReqBody(t, r1, http.MethodPut, "/api/automate/v1/flows/flw_payroll/draft", `{"definition":{"nodes":[],"edges":[]}}`, nil); w.Code != http.StatusInternalServerError {
		t.Errorf("updateDraft = %d, want 500", w.Code)
	}
	r2, _ := mk()
	if w, _ := doReq(t, r2, http.MethodPost, "/api/automate/v1/flows/flw_payroll/publish", nil); w.Code != http.StatusInternalServerError {
		t.Errorf("publish = %d, want 500", w.Code)
	}
	r3, _ := mk()
	if w, _ := doReqBody(t, r3, http.MethodPut, "/api/automate/v1/connections/conn_hr", `{"name":"n","type":"postgres"}`, nil); w.Code != http.StatusInternalServerError {
		t.Errorf("updateConn = %d, want 500", w.Code)
	}
	r4, _ := mk()
	if w, _ := doReq(t, r4, http.MethodDelete, "/api/automate/v1/connections/conn_hr", nil); w.Code != http.StatusInternalServerError {
		t.Errorf("deleteConn = %d, want 500", w.Code)
	}
}
