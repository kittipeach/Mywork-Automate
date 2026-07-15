package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mywork/automate/apps/automate-api/internal/audit"
	"github.com/mywork/automate/apps/automate-api/internal/notify"
	"github.com/mywork/automate/apps/automate-api/internal/preview"
	"github.com/mywork/automate/apps/automate-api/internal/runner"
	"github.com/mywork/automate/apps/automate-api/internal/scheduler"
	"github.com/mywork/automate/apps/automate-api/internal/store"
	"github.com/mywork/automate/internal/config"
	"github.com/mywork/automate/internal/flowspec"
)

// doReqBody performs a request with a JSON body and returns the recorder + parsed body.
func doReqBody(t *testing.T, r http.Handler, method, path, jsonBody string, headers map[string]string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var body map[string]any
	if w.Body.Len() > 0 {
		_ = json.Unmarshal(w.Body.Bytes(), &body)
	}
	return w, body
}

// --- fakeStore write methods (complete the store.Store interface) ---

func (f *fakeStore) GetFlowDefinition(_ context.Context, id string) (flowspec.FlowDef, error) {
	if f.errDef != nil {
		return flowspec.FlowDef{}, f.errDef
	}
	if f.defMissing {
		return flowspec.FlowDef{}, store.NewNotFound("flow has no definition: " + id)
	}
	return f.def, nil
}

func (f *fakeStore) CreateExecution(_ context.Context, e store.Execution) error {
	if f.errWrite != nil {
		return f.errWrite
	}
	f.created = append(f.created, e)
	return nil
}

func (f *fakeStore) FinishExecution(_ context.Context, id, status string, durationMs int64, steps []store.ExecutionStep) error {
	f.finished = append(f.finished, finishCall{id: id, status: status, durationMs: durationMs, steps: steps})
	return nil
}

func (f *fakeStore) SetExecutionStatus(_ context.Context, id, status string) error {
	if f.errSetExecStatus != nil {
		return f.errSetExecStatus
	}
	if f.execStatusNotFound {
		return store.NewNotFound("execution not found: " + id)
	}
	f.execStatusSets = append(f.execStatusSets, statusSet{id: id, status: status})
	for i := range f.execs {
		if f.execs[i].ID == id {
			f.execs[i].Status = status
		}
	}
	return nil
}

func (f *fakeStore) CreateFlow(_ context.Context, name, folder string) (store.FlowSummary, error) {
	if f.errWrite != nil {
		return store.FlowSummary{}, f.errWrite
	}
	fl := store.FlowSummary{ID: "flw_new", Name: name, Folder: folder, Status: "draft", Version: 0}
	f.flows = append(f.flows, fl)
	return fl, nil
}

func (f *fakeStore) UpdateFlowDefinition(_ context.Context, id string, _ flowspec.FlowDef) error {
	if f.errWrite != nil {
		return f.errWrite
	}
	if f.writeNotFound {
		return store.NewNotFound("flow not found: " + id)
	}
	for _, fl := range f.flows {
		if fl.ID == id {
			return nil
		}
	}
	return store.NewNotFound("flow not found: " + id)
}

func (f *fakeStore) PublishFlow(_ context.Context, id string) (store.FlowSummary, error) {
	if f.errPublish != nil {
		return store.FlowSummary{}, f.errPublish
	}
	if f.errWrite != nil {
		return store.FlowSummary{}, f.errWrite
	}
	for _, fl := range f.flows {
		if fl.ID == id {
			fl.Version++
			fl.Status = "published"
			return fl, nil
		}
	}
	return store.FlowSummary{}, store.NewNotFound("flow not found: " + id)
}

func (f *fakeStore) SetFlowStatus(_ context.Context, id, status string) error {
	if f.errWrite != nil {
		return f.errWrite
	}
	if f.writeNotFound {
		return store.NewNotFound("flow not found: " + id)
	}
	for i := range f.flows {
		if f.flows[i].ID == id {
			f.statusSets = append(f.statusSets, statusSet{id: id, status: status})
			f.flows[i].Status = status
			return nil
		}
	}
	return store.NewNotFound("flow not found: " + id)
}

func (f *fakeStore) CreateVersion(_ context.Context, flowID string, versionNo int, def flowspec.FlowDef, changeNote, publishedBy string) error {
	if f.errCreateVersion != nil {
		return f.errCreateVersion
	}
	f.versionsCreated = append(f.versionsCreated, versionCreate{
		flowID: flowID, versionNo: versionNo, def: def, changeNote: changeNote, publishedBy: publishedBy,
	})
	return nil
}

