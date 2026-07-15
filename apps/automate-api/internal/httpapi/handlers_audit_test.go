package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/mywork/automate/apps/automate-api/internal/audit"
	"github.com/mywork/automate/apps/automate-api/internal/auth"
	"github.com/mywork/automate/apps/automate-api/internal/runner"
	"github.com/mywork/automate/apps/automate-api/internal/scheduler"
	"github.com/mywork/automate/apps/automate-api/internal/store"
	"github.com/mywork/automate/internal/config"
	"github.com/mywork/automate/internal/flowspec"
)

// routerWithLogger builds a router with a real (discarding) logger so the WARN
// paths — audit-record failure and scheduler failure during publish — execute.
func routerWithLogger(st store.Store, run runner.Runner, auditSvc audit.Service, sched scheduler.Scheduler) http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewRouter(config.Config{Env: config.EnvDev, FileStore: config.FileStoreLocal},
		st, run, auditSvc, sched, AuthConfig{Logger: logger}, nil)
}

// --- fakes ---

// fakeAudit is an in-memory audit.Service that records every Record call and
// serves a fixed list. recErr forces Record to fail (to prove an audit failure
// never breaks the request); listErr forces List to fail.
type fakeAudit struct {
	records []audit.Entry
	list    []audit.Entry
	lastF   audit.Filter
	recErr  error
	listErr error
}

func (f *fakeAudit) Record(_ context.Context, e audit.Entry) error {
	if f.recErr != nil {
		return f.recErr
	}
	f.records = append(f.records, e)
	return nil
}

func (f *fakeAudit) List(_ context.Context, filter audit.Filter) ([]audit.Entry, error) {
	f.lastF = filter
	if f.listErr != nil {
		return nil, f.listErr
	}
	if f.list == nil {
		return []audit.Entry{}, nil
	}
	return f.list, nil
}

// actions returns the ordered action strings of every recorded entry.
func (f *fakeAudit) actions() []string {
	out := make([]string, len(f.records))
	for i, e := range f.records {
		out[i] = e.Action
	}
	return out
}

// syncCall / deleteCall capture a Scheduler invocation for assertions.
type syncCall struct {
	flowID string
	spec   scheduler.Spec
	in     flowspec.FlowInput
}

// fakeScheduler records Sync/Delete/Pause/Resume calls. The *Err fields force the
// matching method to fail so the "scheduler error is non-fatal" paths are covered.
type fakeScheduler struct {
	syncs     []syncCall
	deletes   []string
	pauses    []string
	resumes   []string
	syncErr   error
	deleteErr error
	pauseErr  error
	resumeErr error
}

func (f *fakeScheduler) Sync(_ context.Context, flowID string, spec scheduler.Spec, in flowspec.FlowInput) error {
	f.syncs = append(f.syncs, syncCall{flowID: flowID, spec: spec, in: in})
	return f.syncErr
}
func (f *fakeScheduler) Delete(_ context.Context, flowID string) error {
	f.deletes = append(f.deletes, flowID)
	return f.deleteErr
}
func (f *fakeScheduler) Pause(_ context.Context, flowID string) error {
	f.pauses = append(f.pauses, flowID)
	return f.pauseErr
}
func (f *fakeScheduler) Resume(_ context.Context, flowID string) error {
	f.resumes = append(f.resumes, flowID)
	return f.resumeErr
}

// scheduleDef is a flow definition carrying a trigger.schedule node so
// SpecFromDefinition reports hasSchedule == true.
func scheduleDef() flowspec.FlowDef {
	return flowspec.FlowDef{
		Nodes: []flowspec.NodeDef{
			{ID: "s1", Type: "trigger.schedule", Name: "Every 15m",
				Config: json.RawMessage(`{"mode":"simple","everyMinutes":15}`)},
			{ID: "q1", Type: "db.query", Name: "Query"},
		},
		Edges: []flowspec.EdgeDef{{Source: "s1", Target: "q1"}},
	}
}

// --- audit recording on mutating actions ---

