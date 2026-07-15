package httpapi

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/mywork/automate/apps/automate-api/internal/preview"
	"github.com/mywork/automate/internal/flowspec"
)

// --- fake preview.Querier for the handler tests ---

type fakePreviewRows struct {
	cols []string
	rows [][]any
	i    int
}

func (r *fakePreviewRows) Next() bool {
	if r.i >= len(r.rows) {
		return false
	}
	r.i++
	return true
}
func (r *fakePreviewRows) Values() ([]any, error) { return r.rows[r.i-1], nil }
func (r *fakePreviewRows) FieldDescriptions() []pgconn.FieldDescription {
	fds := make([]pgconn.FieldDescription, len(r.cols))
	for i, c := range r.cols {
		fds[i] = pgconn.FieldDescription{Name: c}
	}
	return fds
}
func (r *fakePreviewRows) Err() error { return nil }
func (r *fakePreviewRows) Close()     {}

type fakeQuerier struct {
	rows     *fakePreviewRows
	queryErr error
}

func (q *fakeQuerier) Query(_ context.Context, _ string, _ ...any) (preview.Rows, error) {
	if q.queryErr != nil {
		return nil, q.queryErr
	}
	// Reset the cursor so a querier reused across a schema+preview call replays.
	q.rows.i = 0
	return q.rows, nil
}

// --- cancel ---

func TestCancelExecution_Happy(t *testing.T) {
	fake := seedFake()
	run := &fakeRunner{}
	r := routerWithRunner(fake, run)

	// exe_1003 is "running" in the seed → cancellable.
	w, body := doReq(t, r, http.MethodPost, "/api/automate/v1/executions/exe_1003/cancel", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if body["status"] != "cancelled" {
		t.Errorf("status = %v, want cancelled", body["status"])
	}
	if len(run.cancelled) != 1 || run.cancelled[0] != "exe_1003" {
		t.Errorf("runner.Cancel not called with exe_1003: %v", run.cancelled)
	}
	if len(fake.execStatusSets) != 1 || fake.execStatusSets[0].status != "cancelled" {
		t.Errorf("SetExecutionStatus not called: %v", fake.execStatusSets)
	}
}

func TestCancelExecution_Terminal_409(t *testing.T) {
	fake := seedFake()
	run := &fakeRunner{}
	r := routerWithRunner(fake, run)

	// exe_1001 is "success" → terminal → 409.
	w, body := doReq(t, r, http.MethodPost, "/api/automate/v1/executions/exe_1001/cancel", nil)
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", w.Code)
	}
	assertErrorEnvelope(t, body, "conflict")
	if len(run.cancelled) != 0 {
		t.Errorf("runner.Cancel must not run for a terminal execution: %v", run.cancelled)
	}
}

func TestCancelExecution_Missing_404(t *testing.T) {
	r := routerWithRunner(seedFake(), &fakeRunner{})
	w, body := doReq(t, r, http.MethodPost, "/api/automate/v1/executions/nope/cancel", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
	assertErrorEnvelope(t, body, "not_found")
}

func TestCancelExecution_RBAC_403(t *testing.T) {
	// viewer has FlowView but not FlowRun → denied.
	r := routerWithRunner(seedFake(), &fakeRunner{})
	w, body := doReq(t, r, http.MethodPost, "/api/automate/v1/executions/exe_1003/cancel",
		map[string]string{roleHeader: "viewer"})
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
	assertErrorEnvelope(t, body, "forbidden")
}

func TestCancelExecution_NoRunner_503(t *testing.T) {
	r := routerWithRunner(seedFake(), nil)
	w, _ := doReq(t, r, http.MethodPost, "/api/automate/v1/executions/exe_1003/cancel", nil)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}

func TestCancelExecution_RunnerError_500(t *testing.T) {
	run := &fakeRunner{cancelErr: errors.New("temporal down")}
	r := routerWithRunner(seedFake(), run)
	w, body := doReq(t, r, http.MethodPost, "/api/automate/v1/executions/exe_1003/cancel", nil)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	assertErrorEnvelope(t, body, "internal")
}

func TestCancelExecution_SetStatusError_500(t *testing.T) {
	fake := seedFake()
	fake.errSetExecStatus = errors.New("db down")
	r := routerWithRunner(fake, &fakeRunner{})
	w, body := doReq(t, r, http.MethodPost, "/api/automate/v1/executions/exe_1003/cancel", nil)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	assertErrorEnvelope(t, body, "internal")
}

// --- retry ---

func TestRetryExecution_Happy_202(t *testing.T) {
	fake := seedFake()
	fake.def = sampleDef()
	run := &fakeRunner{result: sampleResult()}
	r := routerWithRunner(fake, run)

	// exe_1002 belongs to flw_leave.
	w, body := doReq(t, r, http.MethodPost, "/api/automate/v1/executions/exe_1002/retry", nil)
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", w.Code)
	}
	newID, _ := body["executionId"].(string)
	if newID == "" {
		t.Fatal("missing executionId")
	}
	// A brand-new execution was recorded (running → finished) for the source flow.
	if len(fake.created) != 1 || fake.created[0].FlowID != "flw_leave" {
		t.Fatalf("expected 1 new execution for flw_leave, got %+v", fake.created)
	}
	if fake.created[0].ID == "exe_1002" {
		t.Errorf("retry must create a NEW execution id, got the source id")
	}
	if len(fake.finished) != 1 {
		t.Errorf("expected FinishExecution once, got %d", len(fake.finished))
	}
}