func (f *fakeStore) ListVersions(_ context.Context, flowID string) ([]store.Version, error) {
	if f.errVersions != nil {
		return nil, f.errVersions
	}
	out := make([]store.Version, 0)
	for _, v := range f.versions {
		if v.flowID == flowID {
			out = append(out, v.v)
		}
	}
	return out, nil
}

func (f *fakeStore) GetVersionDefinition(_ context.Context, flowID string, versionNo int) (flowspec.FlowDef, error) {
	if f.errGetVersion != nil {
		return flowspec.FlowDef{}, f.errGetVersion
	}
	for _, v := range f.versions {
		if v.flowID == flowID && v.v.VersionNo == versionNo {
			return v.def, nil
		}
	}
	return flowspec.FlowDef{}, store.NewNotFound("version not found")
}

func (f *fakeStore) CreateConnection(_ context.Context, in store.ConnectionInput) (store.Connection, error) {
	if f.errWrite != nil {
		return store.Connection{}, f.errWrite
	}
	return store.Connection{ID: "conn_new", Name: in.Name, Type: in.Type, Host: in.Host, Status: "untested", AllowedRoles: in.AllowedRoles}, nil
}

func (f *fakeStore) UpdateConnection(_ context.Context, id string, in store.ConnectionInput) (store.Connection, error) {
	if f.errWrite != nil {
		return store.Connection{}, f.errWrite
	}
	for _, c := range f.conns {
		if c.ID == id {
			return store.Connection{ID: id, Name: in.Name, Type: in.Type, Host: in.Host, Status: c.Status, AllowedRoles: in.AllowedRoles}, nil
		}
	}
	return store.Connection{}, store.NewNotFound("connection not found: " + id)
}

func (f *fakeStore) DeleteConnection(_ context.Context, id string) error {
	if f.errWrite != nil {
		return f.errWrite
	}
	for _, c := range f.conns {
		if c.ID == id {
			return nil
		}
	}
	return store.NewNotFound("connection not found: " + id)
}

// --- fake Runner: invokes onDone synchronously so recording is deterministic ---

type fakeRunner struct {
	result    flowspec.FlowResult
	runErr    error // error delivered via onDone (workflow failed)
	startErr  error // error returned from Run (failed to start)
	started   int
	cancelled []string // execution ids passed to Cancel
	cancelErr error    // error returned from Cancel
}

func (r *fakeRunner) Run(_ context.Context, _ string, _ flowspec.FlowInput, onDone func(flowspec.FlowResult, error)) error {
	r.started++
	if r.startErr != nil {
		return r.startErr
	}
	onDone(r.result, r.runErr)
	return nil
}

func (r *fakeRunner) Cancel(_ context.Context, executionID string) error {
	if r.cancelErr != nil {
		return r.cancelErr
	}
	r.cancelled = append(r.cancelled, executionID)
	return nil
}

func routerWithRunner(st store.Store, run runner.Runner) http.Handler {
	return NewRouter(config.Config{Env: config.EnvDev, FileStore: config.FileStoreLocal}, st, run, nil, nil, AuthConfig{}, nil, nil)
}

// routerWithDeps builds a router with an explicit audit.Service and Scheduler so
// the audit/scheduler wiring can be asserted. A nil auditSvc/sched falls back to
// the Noop implementations inside NewRouter.
func routerWithDeps(st store.Store, run runner.Runner, auditSvc audit.Service, sched scheduler.Scheduler) http.Handler {
	return NewRouter(config.Config{Env: config.EnvDev, FileStore: config.FileStoreLocal}, st, run, auditSvc, sched, AuthConfig{}, nil, nil)
}

// routerWithQuerier builds a router with an injected preview querier so the
// query-preview and schema endpoints can be exercised with a fake pool.
func routerWithQuerier(st store.Store, run runner.Runner, q preview.Querier) http.Handler {
	return NewRouter(config.Config{Env: config.EnvDev, FileStore: config.FileStoreLocal}, st, run, nil, nil, AuthConfig{}, q, nil)
}

// routerWithNotifier builds a router with an injected Notifier so the
// run-failure notification wiring (E5-S6) can be asserted from onDone.
func routerWithNotifier(st store.Store, run runner.Runner, n notify.Notifier) http.Handler {
	return NewRouter(config.Config{Env: config.EnvDev, FileStore: config.FileStoreLocal}, st, run, nil, nil, AuthConfig{}, nil, n)
}