func TestRunFlow_RecordsManualRun(t *testing.T) {
	fake := seedFake()
	fake.def = sampleDef()
	aud := &fakeAudit{}
	r := routerWithDeps(fake, &fakeRunner{result: flowspec.FlowResult{Path: []string{"t1"}}}, aud, nil)

	w, _ := doReq(t, r, http.MethodPost, APIBasePath+"/flows/flw_payroll/run", nil)
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", w.Code)
	}
	if len(aud.records) != 1 {
		t.Fatalf("audit records = %d, want 1", len(aud.records))
	}
	e := aud.records[0]
	if e.Action != audit.ActionExecutionManualRun {
		t.Errorf("action = %q, want %q", e.Action, audit.ActionExecutionManualRun)
	}
	if e.ResourceType != "flow" || e.ResourceID != "flw_payroll" {
		t.Errorf("resource = %s/%s, want flow/flw_payroll", e.ResourceType, e.ResourceID)
	}
	if e.Role != "admin" {
		t.Errorf("role = %q, want admin (resolved default)", e.Role)
	}
}

func TestCreateFlow_RecordsCreate(t *testing.T) {
	aud := &fakeAudit{}
	r := routerWithDeps(seedFake(), &fakeRunner{}, aud, nil)
	w, _ := doReqBody(t, r, http.MethodPost, APIBasePath+"/flows", `{"name":"Tax","folder":"Fin"}`, nil)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", w.Code)
	}
	if got := aud.actions(); len(got) != 1 || got[0] != audit.ActionFlowCreate {
		t.Fatalf("actions = %v, want [flow.create]", got)
	}
	if aud.records[0].ResourceID != "flw_new" {
		t.Errorf("resourceId = %q, want flw_new", aud.records[0].ResourceID)
	}
}