func TestRetryExecution_Missing_404(t *testing.T) {
	fake := seedFake()
	fake.def = sampleDef()
	r := routerWithRunner(fake, &fakeRunner{})
	w, body := doReq(t, r, http.MethodPost, "/api/automate/v1/executions/nope/retry", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
	assertErrorEnvelope(t, body, "not_found")
}

func TestRetryExecution_FlowDefinitionMissing_404(t *testing.T) {
	fake := seedFake()
	fake.defMissing = true
	r := routerWithRunner(fake, &fakeRunner{})
	w, _ := doReq(t, r, http.MethodPost, "/api/automate/v1/executions/exe_1002/retry", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestRetryExecution_RBAC_403(t *testing.T) {
	fake := seedFake()
	fake.def = sampleDef()
	r := routerWithRunner(fake, &fakeRunner{})
	w, body := doReq(t, r, http.MethodPost, "/api/automate/v1/executions/exe_1002/retry",
		map[string]string{roleHeader: "viewer"})
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
	assertErrorEnvelope(t, body, "forbidden")
}

func TestRetryExecution_NoRunner_503(t *testing.T) {
	r := routerWithRunner(seedFake(), nil)
	w, _ := doReq(t, r, http.MethodPost, "/api/automate/v1/executions/exe_1002/retry", nil)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}

// --- query preview ---

func TestQueryPreview_Happy_MaskedItemsAndMeta(t *testing.T) {
	fake := seedFake()
	q := &fakeQuerier{rows: &fakePreviewRows{
		cols: []string{"name", "salary"},
		rows: [][]any{{"Alice", 50000}, {"Bob", 60000}},
	}}
	r := routerWithQuerier(fake, &fakeRunner{}, q)

	w, body := doReqBody(t, r, http.MethodPost, "/api/automate/v1/connections/conn_hr/query-preview",
		`{"sql":"SELECT name, salary FROM staff"}`, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	items, _ := body["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("items len = %d, want 2", len(items))
	}
	first := items[0].(map[string]any)
	if first["name"] != "Alice" {
		t.Errorf("name masked unexpectedly: %v", first["name"])
	}
	if first["salary"] != "******" {
		t.Errorf("salary must be masked, got %v", first["salary"])
	}
	meta, _ := body["meta"].(map[string]any)
	if meta == nil {
		t.Fatal("missing meta")
	}
	cols, _ := meta["columns"].([]any)
	if len(cols) != 2 || cols[0] != "name" || cols[1] != "salary" {
		t.Errorf("columns = %v", cols)
	}
	if meta["rowCount"].(float64) != 2 {
		t.Errorf("rowCount = %v, want 2", meta["rowCount"])
	}
	if meta["truncated"].(bool) {
		t.Errorf("truncated = true, want false")
	}
}

func TestQueryPreview_InvalidSQL_400(t *testing.T) {
	fake := seedFake()
	q := &fakeQuerier{rows: &fakePreviewRows{}}
	r := routerWithQuerier(fake, &fakeRunner{}, q)

	w, body := doReqBody(t, r, http.MethodPost, "/api/automate/v1/connections/conn_hr/query-preview",
		`{"sql":"DELETE FROM staff"}`, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	assertErrorEnvelope(t, body, "invalid_sql")
}

func TestQueryPreview_CapsAt50(t *testing.T) {
	fake := seedFake()
	rows := make([][]any, 200)
	for i := range rows {
		rows[i] = []any{i}
	}
	q := &fakeQuerier{rows: &fakePreviewRows{cols: []string{"n"}, rows: rows}}
	r := routerWithQuerier(fake, &fakeRunner{}, q)

	// Request more than the hard cap; the response must clamp to 50 + truncated.
	w, body := doReqBody(t, r, http.MethodPost, "/api/automate/v1/connections/conn_hr/query-preview",
		`{"sql":"SELECT n FROM series","maxRows":1000}`, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	meta := body["meta"].(map[string]any)
	if meta["rowCount"].(float64) != float64(preview.MaxRows) {
		t.Errorf("rowCount = %v, want %d", meta["rowCount"], preview.MaxRows)
	}
	if !meta["truncated"].(bool) {
		t.Errorf("truncated = false, want true")
	}
}

func TestQueryPreview_RBAC_403(t *testing.T) {
	// auditor is an unknown role → lacks FlowView → denied.
	fake := seedFake()
	q := &fakeQuerier{rows: &fakePreviewRows{}}
	r := routerWithQuerier(fake, &fakeRunner{}, q)
	w, body := doReqBody(t, r, http.MethodPost, "/api/automate/v1/connections/conn_hr/query-preview",
		`{"sql":"SELECT 1"}`, map[string]string{roleHeader: "auditor"})
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
	assertErrorEnvelope(t, body, "forbidden")
}

func TestQueryPreview_NoQuerier_503(t *testing.T) {
	r := routerWithRunner(seedFake(), &fakeRunner{}) // nil querier
	w, _ := doReqBody(t, r, http.MethodPost, "/api/automate/v1/connections/conn_hr/query-preview",
		`{"sql":"SELECT 1"}`, nil)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}

func TestQueryPreview_BadJSON_400(t *testing.T) {
	fake := seedFake()
	q := &fakeQuerier{rows: &fakePreviewRows{}}
	r := routerWithQuerier(fake, &fakeRunner{}, q)
	w, body := doReqBody(t, r, http.MethodPost, "/api/automate/v1/connections/conn_hr/query-preview",
		`{not json`, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	assertErrorEnvelope(t, body, "bad_request")
}

func TestQueryPreview_QueryError_500(t *testing.T) {
	fake := seedFake()
	q := &fakeQuerier{queryErr: errors.New("connection refused")}
	r := routerWithQuerier(fake, &fakeRunner{}, q)
	w, body := doReqBody(t, r, http.MethodPost, "/api/automate/v1/connections/conn_hr/query-preview",
		`{"sql":"SELECT 1"}`, nil)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	assertErrorEnvelope(t, body, "internal")
}

// --- schema metadata ---

func TestConnectionSchema_Happy(t *testing.T) {
	fake := seedFake()
	q := &fakeQuerier{rows: &fakePreviewRows{
		cols: []string{"table_name", "column_name", "data_type"},
		rows: [][]any{
			{"employees", "id", "integer"},
			{"employees", "name", "text"},
			{"payroll", "amount", "numeric"},
		},
	}}
	r := routerWithQuerier(fake, &fakeRunner{}, q)

	w, body := doReq(t, r, http.MethodGet, "/api/automate/v1/connections/conn_hr/schema", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	tables, _ := body["tables"].([]any)
	if len(tables) != 2 {
		t.Fatalf("tables = %d, want 2", len(tables))
	}
	emp := tables[0].(map[string]any)
	if emp["name"] != "employees" {
		t.Errorf("first table = %v, want employees", emp["name"])
	}
	cols, _ := emp["columns"].([]any)
	if len(cols) != 2 {
		t.Fatalf("employees columns = %d, want 2", len(cols))
	}
	col0 := cols[0].(map[string]any)
	if col0["name"] != "id" || col0["type"] != "integer" {
		t.Errorf("first column = %v", col0)
	}
}

func TestConnectionSchema_RBAC_403(t *testing.T) {
	fake := seedFake()
	q := &fakeQuerier{rows: &fakePreviewRows{cols: []string{"table_name", "column_name", "data_type"}}}
	r := routerWithQuerier(fake, &fakeRunner{}, q)
	w, body := doReq(t, r, http.MethodGet, "/api/automate/v1/connections/conn_hr/schema",
		map[string]string{roleHeader: "auditor"})
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
	assertErrorEnvelope(t, body, "forbidden")
}

func TestConnectionSchema_NoQuerier_503(t *testing.T) {
	r := routerWithRunner(seedFake(), &fakeRunner{})
	w, _ := doReq(t, r, http.MethodGet, "/api/automate/v1/connections/conn_hr/schema", nil)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}

func TestConnectionSchema_QueryError_500(t *testing.T) {
	fake := seedFake()
	q := &fakeQuerier{queryErr: errors.New("no db")}
	r := routerWithQuerier(fake, &fakeRunner{}, q)
	w, body := doReq(t, r, http.MethodGet, "/api/automate/v1/connections/conn_hr/schema", nil)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	assertErrorEnvelope(t, body, "internal")
}

// sampleResult is a minimal successful FlowResult for the retry happy path,
// exercising the shared run-and-record helper.
func sampleResult() flowspec.FlowResult {
	return flowspec.FlowResult{Path: []string{"t1", "q1"}}
}