// --- tests ---

func sampleDef() flowspec.FlowDef {
	return flowspec.FlowDef{
		Nodes: []flowspec.NodeDef{
			{ID: "t1", Type: "trigger.manual", Name: "Manual"},
			{ID: "q1", Type: "db.query", Name: "Query"},
		},
		Edges: []flowspec.EdgeDef{{Source: "t1", Target: "q1"}},
	}
}

func TestRunFlow_HappyPath_RecordsExecutionAndSteps(t *testing.T) {
	fake := seedFake()
	fake.def = sampleDef()
	run := &fakeRunner{result: flowspec.FlowResult{
		Path:    []string{"t1", "q1"},
		Outputs: map[string]flowspec.NodeOutput{"q1": {Meta: map[string]any{"rowCount": float64(3)}}},
	}}
	r := routerWithRunner(fake, run)

	w, body := doReq(t, r, http.MethodPost, "/api/automate/v1/flows/flw_payroll/run", nil)
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", w.Code)
	}
	if body["executionId"] == nil {
		t.Fatal("missing executionId in response")
	}
	if len(fake.created) != 1 || fake.created[0].Status != "running" {
		t.Fatalf("expected 1 running execution created, got %+v", fake.created)
	}
	if len(fake.finished) != 1 {
		t.Fatalf("expected FinishExecution called once, got %d", len(fake.finished))
	}
	fin := fake.finished[0]
	if fin.status != "success" {
		t.Errorf("finish status = %q, want success", fin.status)
	}
	if len(fin.steps) != 2 || fin.steps[1].NodeType != "db.query" || fin.steps[1].OutputCount != 3 {
		t.Errorf("steps not mapped from FlowResult: %+v", fin.steps)
	}
}

func TestRunFlow_WorkflowError_MarksFailed(t *testing.T) {
	fake := seedFake()
	fake.def = sampleDef()
	run := &fakeRunner{result: flowspec.FlowResult{Path: []string{"t1"}}, runErr: context.DeadlineExceeded}
	r := routerWithRunner(fake, run)

	w, _ := doReq(t, r, http.MethodPost, "/api/automate/v1/flows/flw_payroll/run", nil)
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", w.Code)
	}
	if len(fake.finished) != 1 || fake.finished[0].status != "failed" {
		t.Fatalf("expected a failed FinishExecution, got %+v", fake.finished)
	}
	last := fake.finished[0].steps[len(fake.finished[0].steps)-1]
	if last.Status != "failed" || last.Error == nil {
		t.Errorf("last step should be failed with an error, got %+v", last)
	}
}

func TestRunFlow_NoRunner_503(t *testing.T) {
	r := routerWithRunner(seedFake(), nil)
	w, _ := doReq(t, r, http.MethodPost, "/api/automate/v1/flows/flw_payroll/run", nil)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}

func TestRunFlow_DefinitionMissing_404(t *testing.T) {
	fake := seedFake()
	fake.defMissing = true
	r := routerWithRunner(fake, &fakeRunner{})
	w, _ := doReq(t, r, http.MethodPost, "/api/automate/v1/flows/flw_x/run", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestCreateFlow(t *testing.T) {
	fake := seedFake()
	r := routerWithRunner(fake, &fakeRunner{})
	w, body := doReqBody(t, r, http.MethodPost, "/api/automate/v1/flows", `{"name":"Monthly Tax","folder":"Finance"}`, nil)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", w.Code)
	}
	if body["name"] != "Monthly Tax" {
		t.Errorf("name = %v", body["name"])
	}
}

func TestCreateFlow_MissingName_400(t *testing.T) {
	r := routerWithRunner(seedFake(), &fakeRunner{})
	w, _ := doReqBody(t, r, http.MethodPost, "/api/automate/v1/flows", `{"folder":"x"}`, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestUpdateDraft_And_NotFound(t *testing.T) {
	r := routerWithRunner(seedFake(), &fakeRunner{})
	w, _ := doReqBody(t, r, http.MethodPut, "/api/automate/v1/flows/flw_payroll/draft", `{"definition":{"nodes":[],"edges":[]}}`, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	w2, _ := doReqBody(t, r, http.MethodPut, "/api/automate/v1/flows/nope/draft", `{"definition":{"nodes":[],"edges":[]}}`, nil)
	if w2.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w2.Code)
	}
}

func TestPublishFlow_And_NotFound(t *testing.T) {
	r := routerWithRunner(seedFake(), &fakeRunner{})
	w, body := doReqBody(t, r, http.MethodPost, "/api/automate/v1/flows/flw_payroll/publish", `{"changeNote":"initial"}`, nil)
	if w.Code != http.StatusOK || body["status"] != "published" {
		t.Fatalf("publish = %d %v", w.Code, body["status"])
	}
	w2, _ := doReqBody(t, r, http.MethodPost, "/api/automate/v1/flows/nope/publish", `{"changeNote":"initial"}`, nil)
	if w2.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w2.Code)
	}
}