func TestPublishFlow_RecordsPublish(t *testing.T) {
	aud := &fakeAudit{}
	fake := seedFake()
	fake.def = sampleDef() // no schedule node
	r := routerWithDeps(fake, &fakeRunner{}, aud, &fakeScheduler{})
	w, _ := doReqBody(t, r, http.MethodPost, APIBasePath+"/flows/flw_payroll/publish", `{"changeNote":"note"}`, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if got := aud.actions(); len(got) != 1 || got[0] != audit.ActionFlowPublish {
		t.Fatalf("actions = %v, want [flow.publish]", got)
	}
}

func TestConnectionCRUD_RecordsEachAction(t *testing.T) {
	aud := &fakeAudit{}
	r := routerWithDeps(seedFake(), &fakeRunner{}, aud, nil)

	wc, _ := doReqBody(t, r, http.MethodPost, APIBasePath+"/connections", `{"name":"PG","type":"postgres","host":"h"}`, nil)
	if wc.Code != http.StatusCreated {
		t.Fatalf("create = %d, want 201", wc.Code)
	}
	wu, _ := doReqBody(t, r, http.MethodPut, APIBasePath+"/connections/conn_hr", `{"name":"HR2","type":"postgres","host":"h"}`, nil)
	if wu.Code != http.StatusOK {
		t.Fatalf("update = %d, want 200", wu.Code)
	}
	wd, _ := doReq(t, r, http.MethodDelete, APIBasePath+"/connections/conn_hr", nil)
	if wd.Code != http.StatusNoContent {
		t.Fatalf("delete = %d, want 204", wd.Code)
	}

	want := []string{audit.ActionConnectionCreate, audit.ActionConnectionUpdate, audit.ActionConnectionDelete}
	if got := aud.actions(); !equalStrs(got, want) {
		t.Fatalf("actions = %v, want %v", got, want)
	}
	if aud.records[2].ResourceID != "conn_hr" {
		t.Errorf("delete resourceId = %q, want conn_hr", aud.records[2].ResourceID)
	}
}

// A failing mutating action must NOT be audited (record only on success).
func TestConnectionUpdate_NotFound_NoRecord(t *testing.T) {
	aud := &fakeAudit{}
	r := routerWithDeps(seedFake(), &fakeRunner{}, aud, nil)
	w, _ := doReqBody(t, r, http.MethodPut, APIBasePath+"/connections/nope", `{"name":"x","type":"postgres"}`, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
	if len(aud.records) != 0 {
		t.Errorf("records = %v, want none (failed action)", aud.records)
	}
}

// An audit Record failure must never break the request.
func TestAuditFailure_DoesNotBreakRequest(t *testing.T) {
	aud := &fakeAudit{recErr: errors.New("audit db down")}
	r := routerWithDeps(seedFake(), &fakeRunner{}, aud, nil)
	w, body := doReqBody(t, r, http.MethodPost, APIBasePath+"/flows", `{"name":"Still OK"}`, nil)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 despite audit failure", w.Code)
	}
	if body["name"] != "Still OK" {
		t.Errorf("body name = %v, want Still OK", body["name"])
	}
}

// --- publish → scheduler ---

func TestPublish_WithSchedule_CallsSync(t *testing.T) {
	fake := seedFake()
	fake.def = scheduleDef()
	sch := &fakeScheduler{}
	r := routerWithDeps(fake, &fakeRunner{}, &fakeAudit{}, sch)

	w, _ := doReqBody(t, r, http.MethodPost, APIBasePath+"/flows/flw_payroll/publish", `{"changeNote":"note"}`, map[string]string{roleHeader: "designer"})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if len(sch.syncs) != 1 {
		t.Fatalf("sync calls = %d, want 1", len(sch.syncs))
	}
	call := sch.syncs[0]
	if call.flowID != "flw_payroll" {
		t.Errorf("sync flowID = %q, want flw_payroll", call.flowID)
	}
	if call.spec.EveryMinutes != 15 {
		t.Errorf("sync spec.EveryMinutes = %d, want 15", call.spec.EveryMinutes)
	}
	if len(call.in.ViewerRoles) != 1 || call.in.ViewerRoles[0] != "designer" {
		t.Errorf("sync ViewerRoles = %v, want [designer]", call.in.ViewerRoles)
	}
	if len(call.in.Flow.Nodes) != 2 {
		t.Errorf("sync FlowInput.Flow not the published def: %+v", call.in.Flow)
	}
	if len(sch.deletes) != 0 {
		t.Errorf("unexpected Delete calls: %v", sch.deletes)
	}
}

func TestPublish_NoSchedule_CallsDelete(t *testing.T) {
	fake := seedFake()
	fake.def = sampleDef() // no trigger.schedule node
	sch := &fakeScheduler{}
	r := routerWithDeps(fake, &fakeRunner{}, &fakeAudit{}, sch)

	w, _ := doReqBody(t, r, http.MethodPost, APIBasePath+"/flows/flw_payroll/publish", `{"changeNote":"note"}`, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if len(sch.deletes) != 1 || sch.deletes[0] != "flw_payroll" {
		t.Fatalf("delete calls = %v, want [flw_payroll]", sch.deletes)
	}
	if len(sch.syncs) != 0 {
		t.Errorf("unexpected Sync calls: %+v", sch.syncs)
	}
}

// A scheduler error is non-fatal: publish already committed, so it still 200s.
func TestPublish_SchedulerSyncError_StillPublishes(t *testing.T) {
	fake := seedFake()
	fake.def = scheduleDef()
	sch := &fakeScheduler{syncErr: errors.New("temporal unreachable")}
	r := routerWithDeps(fake, &fakeRunner{}, &fakeAudit{}, sch)

	w, body := doReqBody(t, r, http.MethodPost, APIBasePath+"/flows/flw_payroll/publish", `{"changeNote":"note"}`, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 despite scheduler error", w.Code)
	}
	if body["status"] != "published" {
		t.Errorf("status = %v, want published", body["status"])
	}
}

// A malformed schedule config makes SpecFromDefinition error; publish still 200s
// and neither Sync nor Delete is called.
func TestPublish_BadScheduleConfig_StillPublishes(t *testing.T) {
	fake := seedFake()
	fake.def = flowspec.FlowDef{Nodes: []flowspec.NodeDef{
		{ID: "s1", Type: "trigger.schedule", Config: json.RawMessage(`{"mode":"bogus"}`)},
	}}
	sch := &fakeScheduler{}
	r := routerWithDeps(fake, &fakeRunner{}, &fakeAudit{}, sch)

	w, _ := doReqBody(t, r, http.MethodPost, APIBasePath+"/flows/flw_payroll/publish", `{"changeNote":"note"}`, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if len(sch.syncs) != 0 || len(sch.deletes) != 0 {
		t.Errorf("scheduler should not be called on bad config: syncs=%v deletes=%v", sch.syncs, sch.deletes)
	}
}

// When the definition can't be loaded for the schedule sync, publish still 200s.
func TestPublish_DefinitionLoadError_StillPublishes(t *testing.T) {
	fake := seedFake()
	fake.errDef = errors.New("def load boom")
	sch := &fakeScheduler{}
	r := routerWithDeps(fake, &fakeRunner{}, &fakeAudit{}, sch)

	w, _ := doReqBody(t, r, http.MethodPost, APIBasePath+"/flows/flw_payroll/publish", `{"changeNote":"note"}`, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if len(sch.syncs) != 0 || len(sch.deletes) != 0 {
		t.Errorf("scheduler should not be called when def load fails")
	}
}

// --- GET /audit-logs ---

func TestAuditLogs_ReturnsList(t *testing.T) {
	aud := &fakeAudit{list: []audit.Entry{
		{ID: 2, Action: "flow.publish", Role: "admin", CreatedAt: "2026-07-14T10:00:00.000Z"},
		{ID: 1, Action: "auth.login", Role: "admin", CreatedAt: "2026-07-14T09:00:00.000Z"},
	}}
	r := routerWithDeps(seedFake(), &fakeRunner{}, aud, nil)

	w, body := doReq(t, r, http.MethodGet,
		APIBasePath+"/audit-logs?action=flow.publish&from=2026-07-01T00:00:00Z&to=2026-07-31T00:00:00Z&limit=50",
		map[string]string{roleHeader: "admin"})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	logs, ok := body["auditLogs"].([]any)
	if !ok || len(logs) != 2 {
		t.Fatalf("auditLogs = %v, want 2 entries", body["auditLogs"])
	}
	first := logs[0].(map[string]any)
	if first["action"] != "flow.publish" {
		t.Errorf("first action = %v, want flow.publish", first["action"])
	}
	// Filter is threaded through to the service.
	if aud.lastF.Action != "flow.publish" || aud.lastF.Limit != 50 {
		t.Errorf("filter = %+v, want action=flow.publish limit=50", aud.lastF)
	}
	if aud.lastF.From == "" || aud.lastF.To == "" {
		t.Errorf("from/to not threaded: %+v", aud.lastF)
	}
}

func TestAuditLogs_BadLimit_DefaultsToZero(t *testing.T) {
	aud := &fakeAudit{}
	r := routerWithDeps(seedFake(), &fakeRunner{}, aud, nil)
	w, _ := doReq(t, r, http.MethodGet, APIBasePath+"/audit-logs?limit=notanumber",
		map[string]string{roleHeader: "admin"})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if aud.lastF.Limit != 0 {
		t.Errorf("limit = %d, want 0 (malformed → default)", aud.lastF.Limit)
	}
}

func TestAuditLogs_ListError_500(t *testing.T) {
	aud := &fakeAudit{listErr: errors.New("audit db down")}
	r := routerWithDeps(seedFake(), &fakeRunner{}, aud, nil)
	w, body := doReq(t, r, http.MethodGet, APIBasePath+"/audit-logs",
		map[string]string{roleHeader: "admin"})
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	assertErrorEnvelope(t, body, "internal")
}

func TestAuditLogs_ForbiddenForNonAdmin(t *testing.T) {
	for _, role := range []string{"designer", "operator", "viewer", "auditor"} {
		t.Run(role, func(t *testing.T) {
			aud := &fakeAudit{list: []audit.Entry{{ID: 1, Action: "auth.login"}}}
			r := routerWithDeps(seedFake(), &fakeRunner{}, aud, nil)
			w, body := doReq(t, r, http.MethodGet, APIBasePath+"/audit-logs",
				map[string]string{roleHeader: role})
			if w.Code != http.StatusForbidden {
				t.Fatalf("role %s status = %d, want 403", role, w.Code)
			}
			assertErrorEnvelope(t, body, "forbidden")
		})
	}
}

func TestAuditLogs_AllowedForDefaultAdmin(t *testing.T) {
	aud := &fakeAudit{list: []audit.Entry{{ID: 1, Action: "auth.login", Role: "admin"}}}
	r := routerWithDeps(seedFake(), &fakeRunner{}, aud, nil)
	// No X-Role header → resolves to defaultRole (admin).
	w, _ := doReq(t, r, http.MethodGet, APIBasePath+"/audit-logs", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

// --- WARN paths (logger configured) ---

// With a logger wired, an audit-record failure still succeeds and takes the WARN
// branch in handlers.record.
func TestAuditFailure_WithLogger_WarnsAndSucceeds(t *testing.T) {
	aud := &fakeAudit{recErr: errors.New("audit db down")}
	r := routerWithLogger(seedFake(), &fakeRunner{}, aud, nil)
	w, _ := doReqBody(t, r, http.MethodPost, APIBasePath+"/flows", `{"name":"Logged"}`, nil)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", w.Code)
	}
}

// With a logger wired, a scheduler Sync failure takes the WARN branch and still
// returns the published flow.
func TestPublish_SchedulerError_WithLogger_Warns(t *testing.T) {
	fake := seedFake()
	fake.def = scheduleDef()
	sch := &fakeScheduler{syncErr: errors.New("temporal down")}
	r := routerWithLogger(fake, &fakeRunner{}, &fakeAudit{}, sch)
	w, _ := doReqBody(t, r, http.MethodPost, APIBasePath+"/flows/flw_payroll/publish", `{"changeNote":"note"}`, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

// With a logger wired, a scheduler Delete failure (no-schedule branch) also warns
// and still 200s.
func TestPublish_SchedulerDeleteError_WithLogger_Warns(t *testing.T) {
	fake := seedFake()
	fake.def = sampleDef()
	sch := &fakeScheduler{deleteErr: errors.New("temporal down")}
	r := routerWithLogger(fake, &fakeRunner{}, &fakeAudit{}, sch)
	w, _ := doReqBody(t, r, http.MethodPost, APIBasePath+"/flows/flw_payroll/publish", `{"changeNote":"note"}`, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

// With a logger wired, a definition-load error during schedule sync warns and
// still 200s.
func TestPublish_DefLoadError_WithLogger_Warns(t *testing.T) {
	fake := seedFake()
	fake.errDef = errors.New("def boom")
	r := routerWithLogger(fake, &fakeRunner{}, &fakeAudit{}, &fakeScheduler{})
	w, _ := doReqBody(t, r, http.MethodPost, APIBasePath+"/flows/flw_payroll/publish", `{"changeNote":"note"}`, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

// --- auth login → audit Record ---

func TestLocalLogin_RecordsAuthEvents(t *testing.T) {
	svc := loginService(t)
	aud := &fakeAudit{}
	deps := authDeps{service: svc, audit: aud}
	ah := &authHandlers{deps: deps}
	r := gin.New()
	v1 := r.Group(APIBasePath)
	v1.POST("/auth/local/login", ah.localLogin)

	// Failure then success.
	postJSON(t, r, APIBasePath+"/auth/local/login",
		`{"email":"`+auth.DevAdminEmail+`","password":"bad-000000000"}`, nil)
	postJSON(t, r, APIBasePath+"/auth/local/login",
		`{"email":"`+auth.DevAdminEmail+`","password":"`+auth.DevAdminPassword+`"}`, nil)

	want := []string{audit.ActionAuthFailed, audit.ActionAuthLogin}
	if got := aud.actions(); !equalStrs(got, want) {
		t.Fatalf("audit actions = %v, want %v", got, want)
	}
	login := aud.records[1]
	if login.Role != "admin" {
		t.Errorf("login role = %q, want admin", login.Role)
	}
	if login.UserID == "" {
		t.Error("login record missing userId")
	}
}

func TestHighestRoleStr(t *testing.T) {
	tests := []struct {
		name  string
		roles []string
		want  string
	}{
		{"picks highest", []string{"viewer", "admin", "operator"}, "admin"},
		{"none valid → empty", []string{"auditor", "ghost"}, ""},
		{"empty → empty", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := highestRoleStr(tt.roles); got != tt.want {
				t.Errorf("highestRoleStr(%v) = %q, want %q", tt.roles, got, tt.want)
			}
		})
	}
}
