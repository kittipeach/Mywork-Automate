package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mywork/automate/apps/automate-api/internal/store"
	"github.com/mywork/automate/internal/config"
	"github.com/mywork/automate/internal/flowspec"
)

// fakeStore is an in-memory store.Store for handler tests. It applies the same
// filter semantics as the pgx implementation so the handler wiring is exercised
// end-to-end. The err* fields let a test force each method to return an error.
type fakeStore struct {
	flows []store.FlowSummary
	execs []store.Execution
	conns []store.Connection

	errFlows   error
	errGetFlow error
	errExecs   error
	errGetExec error
	errConns   error

	// write-path state/hooks
	def        flowspec.FlowDef
	defMissing bool
	errDef     error // non-notfound error from GetFlowDefinition
	created    []store.Execution
	finished   []finishCall
	errWrite   error // non-notfound error from any write method
}

type finishCall struct {
	id         string
	status     string
	durationMs int64
	steps      []store.ExecutionStep
}

func sp(s string) *string { return &s }

func seedFake() *fakeStore {
	return &fakeStore{
		flows: []store.FlowSummary{
			{ID: "flw_payroll", Name: "Payroll → Bank MFT", Folder: "Finance", Status: "published", Version: 3, UpdatedAt: "2026-07-12T23:00:00.000Z", LastRun: &store.LastRun{Status: "success", At: "2026-07-13T23:00:00.000Z"}},
			{ID: "flw_headcount", Name: "Daily Headcount → Email", Folder: "HR Ops", Status: "published", Version: 2, UpdatedAt: "2026-07-11T23:00:00.000Z", LastRun: &store.LastRun{Status: "success", At: "2026-07-13T23:00:00.000Z"}},
			{ID: "flw_leave", Name: "Leave Balance Report", Folder: "HR Ops", Status: "paused", Version: 1, UpdatedAt: "2026-07-08T23:00:00.000Z", LastRun: &store.LastRun{Status: "failed", At: "2026-07-10T23:00:00.000Z"}},
			{ID: "flw_newhire", Name: "New Hire Onboarding Export", Folder: "HR Ops", Status: "draft", Version: 0, UpdatedAt: "2026-07-13T23:00:00.000Z"},
			{ID: "flw_gov", Name: "Gov Submission (TIS-620)", Folder: "Compliance", Status: "stopped", Version: 4, UpdatedAt: "2026-07-04T23:00:00.000Z", LastRun: &store.LastRun{Status: "cancelled", At: "2026-07-05T23:00:00.000Z"}},
		},
		execs: []store.Execution{
			{ID: "exe_1001", FlowID: "flw_payroll", FlowName: "Payroll → Bank MFT", Status: "success", Trigger: "schedule", StartedAt: "2026-07-13T23:00:00.000Z", DurationMs: 48213, Version: 3, Steps: []store.ExecutionStep{
				{NodeID: "t1", NodeName: "Schedule 06:00", NodeType: "trigger.schedule", Status: "success", DurationMs: 5, InputCount: 0, OutputCount: 1},
				{NodeID: "q1", NodeName: "Query Payroll", NodeType: "db.query", Status: "success", DurationMs: 2103, InputCount: 1, OutputCount: 45012},
			}},
			{ID: "exe_1002", FlowID: "flw_leave", FlowName: "Leave Balance Report", Status: "failed", Trigger: "schedule", StartedAt: "2026-07-10T23:00:00.000Z", DurationMs: 9210, Version: 1, Steps: []store.ExecutionStep{
				{NodeID: "q1", NodeName: "Query Leave", NodeType: "db.query", Status: "failed", DurationMs: 9200, InputCount: 1, OutputCount: 0, Error: sp("connection timeout to hr-db")},
			}},
			{ID: "exe_1003", FlowID: "flw_headcount", FlowName: "Daily Headcount → Email", Status: "running", Trigger: "manual", StartedAt: "2026-07-14T02:00:00.000Z", DurationMs: 0, Version: 2, Steps: []store.ExecutionStep{}},
		},
		conns: []store.Connection{
			{ID: "conn_hr", Name: "HR Postgres (read-only)", Type: "postgres", Host: "hr-db.internal:5432", Status: "ok", AllowedRoles: []string{"admin", "designer"}},
			{ID: "conn_pay", Name: "Payroll Postgres", Type: "postgres", Host: "pay-db.internal:5432", Status: "ok", AllowedRoles: []string{"admin"}},
			{ID: "conn_bankmft", Name: "Bank MFT", Type: "sftp", Host: "mft.bank.co.th:22", Status: "ok", AllowedRoles: []string{"admin", "designer"}},
			{ID: "conn_smtp", Name: "Corp SMTP", Type: "smtp", Host: "smtp.mywork.co:587", Status: "untested", AllowedRoles: []string{"admin", "designer", "operator"}},
		},
	}
}