func TestConnectionCRUD(t *testing.T) {
	r := routerWithRunner(seedFake(), &fakeRunner{})
	// create
	w, _ := doReqBody(t, r, http.MethodPost, "/api/automate/v1/connections", `{"name":"New PG","type":"postgres","host":"h:5432"}`, nil)
	if w.Code != http.StatusCreated {
		t.Fatalf("create = %d, want 201", w.Code)
	}
	// create missing type -> 400
	wb, _ := doReqBody(t, r, http.MethodPost, "/api/automate/v1/connections", `{"name":"x"}`, nil)
	if wb.Code != http.StatusBadRequest {
		t.Fatalf("create bad = %d, want 400", wb.Code)
	}
	// update existing + not found
	wu, _ := doReqBody(t, r, http.MethodPut, "/api/automate/v1/connections/conn_hr", `{"name":"HR2","type":"postgres","host":"h"}`, nil)
	if wu.Code != http.StatusOK {
		t.Fatalf("update = %d, want 200", wu.Code)
	}
	wnf, _ := doReqBody(t, r, http.MethodPut, "/api/automate/v1/connections/nope", `{"name":"x","type":"postgres"}`, nil)
	if wnf.Code != http.StatusNotFound {
		t.Fatalf("update nf = %d, want 404", wnf.Code)
	}
	// delete + not found
	wd, _ := doReq(t, r, http.MethodDelete, "/api/automate/v1/connections/conn_hr", nil)
	if wd.Code != http.StatusNoContent {
		t.Fatalf("delete = %d, want 204", wd.Code)
	}
	wdn, _ := doReq(t, r, http.MethodDelete, "/api/automate/v1/connections/nope", nil)
	if wdn.Code != http.StatusNotFound {
		t.Fatalf("delete nf = %d, want 404", wdn.Code)
	}
	// test connection stub
	wt, tb := doReq(t, r, http.MethodPost, "/api/automate/v1/connections/conn_hr/test", nil)
	if wt.Code != http.StatusOK || tb["status"] != "ok" {
		t.Fatalf("test = %d %v", wt.Code, tb["status"])
	}
}

// --- E5-S6: run-failure notifications wired into onDone ---

// fakeNotifier records every RunFailed call so the onDone wiring can be asserted,
// and can be primed to return an error (proving a notify failure is non-fatal).
type fakeNotifier struct {
	calls []notify.FailureNotice
	err   error
}

func (n *fakeNotifier) RunFailed(_ context.Context, in notify.FailureNotice) error {
	n.calls = append(n.calls, in)
	return n.err
}

// defWithFailureEmails is sampleDef plus a per-flow failure-recipient list
// (flowspec Settings.Notification.FailureEmails).
func defWithFailureEmails(emails ...string) flowspec.FlowDef {
	def := sampleDef()
	def.Settings = &flowspec.Settings{
		Notification: &flowspec.NotificationSettings{FailureEmails: emails},
	}
	return def
}

func TestRunFlow_Failed_NotifiesConfiguredRecipients(t *testing.T) {
	fake := seedFake()
	fake.def = defWithFailureEmails("ops@mywork.local", "oncall@mywork.local")
	run := &fakeRunner{result: flowspec.FlowResult{Path: []string{"t1"}}, runErr: context.DeadlineExceeded}
	notifier := &fakeNotifier{}
	r := routerWithNotifier(fake, run, notifier)

	w, _ := doReq(t, r, http.MethodPost, "/api/automate/v1/flows/flw_payroll/run", nil)
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", w.Code)
	}
	if len(notifier.calls) != 1 {
		t.Fatalf("expected exactly one RunFailed call, got %d", len(notifier.calls))
	}
	got := notifier.calls[0]
	if got.FlowName != "Payroll → Bank MFT" {
		t.Errorf("flow name = %q, want the flow's name", got.FlowName)
	}
	if got.Error != context.DeadlineExceeded.Error() {
		t.Errorf("error = %q, want the run error message", got.Error)
	}
	if len(got.Recipients) != 2 || got.Recipients[0] != "ops@mywork.local" || got.Recipients[1] != "oncall@mywork.local" {
		t.Errorf("recipients = %v, want the def's FailureEmails", got.Recipients)
	}
	// The execution id notified must be the one recorded as running.
	if len(fake.created) != 1 || got.ExecutionID != fake.created[0].ID {
		t.Errorf("notified execID = %q, want the created execution id", got.ExecutionID)
	}
}