func (f *fakeStore) ListFlows(_ context.Context, filter store.FlowFilter) ([]store.FlowSummary, error) {
	if f.errFlows != nil {
		return nil, f.errFlows
	}
	out := make([]store.FlowSummary, 0)
	for _, fl := range f.flows {
		if filter.Q != "" && !strings.Contains(strings.ToLower(fl.Name), strings.ToLower(filter.Q)) {
			continue
		}
		if filter.Status != "" && fl.Status != filter.Status {
			continue
		}
		if filter.Folder != "" && fl.Folder != filter.Folder {
			continue
		}
		out = append(out, fl)
	}
	return out, nil
}

func (f *fakeStore) GetFlow(_ context.Context, id string) (store.FlowSummary, error) {
	if f.errGetFlow != nil {
		return store.FlowSummary{}, f.errGetFlow
	}
	for _, fl := range f.flows {
		if fl.ID == id {
			return fl, nil
		}
	}
	return store.FlowSummary{}, store.NewNotFound("flow not found: " + id)
}

func (f *fakeStore) ListExecutions(_ context.Context, filter store.ExecutionFilter) ([]store.Execution, error) {
	if f.errExecs != nil {
		return nil, f.errExecs
	}
	out := make([]store.Execution, 0)
	for _, e := range f.execs {
		if filter.Status != "" && e.Status != filter.Status {
			continue
		}
		if filter.FlowID != "" && e.FlowID != filter.FlowID {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

func (f *fakeStore) GetExecution(_ context.Context, id string) (store.Execution, error) {
	if f.errGetExec != nil {
		return store.Execution{}, f.errGetExec
	}
	for _, e := range f.execs {
		if e.ID == id {
			return e, nil
		}
	}
	return store.Execution{}, store.NewNotFound("execution not found: " + id)
}

func (f *fakeStore) ListConnections(_ context.Context, roles []string) ([]store.Connection, error) {
	if f.errConns != nil {
		return nil, f.errConns
	}
	out := make([]store.Connection, 0)
	for _, c := range f.conns {
		if len(c.AllowedRoles) == 0 || intersects(c.AllowedRoles, roles) {
			out = append(out, c)
		}
	}
	return out, nil
}

func intersects(a, b []string) bool {
	set := make(map[string]bool, len(a))
	for _, x := range a {
		set[x] = true
	}
	for _, y := range b {
		if set[y] {
			return true
		}
	}
	return false
}

func newTestRouter(st store.Store) http.Handler {
	return NewRouter(config.Config{Env: config.EnvDev, FileStore: config.FileStoreLocal}, st, nil)
}

// doReq performs a request with optional headers and returns the recorder plus
// the parsed JSON body as a generic map (nil when the body is empty).
func doReq(t *testing.T, r http.Handler, method, path string, headers map[string]string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	r.ServeHTTP(w, req)
	var body map[string]any
	if w.Body.Len() > 0 {
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("invalid JSON from %s %s: %v (%s)", method, path, err, w.Body.String())
		}
	}
	return w, body
}

func TestListFlows_All(t *testing.T) {
	r := newTestRouter(seedFake())
	w, body := doReq(t, r, http.MethodGet, APIBasePath+"/flows", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	flows, ok := body["flows"].([]any)
	if !ok {
		t.Fatalf("flows key missing/not array: %v", body)
	}
	if len(flows) != 5 {
		t.Fatalf("flows len = %d, want 5", len(flows))
	}
	// spot-check field names byte-for-byte on the first flow.
	first := flows[0].(map[string]any)
	for _, key := range []string{"id", "name", "folder", "status", "updatedAt", "version"} {
		if _, ok := first[key]; !ok {
			t.Errorf("flow missing key %q: %v", key, first)
		}
	}
	lastRun, ok := first["lastRun"].(map[string]any)
	if !ok {
		t.Fatalf("lastRun missing on published flow: %v", first)
	}
	if lastRun["status"] == nil || lastRun["at"] == nil {
		t.Errorf("lastRun shape wrong: %v", lastRun)
	}
}

func TestListFlows_DraftOmitsLastRun(t *testing.T) {
	r := newTestRouter(seedFake())
	_, body := doReq(t, r, http.MethodGet, APIBasePath+"/flows?status=draft", nil)
	flows := body["flows"].([]any)
	if len(flows) != 1 {
		t.Fatalf("draft flows len = %d, want 1", len(flows))
	}
	if _, present := flows[0].(map[string]any)["lastRun"]; present {
		t.Errorf("draft flow should omit lastRun, got %v", flows[0])
	}
}

func TestListFlows_Filters(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  int
	}{
		{"q substring case-insensitive", "?q=payroll", 1},
		{"q no match", "?q=zzz", 0},
		{"status published", "?status=published", 2},
		{"folder HR Ops", "?folder=HR+Ops", 3},
		{"combined status+folder", "?status=published&folder=HR+Ops", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestRouter(seedFake())
			w, body := doReq(t, r, http.MethodGet, APIBasePath+"/flows"+tt.query, nil)
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", w.Code)
			}
			flows, _ := body["flows"].([]any)
			if len(flows) != tt.want {
				t.Errorf("flows len = %d, want %d (query %s)", len(flows), tt.want, tt.query)
			}
		})
	}
}

func TestListFlows_StoreError(t *testing.T) {
	fs := seedFake()
	fs.errFlows = errors.New("boom")
	r := newTestRouter(fs)
	w, body := doReq(t, r, http.MethodGet, APIBasePath+"/flows", nil)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	assertErrorEnvelope(t, body, "internal")
}

func TestGetFlow_OK(t *testing.T) {
	r := newTestRouter(seedFake())
	w, body := doReq(t, r, http.MethodGet, APIBasePath+"/flows/flw_payroll", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if body["id"] != "flw_payroll" {
		t.Errorf("id = %v, want flw_payroll", body["id"])
	}
	if body["name"] != "Payroll → Bank MFT" {
		t.Errorf("name = %v", body["name"])
	}
}

func TestGetFlow_NotFound(t *testing.T) {
	r := newTestRouter(seedFake())
	w, body := doReq(t, r, http.MethodGet, APIBasePath+"/flows/nope", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
	assertErrorEnvelope(t, body, "not_found")
}

func TestGetFlow_StoreError(t *testing.T) {
	fs := seedFake()
	fs.errGetFlow = errors.New("db down")
	r := newTestRouter(fs)
	w, body := doReq(t, r, http.MethodGet, APIBasePath+"/flows/flw_payroll", nil)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	assertErrorEnvelope(t, body, "internal")
}

func TestListExecutions_All(t *testing.T) {
	r := newTestRouter(seedFake())
	w, body := doReq(t, r, http.MethodGet, APIBasePath+"/executions", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	execs, _ := body["executions"].([]any)
	if len(execs) != 3 {
		t.Fatalf("executions len = %d, want 3", len(execs))
	}
	first := execs[0].(map[string]any)
	for _, key := range []string{"id", "flowId", "flowName", "status", "trigger", "startedAt", "durationMs", "version", "steps"} {
		if _, ok := first[key]; !ok {
			t.Errorf("execution missing key %q: %v", key, first)
		}
	}
}

func TestListExecutions_Filters(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  int
	}{
		{"status failed", "?status=failed", 1},
		{"flowId payroll", "?flowId=flw_payroll", 1},
		{"status running", "?status=running", 1},
		{"combined no match", "?status=failed&flowId=flw_payroll", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestRouter(seedFake())
			_, body := doReq(t, r, http.MethodGet, APIBasePath+"/executions"+tt.query, nil)
			execs, _ := body["executions"].([]any)
			if len(execs) != tt.want {
				t.Errorf("execs len = %d, want %d (query %s)", len(execs), tt.want, tt.query)
			}
		})
	}
}

func TestListExecutions_StoreError(t *testing.T) {
	fs := seedFake()
	fs.errExecs = errors.New("boom")
	r := newTestRouter(fs)
	w, body := doReq(t, r, http.MethodGet, APIBasePath+"/executions", nil)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	assertErrorEnvelope(t, body, "internal")
}

func TestGetExecution_OKWithSteps(t *testing.T) {
	r := newTestRouter(seedFake())
	w, body := doReq(t, r, http.MethodGet, APIBasePath+"/executions/exe_1001", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	steps, ok := body["steps"].([]any)
	if !ok || len(steps) != 2 {
		t.Fatalf("steps = %v, want 2", body["steps"])
	}
	step := steps[0].(map[string]any)
	for _, key := range []string{"nodeId", "nodeName", "nodeType", "status", "durationMs", "inputCount", "outputCount"} {
		if _, ok := step[key]; !ok {
			t.Errorf("step missing key %q: %v", key, step)
		}
	}
	// success step must NOT carry an error key (omitempty).
	if _, present := step["error"]; present {
		t.Errorf("success step should omit error, got %v", step)
	}
}

func TestGetExecution_ErrorStepIncludesError(t *testing.T) {
	r := newTestRouter(seedFake())
	_, body := doReq(t, r, http.MethodGet, APIBasePath+"/executions/exe_1002", nil)
	steps := body["steps"].([]any)
	step := steps[0].(map[string]any)
	if step["error"] != "connection timeout to hr-db" {
		t.Errorf("error = %v, want connection timeout to hr-db", step["error"])
	}
}

func TestGetExecution_NotFound(t *testing.T) {
	r := newTestRouter(seedFake())
	w, body := doReq(t, r, http.MethodGet, APIBasePath+"/executions/nope", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
	assertErrorEnvelope(t, body, "not_found")
}

func TestGetExecution_StoreError(t *testing.T) {
	fs := seedFake()
	fs.errGetExec = errors.New("db down")
	r := newTestRouter(fs)
	w, body := doReq(t, r, http.MethodGet, APIBasePath+"/executions/exe_1001", nil)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	assertErrorEnvelope(t, body, "internal")
}

func TestListConnections_RBAC(t *testing.T) {
	tests := []struct {
		name    string
		role    string // "" means no header (defaults to admin)
		wantIDs []string
	}{
		{"no header defaults to admin sees all", "", []string{"conn_hr", "conn_pay", "conn_bankmft", "conn_smtp"}},
		{"admin sees all", "admin", []string{"conn_hr", "conn_pay", "conn_bankmft", "conn_smtp"}},
		{"designer excludes payroll-only", "designer", []string{"conn_hr", "conn_bankmft", "conn_smtp"}},
		{"operator sees only smtp", "operator", []string{"conn_smtp"}},
		{"unknown role sees none", "auditor", []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestRouter(seedFake())
			headers := map[string]string{}
			if tt.role != "" {
				headers[roleHeader] = tt.role
			}
			w, body := doReq(t, r, http.MethodGet, APIBasePath+"/connections", headers)
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", w.Code)
			}
			conns, _ := body["connections"].([]any)
			var gotIDs []string
			for _, c := range conns {
				gotIDs = append(gotIDs, c.(map[string]any)["id"].(string))
			}
			if !equalStrs(gotIDs, tt.wantIDs) {
				t.Errorf("ids = %v, want %v", gotIDs, tt.wantIDs)
			}
		})
	}
}

func TestListConnections_Shape(t *testing.T) {
	r := newTestRouter(seedFake())
	_, body := doReq(t, r, http.MethodGet, APIBasePath+"/connections", nil)
	conns := body["connections"].([]any)
	first := conns[0].(map[string]any)
	for _, key := range []string{"id", "name", "type", "host", "status", "allowedRoles"} {
		if _, ok := first[key]; !ok {
			t.Errorf("connection missing key %q: %v", key, first)
		}
	}
	if _, ok := first["allowedRoles"].([]any); !ok {
		t.Errorf("allowedRoles not an array: %v", first["allowedRoles"])
	}
}

func TestListConnections_StoreError(t *testing.T) {
	fs := seedFake()
	fs.errConns = errors.New("boom")
	r := newTestRouter(fs)
	w, body := doReq(t, r, http.MethodGet, APIBasePath+"/connections", nil)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	assertErrorEnvelope(t, body, "internal")
}

func TestListNodes(t *testing.T) {
	r := newTestRouter(seedFake())
	w, body := doReq(t, r, http.MethodGet, APIBasePath+"/nodes", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	ns, ok := body["nodes"].([]any)
	if !ok {
		t.Fatalf("nodes key missing/not array: %v", body)
	}
	if len(ns) != 9 {
		t.Fatalf("nodes len = %d, want 9", len(ns))
	}
	first := ns[0].(map[string]any)
	for _, key := range []string{"type", "category", "label", "description", "icon", "accent", "inputs", "outputs", "schema"} {
		if _, ok := first[key]; !ok {
			t.Errorf("node missing key %q: %v", key, first)
		}
	}
	if first["type"] != "trigger.schedule" {
		t.Errorf("first node type = %v, want trigger.schedule", first["type"])
	}
}

func assertErrorEnvelope(t *testing.T, body map[string]any, wantCode string) {
	t.Helper()
	env, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("missing error envelope: %v", body)
	}
	if env["code"] != wantCode {
		t.Errorf("error.code = %v, want %v", env["code"], wantCode)
	}
	if _, ok := env["message"].(string); !ok {
		t.Errorf("error.message missing/not string: %v", env)
	}
}

func equalStrs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