func TestRunFlow_Success_DoesNotNotify(t *testing.T) {
	fake := seedFake()
	fake.def = defWithFailureEmails("ops@mywork.local")
	run := &fakeRunner{result: flowspec.FlowResult{Path: []string{"t1", "q1"}}} // no runErr → success
	notifier := &fakeNotifier{}
	r := routerWithNotifier(fake, run, notifier)

	w, _ := doReq(t, r, http.MethodPost, "/api/automate/v1/flows/flw_payroll/run", nil)
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", w.Code)
	}
	if len(notifier.calls) != 0 {
		t.Fatalf("a successful run must not notify, got %d calls", len(notifier.calls))
	}
}

func TestRunFlow_Failed_NoNotificationSettings_DoesNotNotify(t *testing.T) {
	fake := seedFake()
	fake.def = sampleDef() // no Settings at all
	run := &fakeRunner{result: flowspec.FlowResult{Path: []string{"t1"}}, runErr: context.DeadlineExceeded}
	notifier := &fakeNotifier{}
	r := routerWithNotifier(fake, run, notifier)

	w, _ := doReq(t, r, http.MethodPost, "/api/automate/v1/flows/flw_payroll/run", nil)
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", w.Code)
	}
	if len(notifier.calls) != 0 {
		t.Fatalf("a flow with no notification settings must not notify, got %d calls", len(notifier.calls))
	}
}

func TestRunFlow_Failed_EmptyRecipients_DoesNotNotify(t *testing.T) {
	fake := seedFake()
	fake.def = defWithFailureEmails() // Notification present but no emails
	run := &fakeRunner{result: flowspec.FlowResult{Path: []string{"t1"}}, runErr: context.DeadlineExceeded}
	notifier := &fakeNotifier{}
	r := routerWithNotifier(fake, run, notifier)

	doReq(t, r, http.MethodPost, "/api/automate/v1/flows/flw_payroll/run", nil)
	if len(notifier.calls) != 0 {
		t.Fatalf("empty FailureEmails must not notify, got %d calls", len(notifier.calls))
	}
}

// A notify failure is non-fatal: the run is still recorded (FinishExecution ran)
// and the request still succeeds. Exercises the WARN branch in onDone.
func TestRunFlow_Failed_NotifyError_IsNonFatal(t *testing.T) {
	fake := seedFake()
	fake.def = defWithFailureEmails("ops@mywork.local")
	run := &fakeRunner{result: flowspec.FlowResult{Path: []string{"t1"}}, runErr: context.DeadlineExceeded}
	notifier := &fakeNotifier{err: errors.New("smtp unreachable")}
	r := routerWithNotifier(fake, run, notifier)

	w, _ := doReq(t, r, http.MethodPost, "/api/automate/v1/flows/flw_payroll/run", nil)
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202 even when notify fails", w.Code)
	}
	if len(fake.finished) != 1 || fake.finished[0].status != "failed" {
		t.Fatalf("run must still be recorded failed, got %+v", fake.finished)
	}
	if len(notifier.calls) != 1 {
		t.Fatalf("expected one RunFailed attempt, got %d", len(notifier.calls))
	}
}

// When no Notifier is injected, NewRouter substitutes notify.Noop: a failed run
// with recipients must still succeed (the Noop swallows the notice).
func TestRunFlow_Failed_NilNotifier_UsesNoop(t *testing.T) {
	fake := seedFake()
	fake.def = defWithFailureEmails("ops@mywork.local")
	run := &fakeRunner{result: flowspec.FlowResult{Path: []string{"t1"}}, runErr: context.DeadlineExceeded}
	r := routerWithRunner(fake, run) // nil notifier → Noop

	w, _ := doReq(t, r, http.MethodPost, "/api/automate/v1/flows/flw_payroll/run", nil)
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", w.Code)
	}
	if len(fake.finished) != 1 || fake.finished[0].status != "failed" {
		t.Fatalf("run must still be recorded failed, got %+v", fake.finished)
	}
}
